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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"vmware-migration-kit/plugins/module_utils/logger"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

// nbdWaitTimeout is how long a hot-added disk may take to show up in the
// proxy (SCSI rescan + udev) and become servable by nbdkit. Variable so the
// tests do not have to wait for it.
var nbdWaitTimeout = 60 * time.Second

// Export is a snapshot disk attached to the proxy VM and served, read-only,
// by the nbdkit service of the proxy under the export name of its by-id link.
type Export struct {
	p *Proxy
	// FileName is the datastore path of the exported disk, e.g.
	// "[ds1] vm/vm.vmdk". Used to detach exactly this disk.
	FileName string
	// Name is the NBD export name: the by-id link of the disk in the proxy,
	// "wwn-0x<disk uuid>".
	Name string
	// Capacity is the expected size in bytes, to be checked by the caller
	// against the size advertised by the NBD server.
	Capacity int64
	closed   bool
}

// URI is the NBD URI to give to nbdcopy or libnbd.
func (e *Export) URI() string {
	return fmt.Sprintf("nbd://%s/%s", net.JoinHostPort(e.p.Config.Address, strconv.Itoa(e.p.Config.Port)), e.Name)
}

// Export hot-adds the snapshot disk to the proxy VM, read-only, and returns
// the NBD export the conversion host can read it from. snapshotDisk must come
// from the source VM snapshot configuration
// (snapshot.config.hardware.device): its backing file is the frozen
// point-in-time disk, the snapshot chain is handled by vSphere.
//
// Nothing is executed in the proxy: the nbdkit service running there serves
// the new disk as soon as the guest kernel exposes it, because the file plugin
// resolves the export name at connection time.
//
// The caller must Close the export BEFORE removing the snapshot.
func (p *Proxy) Export(ctx context.Context, snapshotDisk *types.VirtualDisk) (*Export, error) {
	fileName, _, _ := diskBacking(snapshotDisk)
	if fileName == "" {
		return nil, fmt.Errorf("snapshot disk %d has no file backing", snapshotDisk.Key)
	}
	disk, s, err := p.attach(ctx, fileName, diskDatastore(snapshotDisk))
	if err != nil {
		return nil, err
	}
	_, uuid, _ := diskBacking(disk)
	exp := &Export{p: p, FileName: fileName, Capacity: snapshotDisk.CapacityInBytes}
	if uuid == "" {
		return nil, errors.Join(
			fmt.Errorf("attached disk %s has no UUID: is disk.EnableUUID=TRUE on proxy VM %s?", fileName, p.Config.VMName),
			exp.Close(ctx))
	}
	exp.Name = WWNFromUUID(uuid)
	logger.Log.Infof("Disk %s attached to proxy %s on SCSI %d:%d, waiting for export %s",
		fileName, p.Config.VMName, s.Bus, s.Unit, exp.Name)

	// The disk appears in the proxy asynchronously (SCSI hot-add + udev), so
	// wait for the export itself, not just for the server. This also checks
	// client-side that it is read-only and has the expected size, which rules
	// out a stale or wrong device.
	if err := WaitExport(ctx, p.Config.Address, p.Config.Port, exp.Name, exp.Capacity, nbdWaitTimeout); err != nil {
		return nil, errors.Join(err, exp.Close(ctx))
	}
	logger.Log.Infof("Disk %s exported at %s", fileName, exp.URI())
	return exp, nil
}

// Close detaches the disk from the proxy VM, keeping its files. It is safe to
// call several times. Nothing has to be stopped in the proxy: the nbdkit
// service opens the disk only while a client is connected.
func (e *Export) Close(ctx context.Context) error {
	if e.closed {
		return nil
	}
	err := e.p.detach(ctx, e.FileName)
	e.closed = true
	return err
}

// CleanupStale detaches (keeping files) any disk left attached to the proxy
// that belongs to srcVM's disk chain, e.g. after a crashed migration. A
// leftover attachment would block the removal of the snapshot.
func (p *Proxy) CleanupStale(ctx context.Context, srcVM *object.VirtualMachine) error {
	var o mo.VirtualMachine
	if err := srcVM.Properties(ctx, srcVM.Reference(), []string{"layoutEx.file"}, &o); err != nil {
		return err
	}
	srcFiles := map[string]bool{}
	if o.LayoutEx != nil {
		for _, f := range o.LayoutEx.File {
			if f.Type == string(types.VirtualMachineFileLayoutExFileTypeDiskDescriptor) {
				srcFiles[f.Name] = true
			}
		}
	}
	devices, err := p.VM.Device(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, d := range devices.SelectByType((*types.VirtualDisk)(nil)) {
		disk := d.(*types.VirtualDisk)
		fileName, _, mode := diskBacking(disk)
		if !srcFiles[fileName] || mode != string(types.VirtualDiskModeIndependent_nonpersistent) {
			continue
		}
		logger.Log.Warnf("Found stale proxy attachment of %s, cleaning up", fileName)
		exp := &Export{p: p, FileName: fileName}
		if err := exp.Close(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// nbdMagic is what an NBD newstyle server sends first: "NBDMAGIC" + "IHAVEOPT".
var nbdMagic = []byte("NBDMAGICIHAVEOPT")

// ProbeNBD connects to address:port and checks the NBD server greeting. It
// says the nbdkit service of the proxy is running and reachable; the export
// itself is checked by the caller when it connects (size and read-only).
func ProbeNBD(address string, port int, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(address, strconv.Itoa(port)), timeout)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	buf := make([]byte, len(nbdMagic))
	if _, err := io.ReadFull(conn, buf); err != nil {
		return fmt.Errorf("no NBD greeting: %w", err)
	}
	if !bytes.Equal(buf, nbdMagic) {
		return fmt.Errorf("unexpected NBD greeting %q", buf)
	}
	return nil
}

// WaitNBD waits until the nbdkit service of the proxy answers.
func WaitNBD(ctx context.Context, address string, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		err := ProbeNBD(address, port, 5*time.Second)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("NBD server %s:%d not reachable from this host "+
				"(is the nbdkit service running on the proxy and the port open for this host?): %w", address, port, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
