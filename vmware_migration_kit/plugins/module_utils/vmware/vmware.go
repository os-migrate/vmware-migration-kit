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
 * Copyright 2024 Red Hat, Inc.
 *
 */

package vmware

import (
	"context"
	"crypto/sha1"
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"

	"vmware-migration-kit/vmware_migration_kit/plugins/module_utils/logger"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"libguestfs.org/libnbd"
)

type VddkConfig struct {
	VirtualMachine    *object.VirtualMachine
	SnapshotReference types.ManagedObjectReference
	DiskKey           int32
}

const maxChunkSize = 64 * 1024 * 1024

func VMWareAuth(ctx context.Context, server string, user string, password string) (*govmomi.Client, error) {
	u, _ := url.Parse("https://" + server + "/sdk")
	ProcessUrl(u, user, password)
	c, err := govmomi.NewClient(ctx, u, true)
	if err != nil {
		logger.Log.Infof("Failed to authenticate to VMware client %v", err)
		return nil, err
	}
	return c, nil
}

func ProcessUrl(u *url.URL, user string, password string) {
	if user != "" {
		u.User = url.UserPassword(user, password)
	}
}

func GetThumbprint(host string, port string) (string, error) {
	config := tls.Config{
		InsecureSkipVerify: true,
	}
	if port == "" {
		port = "443"
	}

	conn, err := tls.Dial("tcp", fmt.Sprintf("%s:%s", host, port), &config)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	if len(conn.ConnectionState().PeerCertificates) == 0 {
		logger.Log.Infof("No certificates found")
		return "", errors.New("no certificates found")
	}

	certificate := conn.ConnectionState().PeerCertificates[0]
	sha1Bytes := sha1.Sum(certificate.Raw)

	thumbprint := make([]string, len(sha1Bytes))
	for i, b := range sha1Bytes {
		thumbprint[i] = fmt.Sprintf("%02X", b)
	}

	return strings.Join(thumbprint, ":"), nil
}

func (v *VddkConfig) IsWindowsFamily(ctx context.Context) (bool, error) {
	var vmConfig mo.VirtualMachine
	err := v.VirtualMachine.Properties(ctx, v.VirtualMachine.Reference(), []string{"config.guestFullName", "config.guestId"}, &vmConfig)
	if err != nil {
		return false, fmt.Errorf("failed to retrieve VM properties: %w", err)
	}
	logger.Log.Infof("Guest Full name: %v", vmConfig.Config.GuestFullName)
	if strings.Contains(strings.ToLower(vmConfig.Config.GuestFullName), "windows") ||
		strings.Contains(strings.ToLower(vmConfig.Config.GuestFullName), "microsoft") {
		return true, nil
	}
	logger.Log.Infof("Guest ID: %v", vmConfig.Config.GuestId)
	if strings.Contains(strings.ToLower(vmConfig.Config.GuestId), "windows") ||
		strings.Contains(strings.ToLower(vmConfig.Config.GuestId), "microsoft") {
		return true, nil
	}

	logger.Log.Infof("No Windows OS found in Guest Full name or ID strings...")
	return false, nil
}

func (v *VddkConfig) IsRhelCentosFamily(ctx context.Context) (bool, error) {
	var vmConfig mo.VirtualMachine
	err := v.VirtualMachine.Properties(ctx, v.VirtualMachine.Reference(), []string{"config.guestFullName", "config.guestId"}, &vmConfig)
	if err != nil {
		return false, fmt.Errorf("failed to retrieve VM properties: %w", err)
	}

	guestFullName := strings.ToLower(vmConfig.Config.GuestFullName)
	guestId := strings.ToLower(vmConfig.Config.GuestId)

	logger.Log.Infof("Guest Full name: %v", vmConfig.Config.GuestFullName)
	logger.Log.Infof("Guest ID: %v", vmConfig.Config.GuestId)

	if strings.Contains(guestFullName, "red hat") || strings.Contains(guestFullName, "centos") || strings.Contains(guestFullName, "rhel") ||
		strings.Contains(guestId, "rhel") || strings.Contains(guestId, "centos") {
		if strings.Contains(guestFullName, "8") || strings.Contains(guestFullName, "9") ||
			strings.Contains(guestId, "8") || strings.Contains(guestId, "9") {
			logger.Log.Infof("Detected RHEL/CentOS family (8 or newer).")
			return true, nil
		}
		logger.Log.Infof("Detected RHEL/CentOS family but version is not 8 or newer.")
		return false, nil
	}

	logger.Log.Infof("No RHEL/CentOS family detected in Guest Full name or ID.")
	return false, nil
}

