/*
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright 2026 Red Hat, Inc.
 *
 */

package proxy

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

// Status of a pre-check.
type Status string

const (
	StatusOK   Status = "ok"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
)

// Check is the result of one pre-check.
type Check struct {
	Name   string `json:"name"`
	Target string `json:"target"`
	Status Status `json:"status"`
	Msg    string `json:"msg"`
}

// Facts describes the proxy VM.
type Facts struct {
	Name       string   `json:"name"`
	Address    string   `json:"address"`
	Host       string   `json:"host"`
	PowerState string   `json:"power_state"`
	Datastores []string `json:"datastores"`
	FreeSlots  int      `json:"free_slots"`
	PortMin    int      `json:"port_min"`
	PortMax    int      `json:"port_max"`
}

// Report is the result of Precheck.
type Report struct {
	Checks []Check `json:"checks"`
	Facts  Facts   `json:"facts"`
	Failed bool    `json:"failed"`
}

func (r *Report) add(name, target string, st Status, format string, a ...interface{}) {
	r.Checks = append(r.Checks, Check{Name: name, Target: target, Status: st, Msg: fmt.Sprintf(format, a...)})
	if st == StatusFail {
		r.Failed = true
	}
}

// PrecheckOptions controls Precheck.
type PrecheckOptions struct {
	// VMs to migrate (names or inventory paths).
	VMs []string
	// CBTSync: warn when CBT is disabled on a VM.
	CBTSync bool
	// CheckNetwork starts a temporary nbdkit on the first and last port of the
	// range and checks this host can reach it.
	CheckNetwork bool
}

// proxyInfo is what the source VM checks need to know about the proxy.
type proxyInfo struct {
	datastores map[types.ManagedObjectReference]string
	diskUUIDs  map[string]bool
}

// Precheck verifies the proxy VM is usable and can access every VM to migrate.
func Precheck(ctx context.Context, finder *find.Finder, p *Proxy, opts PrecheckOptions) Report {
	r := Report{Checks: []Check{}}
	info, ready := p.checkProxy(ctx, &r)
	if ready {
		guestOK := p.checkGuest(ctx, &r)
		if opts.CheckNetwork {
			if guestOK {
				p.checkNetwork(ctx, &r)
			} else {
				r.add("network", p.Config.VMName, StatusFail, "skipped: guest operations are not working")
			}
		}
	}
	for _, name := range opts.VMs {
		vm, err := finder.VirtualMachine(ctx, name)
		if err != nil {
			r.add("vm_found", name, StatusFail, "VM not found: %v", err)
			continue
		}
		r.add("vm_found", name, StatusOK, "found %s", vm.InventoryPath)
		p.checkSourceVM(ctx, &r, name, vm, info, opts.CBTSync)
	}
	return r
}

