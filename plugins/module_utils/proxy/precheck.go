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
	// ProxyDiskWWNs are the by-id names of the disks of the proxy itself,
	// used to check the nbdkit service really serves /dev/disk/by-id.
	ProxyDiskWWNs []string `json:"proxy_disk_wwns"`
	FreeSlots     int      `json:"free_slots"`
	Port          int      `json:"port"`
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
	// CheckNetwork checks this host can reach the nbdkit service of the proxy.
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
	info := p.checkProxy(ctx, &r)
	if opts.CheckNetwork {
		p.checkNetwork(&r)
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

// checkProxy checks the proxy VM configuration through the vSphere API and
// returns the information needed by the source VM checks.
func (p *Proxy) checkProxy(ctx context.Context, r *Report) proxyInfo {
	name := p.Config.VMName
	info := proxyInfo{datastores: map[types.ManagedObjectReference]string{}, diskUUIDs: map[string]bool{}}
	r.Facts = Facts{Name: name, Address: p.Config.Address, Port: p.Config.Port,
		Datastores: []string{}, ProxyDiskWWNs: []string{}}

	var o mo.VirtualMachine
	err := p.VM.Properties(ctx, p.VM.Reference(), []string{
		"runtime.powerState", "runtime.host", "guest.toolsRunningStatus", "config.extraConfig", "config.hardware.device",
	}, &o)
	if err != nil || o.Config == nil {
		r.add("proxy_config", name, StatusFail, "cannot read proxy VM configuration: %v", err)
		return info
	}
	r.Facts.PowerState = string(o.Runtime.PowerState)

	if o.Runtime.PowerState == types.VirtualMachinePowerStatePoweredOn {
		r.add("proxy_power", name, StatusOK, "powered on")
	} else {
		r.add("proxy_power", name, StatusFail, "proxy VM is %s, it must be powered on", o.Runtime.PowerState)
	}
	// Tools are not needed to migrate (nothing is executed in the proxy),
	// only to report the proxy IP when it is not configured explicitly.
	if o.Guest != nil && o.Guest.ToolsRunningStatus == string(types.VirtualMachineToolsRunningStatusGuestToolsRunning) {
		r.add("proxy_tools", name, StatusOK, "VMware Tools running")
	} else {
		r.add("proxy_tools", name, StatusWarn, "VMware Tools not running (only used to discover the proxy IP)")
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
	slots := freeSlots(devices)
	r.Facts.FreeSlots = len(slots)
	switch {
	case len(ctrls) == 0:
		r.add("proxy_scsi", name, StatusFail, "no SCSI controller (pvscsi or LSI Logic needed for hot-add)")
	case len(slots) == 0:
		r.add("proxy_scsi", name, StatusFail, "no free SCSI slot on the proxy VM")
	default:
		r.add("proxy_scsi", name, StatusOK, "%d SCSI controller(s), %d free slot(s) -> up to %d concurrent disk exports",
			len(ctrls), len(slots), len(slots))
	}
	for _, d := range devices.SelectByType((*types.VirtualDisk)(nil)) {
		if _, uuid, _ := diskBacking(d.(*types.VirtualDisk)); uuid != "" {
			info.diskUUIDs[strings.ToLower(uuid)] = true
			r.Facts.ProxyDiskWWNs = append(r.Facts.ProxyDiskWWNs, WWNFromUUID(uuid))
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
	return info
}

// checkNetwork checks that this host (the conversion host) can reach the
// nbdkit service of the proxy. Nothing is started in the proxy: the service
// runs there permanently, so its greeting also proves it is up.
func (p *Proxy) checkNetwork(r *Report) {
	target := fmt.Sprintf("%s:%d", p.Config.Address, p.Config.Port)
	if err := ProbeNBD(p.Config.Address, p.Config.Port, 10*time.Second); err != nil {
		r.add("nbd_service", target, StatusFail,
			"no NBD server answering: start the nbdkit service on the proxy "+
				"(nbdkit --readonly --port %d file dir=/dev/disk/by-id) and open the port for this host: %v",
			p.Config.Port, err)
		return
	}
	r.add("nbd_service", target, StatusOK, "nbdkit service reachable from this host")

	// The proxy's own disks are served too, so any export proves the service
	// really serves /dev/disk/by-id and that by-id names are usable.
	if len(r.Facts.ProxyDiskWWNs) == 0 {
		return
	}
	wwn := r.Facts.ProxyDiskWWNs[0]
	info, err := QueryExport(p.Config.Address, p.Config.Port, wwn, 10*time.Second)
	switch {
	case err != nil:
		r.add("nbd_exports", target, StatusWarn,
			"the service answers but does not serve the proxy disk %s: check it runs with dir=/dev/disk/by-id, "+
				"and that disk.EnableUUID=TRUE has been applied (a power cycle is needed) so the guest has "+
				"/dev/disk/by-id/wwn-* links (%v)", wwn, err)
	case !info.ReadOnly:
		r.add("nbd_exports", target, StatusFail,
			"export %s is writable: the nbdkit service of the proxy must run with --readonly", wwn)
	default:
		r.add("nbd_exports", target, StatusOK, "serves disks by their by-id name, read-only (checked with %s)", wwn)
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
