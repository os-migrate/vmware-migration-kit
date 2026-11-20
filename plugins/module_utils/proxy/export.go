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

// Exit codes of the export script run in the proxy guest.
const (
	exitDeviceNotFound = 10
	exitSizeMismatch   = 11
	exitNotReadOnly    = 12
	exitNbdkitFailed   = 13
	exitNoNbdkit       = 14
)

var exportExitMsg = map[int32]string{
	exitDeviceNotFound: "attached disk did not show up in the proxy (check disk.EnableUUID=TRUE)",
	exitSizeMismatch:   "block device size differs from the disk capacity",
	exitNotReadOnly:    "could not set the block device read-only",
	exitNbdkitFailed:   "nbdkit failed to start (port in use or firewall/SELinux issue)",
	exitNoNbdkit:       "nbdkit is not installed in the proxy",
	1:                  "sudo failed (the guest user needs passwordless sudo)",
}

const (
	guestScriptTimeout = 2 * time.Minute
	nbdWaitTimeout     = 30 * time.Second
)

// Export is a snapshot disk exported by nbdkit on the proxy VM.
type Export struct {
	p        *Proxy
	FileName string
	WWN      string
	Port     int
	closed   bool
}

// URI is the NBD URI to give to nbdcopy / libnbd.
func (e *Export) URI() string {
	return fmt.Sprintf("nbd://%s", net.JoinHostPort(e.p.Config.Address, strconv.Itoa(e.Port)))
}

func pidFile(wwn string) string { return "/run/osm-nbdkit-" + wwn + ".pid" }

// exportScript waits for the hot-added disk, forces it read-only, checks its
// size and starts nbdkit on it. nbdkit binds the port before going to the
// background, so a 0 exit code means the export is listening.
func exportScript(wwn string, capacity int64, address string, port int) string {
	return fmt.Sprintf(`W=/dev/disk/by-id/%[1]s
command -v nbdkit >/dev/null 2>&1 || exit %[5]d
udevadm settle 2>/dev/null || true
i=0
while [ ! -e "$W" ]; do i=$((i+1)); [ $i -gt 60 ] && exit %[6]d; sleep 1; done
D=$(readlink -f "$W")
blockdev --setro "$D" || exit %[7]d
for p in "$D"[0-9]*; do [ -e "$p" ] && blockdev --setro "$p"; done
[ "$(blockdev --getro "$D")" = 1 ] || exit %[7]d
[ "$(blockdev --getsize64 "$D")" = "%[2]d" ] || exit %[8]d
nbdkit --readonly --port %[3]d --ipaddr %[4]s --pidfile %[9]s file "$W" || exit %[10]d
`, wwn, capacity, port, address, exitNoNbdkit, exitDeviceNotFound, exitNotReadOnly, exitSizeMismatch, pidFile(wwn), exitNbdkitFailed)
}

// stopScript stops nbdkit for a disk and removes the SCSI device from the
// guest before vSphere detaches it (clean hot-remove).
func stopScript(wwn string) string {
	return fmt.Sprintf(`P=%[2]s
if [ -f "$P" ]; then
  pid=$(cat "$P"); kill "$pid" 2>/dev/null
  i=0; while kill -0 "$pid" 2>/dev/null && [ $i -lt 50 ]; do i=$((i+1)); sleep 0.2; done
  rm -f "$P"
fi
W=/dev/disk/by-id/%[1]s
if [ -e "$W" ]; then echo 1 > "/sys/block/$(basename "$(readlink -f "$W")")/device/delete"; fi
exit 0
`, wwn, pidFile(wwn))
}

// Export hot-adds the snapshot disk to the proxy and exports it read-only with
// nbdkit. snapshotDisk must come from the source VM snapshot configuration
// (snapshot.config.hardware.device): its backing file is the frozen
// point-in-time disk, the snapshot chain is handled by vSphere.
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
	exp := &Export{p: p, FileName: fileName, WWN: WWNFromUUID(uuid), Port: s.Port}
	if uuid == "" {
		return nil, errors.Join(fmt.Errorf("attached disk %s has no UUID", fileName), exp.Close(ctx))
	}

	code, err := p.Guest.Run(ctx, exportScript(exp.WWN, snapshotDisk.CapacityInBytes, p.Config.Address, exp.Port), true, guestScriptTimeout)
	if err == nil && code != 0 {
		msg, ok := exportExitMsg[code]
		if !ok {
			msg = "unexpected error"
		}
		err = fmt.Errorf("export script failed with exit code %d: %s", code, msg)
	}
	if err != nil {
		return nil, errors.Join(fmt.Errorf("failed to export %s on proxy: %w", fileName, err), exp.Close(ctx))
	}
	if err := WaitNBD(ctx, p.Config.Address, exp.Port, nbdWaitTimeout); err != nil {
		return nil, errors.Join(err, exp.Close(ctx))
	}
	logger.Log.Infof("Disk %s exported at %s", fileName, exp.URI())
	return exp, nil
}

// Close stops nbdkit and detaches the disk from the proxy, keeping files.
// It is safe to call several times.
func (e *Export) Close(ctx context.Context) error {
	if e.closed {
		return nil
	}
	var errs []error
	if e.WWN != "wwn-0x" {
		code, err := e.p.Guest.Run(ctx, stopScript(e.WWN), true, guestScriptTimeout)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to stop nbdkit on proxy: %w", err))
		} else if code != 0 {
			errs = append(errs, fmt.Errorf("stop script failed with exit code %d", code))
		}
	}
	if err := e.p.detach(ctx, e.FileName); err != nil {
		errs = append(errs, err)
	}
	e.closed = true
	return errors.Join(errs...)
}

// CleanupStale detaches (keeping files) any disk left attached to the proxy
// that belongs to srcVM's disk chain, e.g. after a crashed migration. A
// leftover attachment would block the snapshot removal on the source VM.
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
		fileName, uuid, mode := diskBacking(disk)
		if !srcFiles[fileName] || mode != string(types.VirtualDiskModeIndependent_nonpersistent) {
			continue
		}
		logger.Log.Warnf("Found stale proxy attachment of %s, cleaning up", fileName)
		exp := &Export{p: p, FileName: fileName, WWN: WWNFromUUID(uuid)}
		if err := exp.Close(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// nbdMagic is what an NBD newstyle server sends first: "NBDMAGIC" + "IHAVEOPT".
var nbdMagic = []byte("NBDMAGICIHAVEOPT")

// ProbeNBD connects to address:port and checks the NBD server greeting.
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

// WaitNBD waits until an NBD server answers on address:port.
func WaitNBD(ctx context.Context, address string, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		err := ProbeNBD(address, port, 5*time.Second)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("NBD server %s:%d not reachable from this host: %w", address, port, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
