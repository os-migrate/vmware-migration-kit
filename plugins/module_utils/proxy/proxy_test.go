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
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/types"
)

// ---------------------------------------------------------------- safety

func TestAttachSpecNeverCreatesAndIsNonPersistent(t *testing.T) {
	ds := types.ManagedObjectReference{Type: "Datastore", Value: "ds-1"}
	spec := attachSpec("[ds1] vm/vm.vmdk", &ds, slot{ControllerKey: 1000, Bus: 0, Unit: 3})

	if spec.FileOperation != "" {
		t.Fatalf("attach spec must not set a FileOperation, got %q", spec.FileOperation)
	}
	if spec.Operation != types.VirtualDeviceConfigSpecOperationAdd {
		t.Fatalf("expected add operation, got %q", spec.Operation)
	}
	disk := spec.Device.(*types.VirtualDisk)
	b := disk.Backing.(*types.VirtualDiskFlatVer2BackingInfo)
	if b.DiskMode != string(types.VirtualDiskModeIndependent_nonpersistent) {
		t.Fatalf("disk must be independent_nonpersistent, got %q", b.DiskMode)
	}
	if b.Parent != nil {
		t.Fatal("attach spec must not create a child disk")
	}
	if b.FileName != "[ds1] vm/vm.vmdk" || b.Datastore == nil || *b.Datastore != ds {
		t.Fatalf("unexpected backing %+v", b)
	}
	if disk.ControllerKey != 1000 || disk.UnitNumber == nil || *disk.UnitNumber != 3 {
		t.Fatalf("unexpected slot: controller %d unit %v", disk.ControllerKey, disk.UnitNumber)
	}
	if disk.CapacityInBytes != 0 || disk.CapacityInKB != 0 {
		t.Fatal("attach spec must not set a capacity (would mean a new disk)")
	}
}

func TestDetachSpecKeepsFiles(t *testing.T) {
	spec := detachSpec(&types.VirtualDisk{})
	if spec.FileOperation != "" {
		t.Fatalf("detach spec must keep files, got FileOperation %q (destroy deletes the source VMDK)", spec.FileOperation)
	}
	if spec.Operation != types.VirtualDeviceConfigSpecOperationRemove {
		t.Fatalf("expected remove operation, got %q", spec.Operation)
	}
}

// ---------------------------------------------------------------- slots / ports

func scsi(key, bus int32) *types.ParaVirtualSCSIController {
	c := &types.ParaVirtualSCSIController{}
	c.Key = key
	c.BusNumber = bus
	c.ScsiCtlrUnitNumber = 7
	return c
}

func diskAt(ctrl, unit int32, file string) *types.VirtualDisk {
	u := unit
	return &types.VirtualDisk{VirtualDevice: types.VirtualDevice{
		ControllerKey: ctrl, UnitNumber: &u,
		Backing: &types.VirtualDiskFlatVer2BackingInfo{
			VirtualDeviceFileBackingInfo: types.VirtualDeviceFileBackingInfo{FileName: file},
			DiskMode:                     "persistent",
		},
	}}
}

func TestPortFor(t *testing.T) {
	cases := []struct {
		bus, unit int32
		want      int
	}{{0, 0, 10809}, {0, 15, 10824}, {1, 0, 10825}, {3, 15, 10872}}
	for _, c := range cases {
		if got := portFor(10809, c.bus, c.unit); got != c.want {
			t.Errorf("portFor(bus %d, unit %d) = %d, want %d", c.bus, c.unit, got, c.want)
		}
	}
	if portFor(DefaultPortMin, 3, 15) != DefaultPortMin+PortsPerProxy-1 {
		t.Error("the last slot must map to the last port of a 64-port range")
	}
}

func TestFreeSlots(t *testing.T) {
	devices := object.VirtualDeviceList{scsi(1000, 0), scsi(1001, 1), diskAt(1000, 0, "os.vmdk")}

	slots := freeSlots(devices, 10809, 10872)
	// 2 controllers x 16 units - 2 controller units (7) - 1 OS disk
	if len(slots) != 29 {
		t.Fatalf("expected 29 free slots, got %d", len(slots))
	}
	if s := slots[0]; s.Bus != 0 || s.Unit != 1 || s.Port != 10810 {
		t.Fatalf("first free slot should be 0:1 port 10810, got %+v", s)
	}
	for _, s := range slots {
		if s.Unit == 7 {
			t.Fatalf("unit 7 is the controller itself, must not be used: %+v", s)
		}
	}
	// Port range smaller than the slots: only slots whose port fits are returned.
	slots = freeSlots(devices, 10809, 10812)
	if len(slots) != 3 { // units 1,2,3 of bus 0
		t.Fatalf("expected 3 slots within 10809-10812, got %d", len(slots))
	}
	if len(freeSlots(object.VirtualDeviceList{diskAt(200, 0, "ide.vmdk")}, 10809, 10872)) != 0 {
		t.Fatal("no SCSI controller must mean no slot")
	}
}