func (v *VddkConfig) IsLinuxFamily(ctx context.Context) (bool, error) {
	var vmConfig mo.VirtualMachine
	err := v.VirtualMachine.Properties(ctx, v.VirtualMachine.Reference(), []string{"config.guestFullName", "config.guestId"}, &vmConfig)
	if err != nil {
		return false, fmt.Errorf("failed to retrieve VM properties: %w", err)
	}

	guestFullName := strings.ToLower(vmConfig.Config.GuestFullName)
	guestId := strings.ToLower(vmConfig.Config.GuestId)

	logger.Log.Infof("Guest Full name: %v", vmConfig.Config.GuestFullName)
	logger.Log.Infof("Guest ID: %v", vmConfig.Config.GuestId)

	if strings.Contains(guestFullName, "linux") || strings.Contains(guestId, "linux") {
		logger.Log.Infof("Detected Linux family.")
		return true, nil
	}

	logger.Log.Infof("No Linux OS detected in Guest Full name or ID.")
	return false, nil
}

func (v *VddkConfig) PowerOffVM(ctx context.Context) error {
	powerState, err := v.VirtualMachine.PowerState(ctx)
	if err != nil {
		return err
	}
	if powerState == types.VirtualMachinePowerStatePoweredOff {
		logger.Log.Infof("VM is already off, skipping...")
		return nil
	} else {
		logger.Log.Infof("Shutting down the VM...")
		err = v.VirtualMachine.ShutdownGuest(ctx)
		if err != nil {
			return err
		}
		err = v.VirtualMachine.WaitForPowerState(ctx, types.VirtualMachinePowerStatePoweredOff)
		if err != nil {
			return err
		}
	}
	return nil
}

func (v *VddkConfig) CreateSnapshot(ctx context.Context) error {
	task, err := v.VirtualMachine.CreateSnapshot(ctx, "osm-snap", "OS Migrate snapshot.", false, false)
	if err != nil {
		logger.Log.Infof("Failed to create snapshot: %v", err)
		return err
	}
	info, err := task.WaitForResult(ctx)
	if err != nil {
		logger.Log.Infof("Timeout to create snapshot: %v", err)
		return err
	}

	v.SnapshotReference = info.Result.(types.ManagedObjectReference)
	logger.Log.Infof("Snapshot created: %s", v.SnapshotReference.Value)
	return nil
}

func (v *VddkConfig) RemoveSnapshot(ctx context.Context) error {
	consolidate := true
	task, err := v.VirtualMachine.RemoveSnapshot(ctx, v.SnapshotReference.Value, false, &consolidate)
	if err != nil {
		logger.Log.Infof("Failed to remove snapshot: %v", err)
		return err
	}
	_, err = task.WaitForResult(ctx)
	if err != nil {
		logger.Log.Infof("Timeout to remove snapshot: %v", err)
		return err
	}
	logger.Log.Infof("Snapshot removed: %s", v.SnapshotReference.Value)
	return nil
}

func GetDiskKeys(ctx context.Context, v *object.VirtualMachine) ([]int32, error) {
	var diskKeys []int32
	var vm mo.VirtualMachine
	err := v.Properties(ctx, v.Reference(), []string{"config.hardware.device"}, &vm)
	if err != nil {
		logger.Log.Infof("Failed to retrieve VM properties: %v", err)
		return nil, err
	}
	for _, device := range vm.Config.Hardware.Device {
		if virtualDisk, ok := device.(*types.VirtualDisk); ok {
			diskKeys = append(diskKeys, virtualDisk.VirtualDevice.Key)
		}
	}
	return diskKeys, nil
}