// checkProxy checks the proxy VM configuration through the vSphere API.
// It returns information for the source VM checks and whether the guest is
// ready for guest operations.
func (p *Proxy) checkProxy(ctx context.Context, r *Report) (proxyInfo, bool) {
	name := p.Config.VMName
	info := proxyInfo{datastores: map[types.ManagedObjectReference]string{}, diskUUIDs: map[string]bool{}}
	r.Facts = Facts{Name: name, Address: p.Config.Address, PortMin: p.Config.PortMin, PortMax: p.Config.PortMax, Datastores: []string{}}

	var o mo.VirtualMachine
	err := p.VM.Properties(ctx, p.VM.Reference(), []string{
		"runtime.powerState", "runtime.host", "guest.toolsRunningStatus", "config.extraConfig", "config.hardware.device",
	}, &o)
	if err != nil || o.Config == nil {
		r.add("proxy_config", name, StatusFail, "cannot read proxy VM configuration: %v", err)
		return info, false
	}
	r.Facts.PowerState = string(o.Runtime.PowerState)

	ready := true
	if o.Runtime.PowerState == types.VirtualMachinePowerStatePoweredOn {
		r.add("proxy_power", name, StatusOK, "powered on")
	} else {
		r.add("proxy_power", name, StatusFail, "proxy VM is %s, it must be powered on", o.Runtime.PowerState)
		ready = false
	}
	if o.Guest != nil && o.Guest.ToolsRunningStatus == string(types.VirtualMachineToolsRunningStatusGuestToolsRunning) {
		r.add("proxy_tools", name, StatusOK, "VMware Tools running")
	} else {
		r.add("proxy_tools", name, StatusFail, "VMware Tools not running (needed for guest operations)")
		ready = false
	}

	enableUUID := false
	for _, opt := range o.Config.ExtraConfig {
		if ov := opt.GetOptionValue(); ov.Key == "disk.EnableUUID" {
			enableUUID = strings.EqualFold(fmt.Sprint(ov.Value), "true")
		}
	}
	if enableUUID {
		r.add("proxy_enable_uuid", name, StatusOK, "disk.EnableUUID=TRUE")
	} else {
		r.add("proxy_enable_uuid", name, StatusFail,
			"disk.EnableUUID is not TRUE: attached disks cannot be identified in the guest (power off the proxy and set it)")
	}

	devices := object.VirtualDeviceList(o.Config.Hardware.Device)
	ctrls := scsiControllers(devices)
	slots := freeSlots(devices, p.Config.PortMin, p.Config.PortMax)
	r.Facts.FreeSlots = len(slots)
	switch {
	case len(ctrls) == 0:
		r.add("proxy_scsi", name, StatusFail, "no SCSI controller (pvscsi or LSI Logic needed for hot-add)")
	case len(slots) == 0:
		r.add("proxy_scsi", name, StatusFail, "no free SCSI slot within ports %d-%d", p.Config.PortMin, p.Config.PortMax)
	default:
		r.add("proxy_scsi", name, StatusOK, "%d SCSI controller(s), %d free slot(s) -> up to %d concurrent disk exports",
			len(ctrls), len(slots), len(slots))
	}
	for _, d := range devices.SelectByType((*types.VirtualDisk)(nil)) {
		if _, uuid, _ := diskBacking(d.(*types.VirtualDisk)); uuid != "" {
			info.diskUUIDs[strings.ToLower(uuid)] = true
		}
	}

	if o.Runtime.Host != nil {
		var host mo.HostSystem
		pc := property.DefaultCollector(p.Client)
		if err := pc.RetrieveOne(ctx, *o.Runtime.Host, []string{"name", "datastore"}, &host); err != nil {
			r.add("proxy_host", name, StatusWarn, "cannot read the proxy ESXi host: %v", err)
		} else {
			r.Facts.Host = host.Name
			var dss []mo.Datastore
			if len(host.Datastore) > 0 {
				if err := pc.Retrieve(ctx, host.Datastore, []string{"name", "summary.accessible"}, &dss); err != nil {
					r.add("proxy_host", name, StatusWarn, "cannot read the proxy host datastores: %v", err)
				}
			}
			for _, ds := range dss {
				if ds.Summary.Accessible {
					info.datastores[ds.Reference()] = ds.Name
					r.Facts.Datastores = append(r.Facts.Datastores, ds.Name)
				}
			}
			sort.Strings(r.Facts.Datastores)
			r.add("proxy_host", name, StatusOK, "on host %s, %d accessible datastore(s)", host.Name, len(info.datastores))
		}
	}
	return info, ready
}

// Exit codes of the guest tools check script.
const (
	exitGuestNoNbdkit     = 20
	exitGuestNoFilePlugin = 21
)

// checkGuest checks guest credentials, sudo and nbdkit inside the proxy.
func (p *Proxy) checkGuest(ctx context.Context, r *Report) bool {
	name := p.Config.VMName
	if err := p.Guest.Validate(ctx); err != nil {
		r.add("proxy_guest_auth", name, StatusFail, "guest credentials rejected for %q: %v", p.Config.GuestUser, err)
		return false
	}
	r.add("proxy_guest_auth", name, StatusOK, "guest credentials valid for %q", p.Config.GuestUser)

	code, err := p.Guest.Run(ctx, "exit 0", true, time.Minute)
	if err != nil || code != 0 {
		r.add("proxy_sudo", name, StatusFail, "passwordless sudo does not work for %q (exit %d, %v)", p.Config.GuestUser, code, err)
		return false
	}
	r.add("proxy_sudo", name, StatusOK, "passwordless sudo works")

	script := fmt.Sprintf(`command -v nbdkit >/dev/null 2>&1 || exit %d
d=$(nbdkit --dump-config | sed -n 's/^plugindir=//p')
[ -e "$d/nbdkit-file-plugin.so" ] || exit %d
exit 0`, exitGuestNoNbdkit, exitGuestNoFilePlugin)
	code, err = p.Guest.Run(ctx, script, true, time.Minute)
	switch {
	case err != nil:
		r.add("proxy_nbdkit", name, StatusFail, "cannot check nbdkit: %v", err)
		return false
	case code == exitGuestNoNbdkit:
		r.add("proxy_nbdkit", name, StatusFail, "nbdkit is not installed in the proxy")
		return false
	case code == exitGuestNoFilePlugin:
		r.add("proxy_nbdkit", name, StatusFail, "nbdkit file plugin is missing (install nbdkit-basic-plugins)")
		return false
	case code != 0:
		r.add("proxy_nbdkit", name, StatusFail, "nbdkit check failed with exit code %d", code)
		return false
	}
	r.add("proxy_nbdkit", name, StatusOK, "nbdkit and its file plugin are installed")
	return true
}