// ---------------------------------------------------------------- helpers

func TestWWNFromUUID(t *testing.T) {
	got := WWNFromUUID("6000C293-43fb-7db7-9ba0-c86bceb5c325")
	if got != "wwn-0x6000c29343fb7db79ba0c86bceb5c325" { // value verified on the PoC proxy
		t.Fatalf("got %s", got)
	}
}

func TestGuestProgramSpec(t *testing.T) {
	s := guestProgramSpec("echo 'a b'", true)
	if s.ProgramPath != "/usr/bin/sudo" || s.Arguments != `-n /bin/sh -c 'echo '\''a b'\'''` {
		t.Fatalf("unexpected root spec %+v", s)
	}
	s = guestProgramSpec("exit 0", false)
	if s.ProgramPath != "/bin/sh" || s.Arguments != "-c 'exit 0'" {
		t.Fatalf("unexpected user spec %+v", s)
	}
}

func TestExportScript(t *testing.T) {
	s := exportScript("wwn-0xabc", 21474836480, "10.0.0.5", 10810)
	for _, want := range []string{
		"W=/dev/disk/by-id/wwn-0xabc",
		`blockdev --setro "$D"`,
		`= "21474836480" ] || exit 11`,
		"nbdkit --readonly --port 10810 --ipaddr 10.0.0.5 --pidfile /run/osm-nbdkit-wwn-0xabc.pid file",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("export script missing %q:\n%s", want, s)
		}
	}
	if !strings.Contains(stopScript("wwn-0xabc"), "/device/delete") {
		t.Error("stop script must hot-remove the SCSI device in the guest")
	}
}

