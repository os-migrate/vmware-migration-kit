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
	"encoding/binary"
	"errors"
	"io"
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

func TestFreeSlots(t *testing.T) {
	devices := object.VirtualDeviceList{scsi(1000, 0), scsi(1001, 1), diskAt(1000, 0, "os.vmdk")}

	// Legacy limit: 2 controllers x 16 units - 2 controller units (7) - 1 OS disk.
	slots := freeSlots(devices, 13)
	if len(slots) != 29 {
		t.Fatalf("expected 29 free slots on an old hardware version, got %d", len(slots))
	}
	if s := slots[0]; s.Bus != 0 || s.Unit != 1 {
		t.Fatalf("first free slot should be 0:1, got %+v", s)
	}
	for _, s := range slots {
		if s.Unit == 7 {
			t.Fatalf("unit 7 is the controller itself, must not be used: %+v", s)
		}
	}

	// PVSCSI on hardware version 14 and later addresses 64 targets, which is
	// what allows dozens of migrations at once on a single proxy.
	slots = freeSlots(devices, 15)
	if len(slots) != 125 { // 2 x (64 - 1) - 1 OS disk
		t.Fatalf("expected 125 free slots on vmx-15, got %d", len(slots))
	}
	if len(freeSlots(object.VirtualDeviceList{diskAt(200, 0, "ide.vmdk")}, 15)) != 0 {
		t.Fatal("no SCSI controller must mean no slot")
	}
}