func (v *VddkConfig) GetDiskKey(ctx context.Context) (int32, error) {
	var diskKeys int32
	var vm mo.VirtualMachine
	err := v.VirtualMachine.Properties(ctx, v.VirtualMachine.Reference(), []string{"config.hardware.device"}, &vm)
	if err != nil {
		logger.Log.Infof("Failed to retrieve VM properties: %v", err)
		return -1, err
	}
	for _, device := range vm.Config.Hardware.Device {
		if virtualDisk, ok := device.(*types.VirtualDisk); ok {
			if virtualDisk.VirtualDevice.Key == v.DiskKey {
				diskKeys = virtualDisk.VirtualDevice.Key
			}
		}
	}
	return diskKeys, nil
}

func (v *VddkConfig) GetDiskSizes(ctx context.Context) (map[string]int64, error) {
	var conf mo.VirtualMachine
	err := v.VirtualMachine.Properties(ctx, v.VirtualMachine.Reference(), []string{"config.hardware.device"}, &conf)
	if err != nil {
		logger.Log.Infof("Failed to retrieve VM properties: %v", err)
		return nil, fmt.Errorf("failed to retrieve VM properties: %w", err)
	}

	diskSizes := make(map[string]int64)
	for _, device := range conf.Config.Hardware.Device {
		if disk, ok := device.(*types.VirtualDisk); ok {
			diskSizes[strconv.Itoa(int(disk.Key))] = disk.CapacityInKB
		}
	}

	return diskSizes, nil
}

func (v *VddkConfig) GetCBTChangeID(ctx context.Context) (string, error) {
	var conf mo.VirtualMachine
	err := v.VirtualMachine.Properties(ctx, v.VirtualMachine.Reference(), []string{"config"}, &conf)
	if err != nil {
		logger.Log.Infof("Failed to get VM config: %v", err)
		return "", err
	}
	// Check if CBT is enabled
	if conf.Config.ChangeTrackingEnabled == nil || !*conf.Config.ChangeTrackingEnabled {
		return "", nil
	} else {
		logger.Log.Infof("CBT is enabled")
	}
	var b mo.VirtualMachineSnapshot
	err = v.VirtualMachine.Properties(ctx, v.SnapshotReference.Reference(), []string{"config.hardware"}, &b)
	if err != nil {
		logger.Log.Infof("Failed to get Snapshot info for osm-snap: %v", err)
		return "", fmt.Errorf("Failed to get Snapshot info for osm-snap: %s", err)
	}

	var disk *types.VirtualDisk
	for _, device := range conf.Config.Hardware.Device {
		if d, ok := device.(*types.VirtualDisk); ok && d.Key == v.DiskKey {
			disk = d
			break
		}
	}
	var changeId *string
	for _, vd := range b.Config.Hardware.Device {
		d := vd.GetVirtualDevice()
		if d.Key != disk.Key {
			continue
		}
		if b, ok := d.Backing.(*types.VirtualDiskFlatVer2BackingInfo); ok {
			changeId = &b.ChangeId
			break
		}
		if b, ok := d.Backing.(*types.VirtualDiskSparseVer2BackingInfo); ok {
			changeId = &b.ChangeId
			break
		}
		if b, ok := d.Backing.(*types.VirtualDiskRawDiskMappingVer1BackingInfo); ok {
			changeId = &b.ChangeId
			break
		}
		if b, ok := d.Backing.(*types.VirtualDiskRawDiskVer2BackingInfo); ok {
			changeId = &b.ChangeId
			break
		}
		logger.Log.Infof("disk %d has backing info without .ChangeId: %t", disk.Key, d.Backing)
		return "", fmt.Errorf("disk %d has backing info without .ChangeId: %t", disk.Key, d.Backing)
	}
	if changeId == nil || *changeId == "" {
		logger.Log.Infof("CBT is not enabled on disk %d", disk.Key)
		return "", fmt.Errorf("CBT is not enabled on disk %d", disk.Key)
	}

	return *changeId, nil
}