func TestDatacenterPath(t *testing.T) {
	cases := map[string]string{
		"/Eco-Datacenter/vm/rhos-migrations/rhel-9.4-1": "/Eco-Datacenter",
		"/Folder/DC/vm/name":                            "/Folder/DC",
		"/ha-datacenter/vm//my-vm":                      "/ha-datacenter", // default vddkpath + "/" + name
		"Eco-Datacenter/vm/x":                           "/Eco-Datacenter",
		"my-vm":                                         "",
		"":                                              "",
	}
	for in, want := range cases {
		if got := datacenterPath(in); got != want {
			t.Errorf("datacenterPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConfigDefaults(t *testing.T) {
	c, err := Config{VMName: "p", GuestUser: "u", GuestPassword: "x"}.withDefaults()
	if err != nil || c.PortMin != 10809 || c.PortMax != 10872 {
		t.Fatalf("unexpected defaults %+v %v", c, err)
	}
	if _, err := (Config{VMName: "p"}).withDefaults(); err == nil {
		t.Fatal("guest credentials must be required")
	}
	if _, err := (Config{VMName: "p", GuestUser: "u", GuestPassword: "x", PortMin: 2000, PortMax: 1000}).withDefaults(); err == nil {
		t.Fatal("inverted port range must be rejected")
	}
}

// ---------------------------------------------------------------- NBD probe

// fakeNBD listens on 127.0.0.1:port and sends greeting to every client.
func fakeNBD(t *testing.T, port int, greeting string) func() {
	t.Helper()
	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Skipf("cannot listen on port %d: %v", port, err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			_, _ = c.Write([]byte(greeting))
			_ = c.Close()
		}
	}()
	return func() { _ = l.Close(); wg.Wait() }
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

func TestProbeNBD(t *testing.T) {
	port := freePort(t)
	stop := fakeNBD(t, port, "NBDMAGICIHAVEOPT\x00\x03")
	if err := ProbeNBD("127.0.0.1", port, 2*time.Second); err != nil {
		t.Fatalf("valid greeting rejected: %v", err)
	}
	stop()

	port = freePort(t)
	stop = fakeNBD(t, port, "SSH-2.0-OpenSSH_9\r\n")
	defer stop()
	if err := ProbeNBD("127.0.0.1", port, 2*time.Second); err == nil {
		t.Fatal("non-NBD greeting accepted")
	}
	if err := ProbeNBD("127.0.0.1", freePort(t), time.Second); err == nil {
		t.Fatal("closed port accepted")
	}
}

// ---------------------------------------------------------------- vcsim

// fakeGuest records the scripts and returns the configured exit codes.
type fakeGuest struct {
	mu      sync.Mutex
	scripts []string
	codes   []int32 // returned in order, then 0
	authErr error
}

func (f *fakeGuest) Validate(context.Context) error { return f.authErr }

func (f *fakeGuest) Run(_ context.Context, script string, _ bool, _ time.Duration) (int32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scripts = append(f.scripts, script)
	if len(f.codes) > 0 {
		c := f.codes[0]
		f.codes = f.codes[1:]
		return c, nil
	}
	return 0, nil
}

// simSetup returns a proxy on DC0_H0_VM0 (with a pvscsi controller added) and
// the source VM DC0_H0_VM1 with a snapshot.
func simSetup(ctx context.Context, t *testing.T, c *vim25.Client, guest GuestRunner, portMin int) (*Proxy, *object.VirtualMachine, *find.Finder) {
	t.Helper()
	finder := find.NewFinder(c, true)
	dc, err := finder.DefaultDatacenter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	finder.SetDatacenter(dc)
	pvm, err := finder.VirtualMachine(ctx, "DC0_H0_VM0")
	if err != nil {
		t.Fatal(err)
	}
	devices, err := pvm.Device(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ctrl, err := devices.CreateSCSIController("pvscsi")
	if err != nil {
		t.Fatal(err)
	}
	if err := pvm.AddDevice(ctx, ctrl); err != nil {
		t.Fatal(err)
	}
	src, err := finder.VirtualMachine(ctx, "DC0_H0_VM1")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Config{VMName: "DC0_H0_VM0", Address: "127.0.0.1", PortMin: portMin, GuestUser: "u", GuestPassword: "p"}.withDefaults()
	if err != nil {
		t.Fatal(err)
	}
	return &Proxy{Client: c, VM: pvm, Config: cfg, Guest: guest}, src, finder
}

func firstDisk(ctx context.Context, t *testing.T, vm *object.VirtualMachine) *types.VirtualDisk {
	t.Helper()
	devices, err := vm.Device(ctx)
	if err != nil {
		t.Fatal(err)
	}
	disks := devices.SelectByType((*types.VirtualDisk)(nil))
	if len(disks) == 0 {
		t.Fatal("source VM has no disk")
	}
	return disks[0].(*types.VirtualDisk)
}

func TestSimExportAttachAndDetach(t *testing.T) {
	simulator.Test(func(ctx context.Context, c *vim25.Client) {
		guest := &fakeGuest{}
		portMin := freePort(t)
		p, src, _ := simSetup(ctx, t, c, guest, portMin)
		srcDisk := firstDisk(ctx, t, src)
		fileName, _, _ := diskBacking(srcDisk)

		// The export port is the first free slot of the new pvscsi controller.
		devices, _ := p.VM.Device(ctx)
		want := freeSlots(devices, p.Config.PortMin, p.Config.PortMax)[0]
		stop := fakeNBD(t, want.Port, "NBDMAGICIHAVEOPT")
		defer stop()

		exp, err := p.Export(ctx, srcDisk)
		if err != nil {
			t.Fatalf("Export: %v", err)
		}
		if exp.Port != want.Port || !strings.HasPrefix(exp.URI(), "nbd://127.0.0.1:") {
			t.Fatalf("unexpected export %s (want port %d)", exp.URI(), want.Port)
		}
		devices, _ = p.VM.Device(ctx)
		attached := disksByFile(devices, fileName)
		if len(attached) != 1 {
			t.Fatalf("expected the source disk attached once to the proxy, got %d", len(attached))
		}
		if _, _, mode := diskBacking(attached[0]); mode != string(types.VirtualDiskModeIndependent_nonpersistent) {
			t.Fatalf("attached disk mode %q", mode)
		}
		if len(guest.scripts) != 1 || !strings.Contains(guest.scripts[0], "nbdkit --readonly --port "+strconv.Itoa(want.Port)) {
			t.Fatalf("export script not run as expected: %v", guest.scripts)
		}

		// A second export of the same disk is refused.
		if _, err := p.Export(ctx, srcDisk); err == nil {
			t.Fatal("double attach must be refused")
		}

		if err := exp.Close(ctx); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if err := exp.Close(ctx); err != nil {
			t.Fatalf("second Close must be a no-op: %v", err)
		}
		devices, _ = p.VM.Device(ctx)
		if len(disksByFile(devices, fileName)) != 0 {
			t.Fatal("disk still attached after Close")
		}
		if !strings.Contains(guest.scripts[len(guest.scripts)-1], "/device/delete") {
			t.Fatal("Close must run the stop script in the guest")
		}
		// The source VM still has its disk (files kept).
		if f, _, _ := diskBacking(firstDisk(ctx, t, src)); f != fileName {
			t.Fatalf("source disk changed: %s", f)
		}
	})
}

func TestSimExportFailureDetaches(t *testing.T) {
	simulator.Test(func(ctx context.Context, c *vim25.Client) {
		guest := &fakeGuest{codes: []int32{exitSizeMismatch}}
		p, src, _ := simSetup(ctx, t, c, guest, freePort(t))
		srcDisk := firstDisk(ctx, t, src)
		fileName, _, _ := diskBacking(srcDisk)

		_, err := p.Export(ctx, srcDisk)
		if err == nil || !strings.Contains(err.Error(), "size differs") {
			t.Fatalf("expected size mismatch error, got %v", err)
		}
		devices, _ := p.VM.Device(ctx)
		if len(disksByFile(devices, fileName)) != 0 {
			t.Fatal("disk must be detached when the export fails")
		}
	})
}

func TestSimDetachRefusesPersistentDisk(t *testing.T) {
	simulator.Test(func(ctx context.Context, c *vim25.Client) {
		p, _, _ := simSetup(ctx, t, c, &fakeGuest{}, freePort(t))
		own := firstDisk(ctx, t, p.VM) // the proxy's own (persistent) disk
		fileName, _, _ := diskBacking(own)
		if err := p.detach(ctx, fileName); err == nil {
			t.Fatal("detach of a persistent disk must be refused")
		}
		devices, _ := p.VM.Device(ctx)
		if len(disksByFile(devices, fileName)) != 1 {
			t.Fatal("proxy disk must still be attached")
		}
	})
}

func TestSimNewInDatacenterOf(t *testing.T) {
	simulator.Test(func(ctx context.Context, c *vim25.Client) {
		cfg := Config{VMName: "DC0_H0_VM0", Address: "127.0.0.1", GuestUser: "u", GuestPassword: "p"}
		// Inventory path of the VM being migrated, in the same datacenter.
		p, err := NewInDatacenterOf(ctx, c, "/DC0/vm/DC0_H0_VM1", cfg)
		if err != nil {
			t.Fatalf("NewInDatacenterOf: %v", err)
		}
		if p.VM.Reference().Value == "" || p.Config.PortMin != DefaultPortMin {
			t.Fatalf("unexpected proxy %+v", p.Config)
		}
		if _, err := NewInDatacenterOf(ctx, c, "/nope/vm/x", cfg); err == nil {
			t.Fatal("unknown datacenter must fail")
		}
	})
}

func TestSimPrecheckSourceVMs(t *testing.T) {
	simulator.Test(func(ctx context.Context, c *vim25.Client) {
		p, _, finder := simSetup(ctx, t, c, &fakeGuest{}, 10809)
		r := Precheck(ctx, finder, p, PrecheckOptions{VMs: []string{"DC0_H0_VM1", "DC0_H0_VM0", "nope"}, CBTSync: true})

		get := func(name, target string) *Check {
			for i := range r.Checks {
				if r.Checks[i].Name == name && strings.HasPrefix(r.Checks[i].Target, target) {
					return &r.Checks[i]
				}
			}
			return nil
		}
		if ch := get("vm_disk_datastore", "DC0_H0_VM1"); ch == nil || ch.Status != StatusOK {
			t.Errorf("source disk datastore should be accessible from the proxy host: %+v", ch)
		}
		if ch := get("vm_cbt", "DC0_H0_VM1"); ch == nil || ch.Status != StatusWarn {
			t.Errorf("CBT disabled + cbt_sync should warn: %+v", ch)
		}
		if ch := get("vm_not_proxy", "DC0_H0_VM0"); ch == nil || ch.Status != StatusFail {
			t.Errorf("migrating the proxy itself must fail: %+v", ch)
		}
		if ch := get("vm_found", "nope"); ch == nil || ch.Status != StatusFail {
			t.Errorf("missing VM must fail: %+v", ch)
		}
		if ch := get("proxy_scsi", "DC0_H0_VM0"); ch == nil || ch.Status != StatusOK {
			t.Errorf("proxy with pvscsi should pass the SCSI check: %+v", ch)
		}
		if ch := get("proxy_enable_uuid", "DC0_H0_VM0"); ch == nil || ch.Status != StatusFail {
			t.Errorf("disk.EnableUUID not set must fail: %+v", ch)
		}
		if !r.Failed {
			t.Error("report must be failed")
		}
		if r.Facts.Host == "" || len(r.Facts.Datastores) == 0 || r.Facts.FreeSlots == 0 {
			t.Errorf("missing facts: %+v", r.Facts)
		}
	})
}