func TestHardwareVersion(t *testing.T) {
	for in, want := range map[string]int{"vmx-15": 15, "vmx-21": 21, "vmx-9": 9, "": 0, "garbage": 0} {
		if got := hardwareVersion(in); got != want {
			t.Errorf("hardwareVersion(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestControllerUnits(t *testing.T) {
	pv := scsi(1000, 0)
	lsi := &types.VirtualLsiLogicController{}
	lsi.Key = 1001
	lsi.ScsiCtlrUnitNumber = 7

	if got := controllerUnits(pv, 15); got != pvscsiUnits {
		t.Errorf("PVSCSI on vmx-15 should offer %d targets, got %d", pvscsiUnits, got)
	}
	// Older hardware, or another controller type, keeps the classic limit.
	if got := controllerUnits(pv, 13); got != legacySCSIUnits {
		t.Errorf("PVSCSI on vmx-13 should keep %d targets, got %d", legacySCSIUnits, got)
	}
	if got := controllerUnits(lsi, 21); got != legacySCSIUnits {
		t.Errorf("LSI Logic should keep %d targets, got %d", legacySCSIUnits, got)
	}
}

// ---------------------------------------------------------------- helpers

func TestWWNFromUUID(t *testing.T) {
	got := WWNFromUUID("6000C293-43fb-7db7-9ba0-c86bceb5c325")
	if got != "wwn-0x6000c29343fb7db79ba0c86bceb5c325" { // value verified on the PoC proxy
		t.Fatalf("got %s", got)
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
	c, err := Config{VMName: "p"}.withDefaults()
	if err != nil || c.Port != DefaultPort {
		t.Fatalf("unexpected defaults %+v %v", c, err)
	}
	if _, err := (Config{}).withDefaults(); err == nil {
		t.Fatal("the proxy VM name must be required")
	}
	if _, err := (Config{VMName: "p", Port: 70000}).withDefaults(); err == nil {
		t.Fatal("an invalid port must be rejected")
	}
}

// ---------------------------------------------------------------- NBD client

// fakeNBD is a minimal NBD newstyle server used to exercise the handshake
// client. exports returns the size and read-only flag of a name, or found
// false to answer NBD_REP_ERR_UNKNOWN.
func fakeNBD(t *testing.T, exports func(name string) (uint64, bool, bool)) (addr string, port int, stop func()) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
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
			go serveFakeNBD(c, exports)
		}
	}()
	a := l.Addr().(*net.TCPAddr)
	return a.IP.String(), a.Port, func() { _ = l.Close(); wg.Wait() }
}

func serveFakeNBD(c net.Conn, exports func(string) (uint64, bool, bool)) {
	defer func() { _ = c.Close() }()
	reply := func(option, repType uint32, payload []byte) error {
		buf := binary.BigEndian.AppendUint64(nil, nbdReplyMagic)
		buf = binary.BigEndian.AppendUint32(buf, option)
		buf = binary.BigEndian.AppendUint32(buf, repType)
		buf = binary.BigEndian.AppendUint32(buf, uint32(len(payload)))
		_, err := c.Write(append(buf, payload...))
		return err
	}
	if _, err := c.Write(append(append([]byte{}, nbdMagic...), 0, 1)); err != nil {
		return
	}
	var clientFlags uint32
	if err := binary.Read(c, binary.BigEndian, &clientFlags); err != nil {
		return
	}
	for {
		header := make([]byte, 16)
		if _, err := io.ReadFull(c, header); err != nil {
			return
		}
		option := binary.BigEndian.Uint32(header[8:12])
		data := make([]byte, binary.BigEndian.Uint32(header[12:16]))
		if _, err := io.ReadFull(c, data); err != nil {
			return
		}
		if option == nbdOptAbort {
			return
		}
		if option != nbdOptInfo || len(data) < 4 {
			_ = reply(option, nbdRepError+1, nil)
			continue
		}
		name := string(data[4 : 4+binary.BigEndian.Uint32(data[:4])])
		size, readOnly, found := exports(name)
		if !found {
			_ = reply(option, nbdRepErrUnknown, []byte("no such export"))
			continue
		}
		var flags uint16
		if readOnly {
			flags |= nbdFlagReadOnly
		}
		info := binary.BigEndian.AppendUint16(nil, nbdInfoExport)
		info = binary.BigEndian.AppendUint64(info, size)
		info = binary.BigEndian.AppendUint16(info, flags)
		if reply(option, nbdRepInfo, info) != nil || reply(option, nbdRepAck, nil) != nil {
			return
		}
	}
}

// oneExport serves a single read-only export of the given name and size.
func oneExport(name string, size uint64) func(string) (uint64, bool, bool) {
	return func(n string) (uint64, bool, bool) {
		if n != name {
			return 0, false, false
		}
		return size, true, true
	}
}

func TestQueryExport(t *testing.T) {
	addr, port, stop := fakeNBD(t, oneExport("wwn-0xabc", 21474836480))
	defer stop()

	info, err := QueryExport(addr, port, "wwn-0xabc", 5*time.Second)
	if err != nil {
		t.Fatalf("QueryExport: %v", err)
	}
	if info.Size != 21474836480 || !info.ReadOnly {
		t.Fatalf("unexpected export info %+v", info)
	}
	if _, err := QueryExport(addr, port, "wwn-0xmissing", 5*time.Second); !errors.Is(err, ErrExportNotFound) {
		t.Fatalf("missing export should give ErrExportNotFound, got %v", err)
	}
}

func TestWaitExportChecksSizeAndReadOnly(t *testing.T) {
	ctx := context.Background()

	addr, port, stop := fakeNBD(t, oneExport("wwn-0xabc", 100))
	if err := WaitExport(ctx, addr, port, "wwn-0xabc", 100, 2*time.Second); err != nil {
		t.Fatalf("matching export rejected: %v", err)
	}
	// A different size means a stale or wrong device in the proxy.
	err := WaitExport(ctx, addr, port, "wwn-0xabc", 200, 2*time.Second)
	if err == nil || !strings.Contains(err.Error(), "wrong or stale device") {
		t.Fatalf("size mismatch not caught: %v", err)
	}
	// A missing export is retried, then reported as "did not appear".
	err = WaitExport(ctx, addr, port, "wwn-0xnope", 100, 2*time.Second)
	if err == nil || !strings.Contains(err.Error(), "did not appear") {
		t.Fatalf("missing export not reported: %v", err)
	}
	stop()

	// A writable export must be refused: the service must run --readonly.
	addr, port, stop = fakeNBD(t, func(string) (uint64, bool, bool) { return 100, false, true })
	defer stop()
	err = WaitExport(ctx, addr, port, "wwn-0xabc", 100, 2*time.Second)
	if err == nil || !strings.Contains(err.Error(), "writable") {
		t.Fatalf("writable export not refused: %v", err)
	}
}

func TestProbeNBD(t *testing.T) {
	addr, port, stop := fakeNBD(t, oneExport("x", 1))
	if err := ProbeNBD(addr, port, 2*time.Second); err != nil {
		t.Fatalf("valid greeting rejected: %v", err)
	}
	stop()

	// A non-NBD server must not be mistaken for one.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	go func() {
		if c, err := l.Accept(); err == nil {
			_, _ = c.Write([]byte("SSH-2.0-OpenSSH_9\r\n"))
			_ = c.Close()
		}
	}()
	a := l.Addr().(*net.TCPAddr)
	if err := ProbeNBD(a.IP.String(), a.Port, 2*time.Second); err == nil {
		t.Fatal("non-NBD greeting accepted")
	}
}

