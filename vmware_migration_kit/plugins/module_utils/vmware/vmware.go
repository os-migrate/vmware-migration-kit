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
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"libguestfs.org/libnbd"
)

type VddkConfig struct {
	VirtualMachine    *object.VirtualMachine
	SnapshotReference types.ManagedObjectReference
}

const maxChunkSize = 64 * 1024 * 1024

var logger *log.Logger
var logFile string = "/tmp/osm-nbdkit.log"

func init() {
	logFile, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	logger = log.New(logFile, "osm-nbdkit: ", log.LstdFlags|log.Lshortfile)
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
		logger.Printf("No certificates found")
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

func (v *VddkConfig) CreateSnapshot(ctx context.Context) error {
	task, err := v.VirtualMachine.CreateSnapshot(ctx, "osm-snap", "OS Migrate snapshot.", false, false)
	if err != nil {
		logger.Printf("Failed to create snapshot: %v", err)
		return err
	}
	info, err := task.WaitForResult(ctx)
	if err != nil {
		logger.Printf("Timeout to create snapshot: %v", err)
		return err
	}

	v.SnapshotReference = info.Result.(types.ManagedObjectReference)
	logger.Printf("Snapshot created: %s", v.SnapshotReference.Value)
	return nil
}

func (v *VddkConfig) RemoveSnapshot(ctx context.Context) error {
	consolidate := true
	task, err := v.VirtualMachine.RemoveSnapshot(ctx, v.SnapshotReference.Value, false, &consolidate)
	if err != nil {
		logger.Printf("Failed to remove snapshot: %v", err)
		return err
	}
	_, err = task.WaitForResult(ctx)
	if err != nil {
		logger.Printf("Timeout to remove snapshot: %v", err)
		return err
	}
	logger.Printf("Snapshot removed: %s", v.SnapshotReference.Value)
	return nil
}

func (v *VddkConfig) GetDiskKey(ctx context.Context) ([]int32, error) {
	var diskKeys []int32
	var vm mo.VirtualMachine
	err := v.VirtualMachine.Properties(ctx, v.VirtualMachine.Reference(), []string{"config.hardware.device"}, &vm)
	if err != nil {
		logger.Printf("Failed to retrieve VM properties: %v", err)
		return nil, err
	}
	for _, device := range vm.Config.Hardware.Device {
		if virtualDisk, ok := device.(*types.VirtualDisk); ok {
			diskKeys = append(diskKeys, virtualDisk.VirtualDevice.Key)
		}
	}
	return diskKeys, nil
}

func (v *VddkConfig) GetDiskSizes(ctx context.Context) (map[string]int64, error) {
	var conf mo.VirtualMachine
	err := v.VirtualMachine.Properties(ctx, v.VirtualMachine.Reference(), []string{"config.hardware.device"}, &conf)
	if err != nil {
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
		return "", err
	}
	// Check if CBT is enabled
	if conf.Config.ChangeTrackingEnabled == nil || !*conf.Config.ChangeTrackingEnabled {
		return "", nil
	} else {
		logger.Printf("CBT is enabled")
	}
	var b mo.VirtualMachineSnapshot
	err = v.VirtualMachine.Properties(ctx, v.SnapshotReference.Reference(), []string{"config.hardware"}, &b)
	if err != nil {
		return "", fmt.Errorf("Failed to get Snapshot info for osm-snap: %s", err)
	}

	var disk *types.VirtualDisk
	diskK, _ := v.GetDiskKey(ctx)
	for _, device := range conf.Config.Hardware.Device {
		if d, ok := device.(*types.VirtualDisk); ok && d.Key == diskK[0] {
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

		return "", fmt.Errorf("disk %d has backing info without .ChangeId: %t", disk.Key, d.Backing)
	}
	if changeId == nil || *changeId == "" {
		return "", fmt.Errorf("CBT is not enabled on disk %d", disk.Key)
	}

	return *changeId, nil
}

func (v *VddkConfig) SyncChangedDiskData(ctx context.Context, path string) error {
	var vmConfig mo.VirtualMachine
	if err := v.VirtualMachine.Properties(ctx, v.VirtualMachine.Reference(), []string{"config.hardware.device"}, &vmConfig); err != nil {
		return fmt.Errorf("failed to retrieve VM properties: %w", err)
	}

	// Get CBT ChangeID and Disk Key
	changeID, err := v.GetCBTChangeID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get CBT change ID: %w", err)
	}
	diskKey, err := v.GetDiskKey(ctx)
	if err != nil {
		return fmt.Errorf("failed to get disk key: %w", err)
	}

	// Determine Disk Size
	var diskSize int64
	for _, device := range vmConfig.Config.Hardware.Device {
		if disk, ok := device.(*types.VirtualDisk); ok {
			diskSize = disk.CapacityInBytes
			break
		}
	}
	if diskSize == 0 {
		return fmt.Errorf("failed to determine disk size")
	}

	// Open file for writing
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_EXCL|syscall.O_DIRECT, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()
	nbd, err := libnbd.Create()
	if err != nil {
		return err
	}
	defer nbd.Close()

	if err := nbd.ConnectUri("nbd://localhost"); err != nil {
		return fmt.Errorf("failed to connect to NBD server: %w", err)
	}

	startOffset := int64(0)
	for {
		// Query Changed Disk Areas
		query := types.QueryChangedDiskAreas{
			This:        v.VirtualMachine.Reference(),
			Snapshot:    &v.SnapshotReference,
			DeviceKey:   diskKey[0],
			StartOffset: startOffset,
			ChangeId:    changeID,
		}
		result, err := methods.QueryChangedDiskAreas(ctx, v.VirtualMachine.Client(), &query)
		if err != nil {
			return fmt.Errorf("failed to query changed disk areas: %w", err)
		}
		for _, area := range result.Returnval.ChangedArea {
			for offset := area.Start; offset < area.Start+area.Length; {
				chunkSize := area.Length - (offset - area.Start)
				if chunkSize > maxChunkSize {
					chunkSize = maxChunkSize
				}

				buf := make([]byte, chunkSize)
				if err := nbd.Pread(buf, uint64(offset), nil); err != nil {
					return fmt.Errorf("failed to read data from NBD: %w", err)
				}

				if _, err := file.WriteAt(buf, offset); err != nil {
					return fmt.Errorf("failed to write data to file: %w", err)
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
