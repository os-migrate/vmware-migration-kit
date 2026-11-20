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

	"vmware-migration-kit/plugins/module_utils/logger"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
)

const (
	unitsPerController = 16
	attachRetries      = 5
)

// slot is a free SCSI slot on the proxy and the NBD port bound to it.
type slot struct {
	ControllerKey int32
	Bus           int32
	Unit          int32
	Port          int
}

// portFor maps a SCSI slot to a port: one port per slot, so parallel
// migrations on the same proxy never need to coordinate.
func portFor(portMin int, bus, unit int32) int {
	return portMin + int(bus)*unitsPerController + int(unit)
}

// scsiControllers returns the SCSI controllers of a device list sorted by bus.
func scsiControllers(devices object.VirtualDeviceList) []types.BaseVirtualSCSIController {
	var ctrls []types.BaseVirtualSCSIController
	for _, d := range devices {
		if c, ok := d.(types.BaseVirtualSCSIController); ok {
			ctrls = append(ctrls, c)
		}
	}
	sort.Slice(ctrls, func(i, j int) bool {
		return ctrls[i].GetVirtualSCSIController().BusNumber < ctrls[j].GetVirtualSCSIController().BusNumber
	})
	return ctrls
}

// freeSlots lists the free SCSI slots whose port fits in [portMin, portMax].
func freeSlots(devices object.VirtualDeviceList, portMin, portMax int) []slot {
	var slots []slot
	for _, c := range scsiControllers(devices) {
		sc := c.GetVirtualSCSIController()
		used := map[int32]bool{sc.ScsiCtlrUnitNumber: true} // the controller itself (unit 7)
		for _, d := range devices {
			vd := d.GetVirtualDevice()
			if vd.ControllerKey == sc.Key && vd.UnitNumber != nil {
				used[*vd.UnitNumber] = true
			}
		}
		for unit := int32(0); unit < unitsPerController; unit++ {
			if used[unit] {
				continue
			}
			port := portFor(portMin, sc.BusNumber, unit)
			if port > portMax {
				continue
			}
			slots = append(slots, slot{ControllerKey: sc.Key, Bus: sc.BusNumber, Unit: unit, Port: port})
		}
	}
	return slots
}

// attachSpec hot-adds an EXISTING disk file read-only.
//
// Safety: FileOperation is deliberately empty (never "create": the file
// exists, never a new child disk) and the disk mode is
// independent_nonpersistent, so whatever the proxy writes goes to a throwaway
// redo log and the source snapshot chain is never modified.
func attachSpec(fileName string, datastore *types.ManagedObjectReference, s slot) *types.VirtualDeviceConfigSpec {
	unit := s.Unit
	disk := &types.VirtualDisk{
		VirtualDevice: types.VirtualDevice{
			Key:           -1,
			ControllerKey: s.ControllerKey,
			UnitNumber:    &unit,
			Backing: &types.VirtualDiskFlatVer2BackingInfo{
				VirtualDeviceFileBackingInfo: types.VirtualDeviceFileBackingInfo{
					FileName:  fileName,
					Datastore: datastore,
				},
				DiskMode: string(types.VirtualDiskModeIndependent_nonpersistent),
			},
		},
	}
	return &types.VirtualDeviceConfigSpec{
		Operation: types.VirtualDeviceConfigSpecOperationAdd,
		Device:    disk,
	}
}

// detachSpec removes a disk from the proxy.
//
// Safety: FileOperation is deliberately empty. "destroy" would DELETE the
// source VM's disk file from the datastore.
func detachSpec(disk *types.VirtualDisk) *types.VirtualDeviceConfigSpec {
	return &types.VirtualDeviceConfigSpec{
		Operation: types.VirtualDeviceConfigSpecOperationRemove,
		Device:    disk,
	}
}

// diskBacking returns the file name, UUID and disk mode of a virtual disk.
func diskBacking(d *types.VirtualDisk) (fileName, uuid, mode string) {
	switch b := d.Backing.(type) {
	case *types.VirtualDiskFlatVer2BackingInfo:
		return b.FileName, b.Uuid, b.DiskMode
	case *types.VirtualDiskSeSparseBackingInfo:
		return b.FileName, b.Uuid, b.DiskMode
	case *types.VirtualDiskSparseVer2BackingInfo:
		return b.FileName, b.Uuid, b.DiskMode
	case *types.VirtualDiskRawDiskMappingVer1BackingInfo:
		return b.FileName, b.Uuid, b.DiskMode
	case types.BaseVirtualDeviceFileBackingInfo:
		return b.GetVirtualDeviceFileBackingInfo().FileName, "", ""
	}
	return "", "", ""
}