// ---------------------------------------------------------------- vcsim

// simSetup returns a proxy on DC0_H0_VM0 (with a pvscsi controller added),
// pointed at the given NBD endpoint, and the source VM DC0_H0_VM1.
func simSetup(ctx context.Context, t *testing.T, c *vim25.Client, addr string, port int) (*Proxy, *object.VirtualMachine, *find.Finder) {
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
	cfg, err := Config{VMName: "DC0_H0_VM0", Address: addr, Port: port}.withDefaults()
	if err != nil {
		t.Fatal(err)
	}
	return &Proxy{Client: c, VM: pvm, Config: cfg}, src, finder
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
		// Serve any export name: the UUID the simulator gives the attached
		// disk is only known after the attach.
		var served string
		addr, port, stop := fakeNBD(t, func(n string) (uint64, bool, bool) {
			served = n
			return 10737418240, true, true
		})
		defer stop()

		p, src, _ := simSetup(ctx, t, c, addr, port)
		srcDisk := firstDisk(ctx, t, src)
		srcDisk.CapacityInBytes = 10737418240
		fileName, _, _ := diskBacking(srcDisk)

		exp, err := p.Export(ctx, srcDisk)
		if err != nil {
			t.Fatalf("Export: %v", err)
		}
		devices, _ := p.VM.Device(ctx)
		attached := disksByFile(devices, fileName)
		if len(attached) != 1 {
			t.Fatalf("expected the source disk attached once to the proxy, got %d", len(attached))
		}
		if _, _, mode := diskBacking(attached[0]); mode != string(types.VirtualDiskModeIndependent_nonpersistent) {
			t.Fatalf("attached disk mode %q", mode)
		}
		// The export name must be the by-id name of the attached disk.
		_, uuid, _ := diskBacking(attached[0])
		if exp.Name != WWNFromUUID(uuid) || served != exp.Name {
			t.Fatalf("export name %q, served %q, disk uuid %q", exp.Name, served, uuid)
		}
		if want := "nbd://" + net.JoinHostPort(addr, strconv.Itoa(port)) + "/" + exp.Name; exp.URI() != want {
			t.Fatalf("URI %q, want %q", exp.URI(), want)
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
		// The source VM still has its disk (files kept).
		if f, _, _ := diskBacking(firstDisk(ctx, t, src)); f != fileName {
			t.Fatalf("source disk changed: %s", f)
		}
	})
}

func TestSimExportDetachesWhenExportNeverAppears(t *testing.T) {
	simulator.Test(func(ctx context.Context, c *vim25.Client) {
		// Server up, but it never serves the disk (as if it had not shown up
		// in the proxy): Export must fail and leave nothing attached.
		addr, port, stop := fakeNBD(t, func(string) (uint64, bool, bool) { return 0, false, false })
		defer stop()

		p, src, _ := simSetup(ctx, t, c, addr, port)
		srcDisk := firstDisk(ctx, t, src)
		fileName, _, _ := diskBacking(srcDisk)

		old := nbdWaitTimeout
		nbdWaitTimeout = 2 * time.Second
		defer func() { nbdWaitTimeout = old }()

		if _, err := p.Export(ctx, srcDisk); err == nil || !strings.Contains(err.Error(), "did not appear") {
			t.Fatalf("expected a timeout error, got %v", err)
		}
		devices, _ := p.VM.Device(ctx)
		if len(disksByFile(devices, fileName)) != 0 {
			t.Fatal("disk must be detached when the export never appears")
		}
	})
}