// checkNetwork starts a temporary nbdkit (null plugin) on the first and last
// port of the range and checks that this host gets the NBD greeting. This
// validates routing, NAT and the proxy firewall end to end.
func (p *Proxy) checkNetwork(ctx context.Context, r *Report) {
	ports := []int{p.Config.PortMin}
	if p.Config.PortMax != p.Config.PortMin {
		ports = append(ports, p.Config.PortMax)
	}
	for _, port := range ports {
		target := fmt.Sprintf("%s:%d", p.Config.Address, port)
		pid := fmt.Sprintf("/run/osm-nbdkit-precheck-%d.pid", port)
		start := fmt.Sprintf("nbdkit --readonly --port %d --ipaddr %s --pidfile %s null size=1M", port, p.Config.Address, pid)
		code, err := p.Guest.Run(ctx, start, true, time.Minute)
		if err != nil || code != 0 {
			r.add("network", target, StatusWarn,
				"could not start a test nbdkit on the proxy (port in use by a running export, or address %s is not local to the proxy): exit %d, %v",
				p.Config.Address, code, err)
			continue
		}
		probeErr := ProbeNBD(p.Config.Address, port, 10*time.Second)
		stop := fmt.Sprintf(`[ -f %[1]s ] && kill "$(cat %[1]s)"; rm -f %[1]s; exit 0`, pid)
		_, _ = p.Guest.Run(ctx, stop, true, time.Minute)
		if probeErr != nil {
			r.add("network", target, StatusFail,
				"conversion host cannot reach NBD on the proxy (open the port range in the proxy firewall for this host, check routing/NAT): %v", probeErr)
			continue
		}
		r.add("network", target, StatusOK, "NBD reachable from this host")
	}
}

// checkSourceVM checks that the proxy can access the disks of a VM to migrate.
func (p *Proxy) checkSourceVM(ctx context.Context, r *Report, name string, vm *object.VirtualMachine, info proxyInfo, cbtSync bool) {
	if vm.Reference() == p.VM.Reference() {
		r.add("vm_not_proxy", name, StatusFail, "the proxy VM cannot migrate itself")
		return
	}
	var o mo.VirtualMachine
	if err := vm.Properties(ctx, vm.Reference(), []string{"config.hardware.device", "config.changeTrackingEnabled"}, &o); err != nil || o.Config == nil {
		r.add("vm_config", name, StatusFail, "cannot read VM configuration: %v", err)
		return
	}
	disks := object.VirtualDeviceList(o.Config.Hardware.Device).SelectByType((*types.VirtualDisk)(nil))
	if len(disks) == 0 {
		r.add("vm_disks", name, StatusFail, "VM has no virtual disk")
		return
	}
	for _, d := range disks {
		disk := d.(*types.VirtualDisk)
		fileName, uuid, mode := diskBacking(disk)
		target := fmt.Sprintf("%s disk %d", name, disk.Key)

		if rdm, ok := disk.Backing.(*types.VirtualDiskRawDiskMappingVer1BackingInfo); ok && rdm.CompatibilityMode == string(types.VirtualDiskCompatibilityModePhysicalMode) {
			r.add("vm_disk_type", target, StatusFail, "%s is a physical-mode RDM: it cannot be snapshotted", fileName)
			continue
		}
		if strings.HasPrefix(mode, "independent") {
			r.add("vm_disk_mode", target, StatusFail, "%s is %s: independent disks are not included in snapshots", fileName, mode)
			continue
		}
		if ds := diskDatastore(disk); ds == nil {
			r.add("vm_disk_datastore", target, StatusFail, "%s has no datastore", fileName)
		} else if dsName, ok := info.datastores[*ds]; ok {
			r.add("vm_disk_datastore", target, StatusOK, "datastore %s is accessible from the proxy host", dsName)
		} else {
			r.add("vm_disk_datastore", target, StatusFail,
				"%s is on a datastore not accessible from the proxy host %s: hot-add impossible (move the proxy or use another proxy)", fileName, r.Facts.Host)
		}
		if uuid != "" && info.diskUUIDs[strings.ToLower(uuid)] {
			r.add("vm_disk_uuid", target, StatusFail, "disk UUID %s is also used by a proxy disk (proxy cloned from this VM?)", uuid)
		}
	}
	cbt := o.Config.ChangeTrackingEnabled != nil && *o.Config.ChangeTrackingEnabled
	switch {
	case cbt:
		r.add("vm_cbt", name, StatusOK, "CBT enabled")
	case cbtSync:
		r.add("vm_cbt", name, StatusWarn, "CBT is disabled: incremental sync is not possible, only full copies")
	default:
		r.add("vm_cbt", name, StatusOK, "CBT disabled (not needed without CBT sync)")
	}
}