// diskDatastore returns the datastore reference of a disk backing, if any.
func diskDatastore(d *types.VirtualDisk) *types.ManagedObjectReference {
	if b, ok := d.Backing.(types.BaseVirtualDeviceFileBackingInfo); ok {
		return b.GetVirtualDeviceFileBackingInfo().Datastore
	}
	return nil
}

// WWNFromUUID converts a vSphere disk UUID to the Linux by-id name exposed
// when disk.EnableUUID=TRUE: "6000C293-43fb-..." -> "wwn-0x600c29343fb...".
func WWNFromUUID(uuid string) string {
	return "wwn-0x" + strings.ToLower(strings.ReplaceAll(uuid, "-", ""))
}

// disksByFile returns the proxy disks whose backing file is exactly fileName.
func disksByFile(devices object.VirtualDeviceList, fileName string) []*types.VirtualDisk {
	var disks []*types.VirtualDisk
	for _, d := range devices.SelectByType((*types.VirtualDisk)(nil)) {
		disk := d.(*types.VirtualDisk)
		if f, _, _ := diskBacking(disk); f == fileName {
			disks = append(disks, disk)
		}
	}
	return disks
}

func (p *Proxy) reconfigure(ctx context.Context, change *types.VirtualDeviceConfigSpec) error {
	task, err := p.VM.Reconfigure(ctx, types.VirtualMachineConfigSpec{
		DeviceChange: []types.BaseVirtualDeviceConfigSpec{change},
	})
	if err != nil {
		return err
	}
	return task.Wait(ctx)
}

// attach hot-adds fileName to the proxy on the first free slot, retrying on
// another slot if a parallel migration took it first. It returns the attached
// disk as seen by vSphere and the slot used.
func (p *Proxy) attach(ctx context.Context, fileName string, datastore *types.ManagedObjectReference) (*types.VirtualDisk, slot, error) {
	var lastErr error
	for attempt := 1; attempt <= attachRetries; attempt++ {
		devices, err := p.VM.Device(ctx)
		if err != nil {
			return nil, slot{}, err
		}
		if len(disksByFile(devices, fileName)) > 0 {
			return nil, slot{}, fmt.Errorf("disk %s is already attached to the proxy VM", fileName)
		}
		slots := freeSlots(devices, p.Config.PortMin, p.Config.PortMax)
		if len(slots) == 0 {
			return nil, slot{}, fmt.Errorf("no free SCSI slot on proxy VM %s within ports %d-%d (use another proxy VM)",
				p.Config.VMName, p.Config.PortMin, p.Config.PortMax)
		}
		s := slots[0]
		logger.Log.Infof("Attaching %s to proxy %s (SCSI %d:%d, port %d), attempt %d",
			fileName, p.Config.VMName, s.Bus, s.Unit, s.Port, attempt)
		if err := p.reconfigure(ctx, attachSpec(fileName, datastore, s)); err != nil {
			lastErr = err
			logger.Log.Warnf("Attach attempt %d failed: %v", attempt, err)
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
			continue
		}
		devices, err = p.VM.Device(ctx)
		if err != nil {
			return nil, s, err
		}
		disks := disksByFile(devices, fileName)
		if len(disks) != 1 {
			return nil, s, fmt.Errorf("attached disk %s not found on the proxy VM (found %d)", fileName, len(disks))
		}
		return disks[0], s, nil
	}
	return nil, slot{}, fmt.Errorf("failed to attach %s after %d attempts: %w", fileName, attachRetries, lastErr)
}

// detach removes every proxy disk backed by exactly fileName, keeping files.
// It refuses to touch a disk that is not independent_nonpersistent: such a
// disk was not attached by us (e.g. the proxy's own OS disk).
func (p *Proxy) detach(ctx context.Context, fileName string) error {
	devices, err := p.VM.Device(ctx)
	if err != nil {
		return err
	}
	for _, disk := range disksByFile(devices, fileName) {
		if _, _, mode := diskBacking(disk); mode != string(types.VirtualDiskModeIndependent_nonpersistent) {
			return fmt.Errorf("refusing to detach %s from the proxy: disk mode %q is not independent_nonpersistent", fileName, mode)
		}
		logger.Log.Infof("Detaching %s from proxy %s (keeping files)", fileName, p.Config.VMName)
		if err := p.reconfigure(ctx, detachSpec(disk)); err != nil {
			return fmt.Errorf("failed to detach %s from the proxy: %w", fileName, err)
		}
	}
	return nil
}