func TestSimDetachRefusesPersistentDisk(t *testing.T) {
	simulator.Test(func(ctx context.Context, c *vim25.Client) {
		p, _, _ := simSetup(ctx, t, c, "127.0.0.1", 10809)
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
		cfg := Config{VMName: "DC0_H0_VM0", Address: "127.0.0.1"}
		p, err := NewInDatacenterOf(ctx, c, "/DC0/vm/DC0_H0_VM1", cfg)
		if err != nil {
			t.Fatalf("NewInDatacenterOf: %v", err)
		}
		if p.VM.Reference().Value == "" || p.Config.Port != DefaultPort {
			t.Fatalf("unexpected proxy %+v", p.Config)
		}
		if _, err := NewInDatacenterOf(ctx, c, "/nope/vm/x", cfg); err == nil {
			t.Fatal("unknown datacenter must fail")
		}
	})
}

func TestSimPrecheck(t *testing.T) {
	simulator.Test(func(ctx context.Context, c *vim25.Client) {
		addr, port, stop := fakeNBD(t, func(string) (uint64, bool, bool) { return 1 << 30, true, true })
		defer stop()
		p, _, finder := simSetup(ctx, t, c, addr, port)
		r := Precheck(ctx, finder, p, PrecheckOptions{
			VMs: []string{"DC0_H0_VM1", "DC0_H0_VM0", "nope"}, CBTSync: true, CheckNetwork: true,
		})

		get := func(name, target string) *Check {
			for i := range r.Checks {
				if r.Checks[i].Name == name && strings.HasPrefix(r.Checks[i].Target, target) {
					return &r.Checks[i]
				}
			}
			return nil
		}
		if ch := get("nbd_service", addr); ch == nil || ch.Status != StatusOK {
			t.Errorf("reachable nbdkit service should pass: %+v", ch)
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
		if ch := get("proxy_enable_uuid", "DC0_H0_VM0"); ch == nil || ch.Status != StatusFail {
			t.Errorf("disk.EnableUUID not set must fail: %+v", ch)
		}
		if ch := get("proxy_tools", "DC0_H0_VM0"); ch == nil || ch.Status == StatusFail {
			t.Errorf("VMware Tools must not be fatal any more: %+v", ch)
		}
		if !r.Failed {
			t.Error("report must be failed")
		}
		if r.Facts.Host == "" || len(r.Facts.Datastores) == 0 || r.Facts.FreeSlots == 0 {
			t.Errorf("missing facts: %+v", r.Facts)
		}
	})
}

// ---------------------------------------------------------------- parallelism

// diskWithUUID builds an attached disk with a given backing file and UUID.
func diskWithUUID(key, ctrl, unit int32, file, uuid string) *types.VirtualDisk {
	d := diskAt(ctrl, unit, file)
	d.Key = key
	d.Backing.(*types.VirtualDiskFlatVer2BackingInfo).Uuid = uuid
	return d
}

// Several migrations share one proxy and one NBD port: disks are told apart by
// export name, which is the disk UUID. Two disks sharing a UUID would collapse
// onto a single /dev/disk/by-id link in the proxy, so one migration could read
// the other's disk. That must be refused rather than silently copied.
func TestDuplicateUUIDIsDetected(t *testing.T) {
	mine := diskWithUUID(2000, 1000, 1, "[ds] a/a.vmdk", "6000C293-43fb-7db7-9ba0-c86bceb5c325")
	other := diskWithUUID(2001, 1000, 2, "[ds] b/b.vmdk", "6000c293-43FB-7db7-9ba0-c86bceb5c325") // same, different case
	unrelated := diskWithUUID(2002, 1000, 3, "[ds] c/c.vmdk", "6000C291-f0c8-470c-061c-1a10d75fcf14")

	// On its own, or with unrelated disks, the disk is unambiguous.
	if got := duplicateUUID(object.VirtualDeviceList{mine}, mine); got != nil {
		t.Fatalf("a disk must not duplicate itself, got %v", got)
	}
	if got := duplicateUUID(object.VirtualDeviceList{mine, unrelated}, mine); got != nil {
		t.Fatalf("distinct UUIDs must not collide, got %v", got)
	}

	// A second disk with the same UUID, whatever the case, is a collision.
	got := duplicateUUID(object.VirtualDeviceList{mine, other, unrelated}, mine)
	if got == nil || got.Key != other.Key {
		t.Fatalf("duplicate UUID not detected, got %v", got)
	}

	// A disk with no UUID cannot be exported at all; that is reported
	// elsewhere, and must not be mistaken for a duplicate of another.
	noUUID := diskWithUUID(2003, 1000, 4, "[ds] d/d.vmdk", "")
	alsoNoUUID := diskWithUUID(2004, 1000, 5, "[ds] e/e.vmdk", "")
	if got := duplicateUUID(object.VirtualDeviceList{noUUID, alsoNoUUID}, noUUID); got != nil {
		t.Fatalf("missing UUIDs must not count as duplicates, got %v", got)
	}
}

// Two disks of two VMs attached at the same time must each get their own
// export name, and each migration waits for its own.
func TestSimParallelExportsAreDistinct(t *testing.T) {
	simulator.Test(func(ctx context.Context, c *vim25.Client) {
		served := map[string]bool{}
		addr, port, stop := fakeNBD(t, func(n string) (uint64, bool, bool) {
			served[n] = true
			return 10737418240, true, true
		})
		defer stop()

		p, src, finder := simSetup(ctx, t, c, addr, port)
		other, err := finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
		if err != nil {
			t.Fatalf("second source VM: %v", err)
		}
		d1, d2 := firstDisk(ctx, t, src), firstDisk(ctx, t, other)
		d1.CapacityInBytes, d2.CapacityInBytes = 10737418240, 10737418240

		e1, err := p.Export(ctx, d1)
		if err != nil {
			t.Fatalf("first export: %v", err)
		}
		e2, err := p.Export(ctx, d2)
		if err != nil {
			t.Fatalf("second export while the first is open: %v", err)
		}
		if e1.Name == e2.Name {
			t.Fatalf("two disks got the same export name %q", e1.Name)
		}
		if !served[e1.Name] || !served[e2.Name] {
			t.Fatalf("each migration must wait for its own export, served: %v", served)
		}
		// Both are attached at once, on different SCSI slots.
		devices, _ := p.VM.Device(ctx)
		if n := len(disksByFile(devices, e1.FileName)) + len(disksByFile(devices, e2.FileName)); n != 2 {
			t.Fatalf("expected both disks attached, got %d", n)
		}
		// Closing one leaves the other alone.
		if err := e1.Close(ctx); err != nil {
			t.Fatalf("close first: %v", err)
		}
		devices, _ = p.VM.Device(ctx)
		if len(disksByFile(devices, e1.FileName)) != 0 || len(disksByFile(devices, e2.FileName)) != 1 {
			t.Fatal("closing one export must not detach the other")
		}
		if err := e2.Close(ctx); err != nil {
			t.Fatalf("close second: %v", err)
		}
	})
}

// The duplicate check must be wired into the attach path, not just available.
//
// The simulator assigns the UUID from the disk file and ignores attempts to
// change it, so the collision can only be built if that ever changes; the
// detection itself is covered by TestDuplicateUUIDIsDetected.
func TestSimAttachedDiskRefusesDuplicateUUID(t *testing.T) {
	simulator.Test(func(ctx context.Context, c *vim25.Client) {
		addr, port, stop := fakeNBD(t, func(string) (uint64, bool, bool) { return 10737418240, true, true })
		defer stop()
		p, src, _ := simSetup(ctx, t, c, addr, port)
		srcDisk := firstDisk(ctx, t, src)
		srcDisk.CapacityInBytes = 10737418240
		fileName, _, _ := diskBacking(srcDisk)

		exp, err := p.Export(ctx, srcDisk)
		if err != nil {
			t.Fatalf("Export: %v", err)
		}
		defer func() { _ = exp.Close(ctx) }()

		// Try to give the proxy's own disk the UUID of the attached one.
		devices, _ := p.VM.Device(ctx)
		_, uuid, _ := diskBacking(disksByFile(devices, fileName)[0])
		own := firstDisk(ctx, t, p.VM)
		own.Backing.(*types.VirtualDiskFlatVer2BackingInfo).Uuid = uuid
		if err := p.VM.EditDevice(ctx, own); err != nil {
			t.Skipf("the simulator refuses to edit a disk UUID: %v", err)
		}
		devices, _ = p.VM.Device(ctx)
		if duplicateUUID(devices, disksByFile(devices, fileName)[0]) == nil {
			t.Skip("the simulator ignored the UUID change, the collision cannot be built here")
		}

		if _, err := p.attachedDisk(ctx, fileName); err == nil ||
			!strings.Contains(err.Error(), "same UUID") {
			t.Fatalf("a disk sharing its UUID with another must be refused, got %v", err)
		}
	})
}
