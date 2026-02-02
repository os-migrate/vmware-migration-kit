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
 * Copyright 2026 Whitestack.
 *
 */

package moduleutils

import (
	"context"
	"testing"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/types"

	vmware_utils "vmware-migration-kit/plugins/module_utils/vmware"
)

func TestGetDatastoreNameForDiskKey(t *testing.T) {
	var failIfError = func(msg string, err error) {
		if err != nil {
			t.Errorf("Error: %s", msg)
			t.Errorf("%v", err)
			t.Fatal("Tests could not be run due an error creating the mock.")
		}
	}

	simulator.Run(func(ctx context.Context, c *vim25.Client) error {
		vmName := "testing_vm_01"
		dStoreName := "LocalDS_0"
		finder := find.NewFinder(c)

		// Datacenter
		dc, err := finder.Datacenter(ctx, "DC0")
		failIfError("Could not find Datacenter", err)
		finder.SetDatacenter(dc)

		// Folder
		folders, err := dc.Folders(ctx)
		failIfError("Could not find Folder", err)

		// ResourcePool
		pool, err := finder.ResourcePool(ctx, "DC0_C0/Resources")
		failIfError("Could not find Pool", err)

		// VM's spec
		spec := types.VirtualMachineConfigSpec{
			Name:    vmName,
			GuestId: string(types.VirtualMachineGuestOsIdentifierOtherGuest),
			Files: &types.VirtualMachineFileInfo{
				VmPathName: "[" + dStoreName + "]",
			},
		}

		// SCSI controller and Disk spec
		var devices object.VirtualDeviceList
		var controllerName string

		scsi, err := devices.CreateSCSIController("scsi")
		failIfError("Could not create SCSI controller", err)
		devices = append(devices, scsi)
		controllerName = devices.Name(scsi)

		controller, err := devices.FindDiskController(controllerName)
		failIfError("Could not find SCSI controller", err)

		disk := &types.VirtualDisk{
			VirtualDevice: types.VirtualDevice{
				Key: devices.NewKey(),
				Backing: &types.VirtualDiskFlatVer2BackingInfo{
					DiskMode:        string(types.VirtualDiskModePersistent),
					ThinProvisioned: types.NewBool(true),
				},
			},
			CapacityInKB: 1 * 1024 * 1024, // 1GB
		}

		devices.AssignController(disk, controller)
		devices = append(devices, disk)

		// Adds device spec to VM spec
		deviceChange, err := devices.ConfigSpec(types.VirtualDeviceConfigSpecOperationAdd)
		failIfError("Could not ADD spec change", err)
		spec.DeviceChange = deviceChange

		// Create VM
		task, err := folders.VmFolder.CreateVM(ctx, spec, pool, nil)
		failIfError("Could not create VM", err)
		info, err := task.WaitForResult(ctx)
		failIfError("Unknown", err)
		vm := object.NewVirtualMachine(c, info.Result.(types.ManagedObjectReference))
		name, err := vm.ObjectName(ctx)
		failIfError("Unknown", err)

		// Tests
		if name != vmName {
			t.Errorf("Test Error: VirtualMachine name does not match %s", vmName)
		}

		diskKeys, err := vmware_utils.GetDiskKeys(ctx, vm)
		failIfError("Could not get disk keys", err)

		if len(diskKeys) == 0 {
			t.Errorf("Test Error: 0 disk keys retrieved for VM %s", vmName)
		}

		dsName, err := vmware_utils.GetDatastoreNameForDiskKey(ctx, vm, diskKeys[0])
		failIfError("Could not get datastore name", err)

		if dsName != dStoreName {
			t.Errorf("Test Error: datastore names mismatch. Wanted '%s'. Found '%s'", dStoreName, dsName)
		}

		return nil
	})
}