func (v *VddkConfig) SyncChangedDiskData(ctx context.Context, targetPath, changeID, sock string) error {
	// Fetch VM configuration
	var vmConfig mo.VirtualMachine
	if err := v.VirtualMachine.Properties(ctx, v.VirtualMachine.Reference(), []string{"config.hardware.device"}, &vmConfig); err != nil {
		logger.Log.Infof("Failed to retrieve VM properties: %v", err)
		return fmt.Errorf("failed to retrieve VM properties: %w", err)
	}
	var diskKey int32
	var diskSize int64
	for _, device := range vmConfig.Config.Hardware.Device {
		if disk, ok := device.(*types.VirtualDisk); ok {
			if disk.Key == v.DiskKey {
				diskKey = disk.Key
				diskSize = disk.CapacityInBytes
				break
			}
		}
	}
	if diskSize == 0 {
		logger.Log.Infof("Failed to determine disk size or locate target disk")
		return fmt.Errorf("failed to determine disk size or locate target disk")
	}
	file, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_EXCL|syscall.O_DIRECT, 0644)
	if err != nil {
		logger.Log.Infof("Failed to open target file %s: %v", targetPath, err)
		return fmt.Errorf("failed to open target file %s: %w", targetPath, err)
	}
	defer file.Close()
	nbd, err := libnbd.Create()
	if err != nil {
		logger.Log.Infof("Failed to initialize NBD: %v", err)
		return fmt.Errorf("failed to initialize NBD: %w", err)
	}
	defer nbd.Close()

	if sock != "" {
		if err := nbd.ConnectUri(sock); err != nil {
			logger.Log.Infof("Failed to set export name: %v", err)
			return fmt.Errorf("failed to set export name: %w", err)
		}
	} else {
		if err := nbd.ConnectUri("nbd://localhost"); err != nil {
			logger.Log.Infof("Failed to connect to NBD server: %v", err)
			return fmt.Errorf("failed to connect to NBD server: %w", err)
		}
	}
	startOffset := int64(0)
	for {
		query := types.QueryChangedDiskAreas{
			This:        v.VirtualMachine.Reference(),
			Snapshot:    &v.SnapshotReference,
			DeviceKey:   diskKey,
			StartOffset: startOffset,
			ChangeId:    changeID,
		}
		result, err := methods.QueryChangedDiskAreas(ctx, v.VirtualMachine.Client(), &query)
		if err != nil {
			logger.Log.Infof("Failed to query changed disk areas: %v", err)
			return fmt.Errorf("failed to query changed disk areas: %w", err)
		}
		changedAreas := result.Returnval.ChangedArea
		if len(changedAreas) == 0 {
			break
		}
		for _, area := range changedAreas {
			offset := area.Start
			for offset < area.Start+area.Length {
				chunkSize := area.Length - (offset - area.Start)
				if chunkSize > maxChunkSize {
					chunkSize = maxChunkSize
				}
				// Read data from NBD
				buffer := make([]byte, chunkSize)
				if err := nbd.Pread(buffer, uint64(offset), nil); err != nil {
					logger.Log.Infof("Failed to read data from NBD: %v", err)
					return fmt.Errorf("failed to read data from NBD: %w", err)
				}
				if _, err := file.WriteAt(buffer, offset); err != nil {
					logger.Log.Infof("Failed to write data to target file: %v", err)
					return fmt.Errorf("failed to write data to target file: %w", err)
				}
				offset += chunkSize
			}
		}
		startOffset = result.Returnval.StartOffset + result.Returnval.Length
		if startOffset >= diskSize {
			break
		}
	}
	return nil
}
