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
 * Copyright 2025 Red Hat, Inc.
 *
 */
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	moduleutils "vmware-migration-kit/plugins/module_utils"
	"vmware-migration-kit/plugins/module_utils/ansible"
	connectivity "vmware-migration-kit/plugins/module_utils/connectivity"
	"vmware-migration-kit/plugins/module_utils/logger"
	"vmware-migration-kit/plugins/module_utils/nbdkit"
	osm_os "vmware-migration-kit/plugins/module_utils/openstack"
	"vmware-migration-kit/plugins/module_utils/vmware"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

/*
example for args.json file:
{
		"user": "root",
		"password": "root",
		"server": "10.0.0.7",
		"vmname": "rhel-9.4-3",
		"cbtsync": false,
		"dst_cloud": {
			"auth": {
				"auth_url": "https://keystone-public-openstack.apps.ocp-4-16.standalone",
				"username": "admin",
				"project_id": "xyz",
				"project_name": "admin",
				"user_domain_name": "Default",
				"password": "admin"
			},
			"region_name": "regionOne",
			"interface": "public",
			"identity_api_version": 3
		}
}
*/

type MigrationConfig struct {
	NbdkitConfig       *nbdkit.NbdkitConfig
	User               string
	Password           string
	Server             string
	Libdir             string
	VmName             string
	VolumeAz           string
	VolumeType         string
	AssumeZero         bool
	OSMDataDir         string
	VddkConfig         *vmware.VddkConfig
	CBTSync            bool
	CutOver            bool
	OSClient           *gophercloud.ProviderClient
	ConvHostName       string
	Compression        string
	RunScript          string
	InstanceUUID       string
	Debug              bool
	CloudOpts          osm_os.DstCloud
	LocalDiskPath      string
	ManageExtVol       bool
	BootScript         string
	ExtraOpts          string
	CinderManageConfig *osm_os.CinderManageConfig
}

// Datastore name to Volume type mapping
type VolumeTypeMapping struct {
	DatastoreName string `json:"vmware_datastore"`
	VolumeType string `json:"openstack_type"`
}

// Ansible
type ModuleArgs struct {
	DstCloud       osm_os.DstCloud `json:"dst_cloud"`
	User           string
	Password       string
	Server         string
	Libdir         string
	VmName         string
	VolumeAz       string
	VolumeType     string
	VolumeTypeMapping []VolumeTypeMapping `json:"volume_type_mapping"`
	AssumeZero     bool
	VddkPath       string
	OSMDataDir     string
	CBTSync        bool
	CutOver        bool
	SkipConversion bool
	ConvHostName   string
	Compression    string
	RunScript      string
	UseSocks       bool
	InstanceUUID   string
	Debug          bool
	LocalDiskPath  string
	ExternalVolume bool
	VolumeName     string
	HostPool       string
	BootScript     string
	ExtraOpts      string
}

func (c *MigrationConfig) VMMigration(parentCtx context.Context, runV2V bool) (string, error) {
	syncVol := false
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create or update volume.
	vmName, err := c.NbdkitConfig.VddkConfig.VirtualMachine.ObjectName(ctx)
	if err != nil {
		logger.Log.Infof("Failed to get VM name: %v", err)
		return "", err
	}
	diskSize, err := c.NbdkitConfig.VddkConfig.GetDiskSizes(ctx)
	if err != nil {
		logger.Log.Infof("Failed to get disks key: %v", err)
		return "", err
	}
	diskNameStr := strconv.Itoa(int(c.NbdkitConfig.VddkConfig.DiskKey))
	volume, err := osm_os.GetVolumeID(c.OSClient, vmName, diskNameStr)
	if err != nil {
		logger.Log.Infof("Failed to get volume: %v", err)
		return "", err
	}
	if volume != nil {
		converted, err := osm_os.IsVolumeConverted(c.OSClient, volume.ID)
		if err != nil {
			logger.Log.Infof("Failed to get volume metadata: %v", err)
			return "", err
		}
		if converted {
			logger.Log.Infof("Volume already converted, skipping migration..")
			return volume.ID, nil
		}
		if c.CBTSync {
			logger.Log.Infof("Volume exists, syncing volume..")
			syncVol = true
		} else {
			logger.Log.Infof("Volume already exists and CBT sync is disabled, skipping migration..")
			return volume.ID, fmt.Errorf("volume already exists")
		}
	}
	isWin, err := c.NbdkitConfig.VddkConfig.IsWindowsFamily(ctx)
	if err != nil {
		return "", err
	}
	// If CBTSync is enabled and CutOver is false, skip V2V conversion
	if c.CBTSync && !c.CutOver {
		runV2V = false
	}
	// We need to shutdown the Windows VM before taking the snapshot when we have to run V2V
	if runV2V && isWin {
		err = c.NbdkitConfig.VddkConfig.PowerOffVM(ctx)
		if err != nil {
			logger.Log.Infof("Failed to power off vm %v", err)
			return "", err
		}
	}
	// Create snapshot
	err = c.NbdkitConfig.VddkConfig.CreateSnapshot(ctx)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := c.NbdkitConfig.VddkConfig.RemoveSnapshot(ctx); err != nil {
			logger.Log.Infof("Failed to remove snapshot: %v", err)
		}
	}()

	var snapshot mo.VirtualMachineSnapshot
	err = c.NbdkitConfig.VddkConfig.VirtualMachine.Properties(ctx, c.NbdkitConfig.VddkConfig.SnapshotReference, []string{"config.hardware"}, &snapshot)
	if err != nil {
		return "", err
	}

	var volMetadata map[string]string
	if volume == nil && err == nil {
		if changeID, _ := c.NbdkitConfig.VddkConfig.GetCBTChangeID(ctx); changeID != "" {
			logger.Log.Infof("CBT enabled, creating new volume and set changeID: %s", changeID)
			volMetadata = map[string]string{
				"osm":       "true",
				"changeID":  changeID,
				"converted": "false",
			}
		} else {
			logger.Log.Infof("Volume not found, creating new volume")
			volMetadata = map[string]string{
				"osm":       "true",
				"converted": "false",
			}
		}
		// TODO:
		// remove hardcoded BuSType:
		// if busType == "scsi" {
		// 	volumeMetadata["hw_disk_bus"] = "scsi"
		// 	volumeMetadata["hw_scsi_model"] = "virtio-scsi"
		// }
		volOpts := osm_os.VolOpts{
			Name:             vmName + "-" + diskNameStr,
			Size:             int(diskSize[diskNameStr] / 1024 / 1024),
			VolumeType:       c.VolumeType,
			AvailabilityZone: c.VolumeAz,
			BusType:          "virtio",
			Metadata:         volMetadata,
		}
		var fw mo.VirtualMachine
		var uefi bool
		uefi = false
		err = c.NbdkitConfig.VddkConfig.VirtualMachine.Properties(ctx, c.NbdkitConfig.VddkConfig.VirtualMachine.Reference(), []string{"config.firmware"}, &fw)
		if err != nil {
			return "", err
		}
		if types.GuestOsDescriptorFirmwareType(fw.Config.Firmware) == types.GuestOsDescriptorFirmwareTypeEfi {
			logger.Log.Infof("UEFI firmware detected")
			uefi = true
		}
		if c.ManageExtVol {
			logger.Log.Infof("Managing existing external volume: %s on host: %s", c.CinderManageConfig.VolumeName, c.CinderManageConfig.HostPool)
			volume, err = osm_os.CinderManage(c.OSClient, c.CinderManageConfig.VolumeName, c.CinderManageConfig.HostPool)
			if err != nil {
				logger.Log.Infof("Failed to manage existing external volume: %v", err)
				return "", err
			}
			// Get volume (need to return volume object)
		} else {
			logger.Log.Infof("Creating new volume..")
			// Create volume
			volume, err = osm_os.CreateVolume(c.OSClient, volOpts, uefi)
			if err != nil {
				logger.Log.Infof("Failed to create volume: %v", err)
				return "", err
			}
		}
	}
	// Attach volume
	instanceUUID, err := osm_os.GetInstanceUUID()
	if err != nil || instanceUUID == "" {
		logger.Log.Infof("Failed to get instance UUID: %v", err)
		logger.Log.Warnf("Instance metadata service is not working, please fix it..")
		logger.Log.Warnf("You can workaround this OpenStack error by providing the instance UUID of the conversion host,")
		logger.Log.Warnf("directly with the option `instanceuuid`.")
		if c.InstanceUUID != "" {
			instanceUUID = c.InstanceUUID
		} else {
			logger.Log.Infof("Unable to get instance UUID, please provide it manually..")
			return "", err
		}
	}
	err = osm_os.AttachVolume(c.OSClient, volume.ID, c.ConvHostName, instanceUUID)
	if err != nil {
		logger.Log.Infof("Failed to attach volume: %v", err)
		return "", err
	}
	// TODO: remove instanceName or handle it properly
	defer func() {
		if err := osm_os.DetachVolume(c.OSClient, volume.ID, "", instanceUUID, c.CloudOpts); err != nil {
			logger.Log.Infof("Failed to detach volume: %v", err)
		}
	}()

	devPath, err := moduleutils.FindDevName(volume.ID)
	if err != nil {
		logger.Log.Infof("Failed to find device name: %v", err)
		return "", err
	}
	// Start copy
	for _, device := range snapshot.Config.Hardware.Device {
		switch disk := device.(type) {
		case *types.VirtualDisk:
			if device.GetVirtualDevice().Key != c.NbdkitConfig.VddkConfig.DiskKey {
				break
			}
			backing := disk.Backing.(types.BaseVirtualDeviceFileBackingInfo)
			info := backing.GetVirtualDeviceFileBackingInfo()

			var nbdSrv *nbdkit.NbdkitServer
			if c.LocalDiskPath != "" {
				logger.Log.Infof("Using local disk path: %s", c.LocalDiskPath)
				nbdSrv, err = c.NbdkitConfig.RunNbdKitFromLocal(
					strings.TrimSpace(strings.SplitN(info.FileName, " ", 2)[1]), c.LocalDiskPath)
			} else {
				nbdSrv, err = c.NbdkitConfig.RunNbdKit(info.FileName)
			}
			sock := nbdSrv.GetSocketPath()
			defer func() {
				if err := nbdSrv.Stop(); err != nil {
					logger.Log.Infof("Failed to stop NBD server: %v", err)
				}
			}()

			if err != nil {
				logger.Log.Infof("Failed to run nbdkit: %v", err)
				return "", err
			}

			if syncVol {
				// Check change id
				osChangeID, err := osm_os.GetOSChangeID(c.OSClient, volume.ID)
				if err != nil {
					logger.Log.Infof("Failed to get OS change ID: %v", err)
					return "", err
				}
				vmChangeID, err := c.NbdkitConfig.VddkConfig.GetCBTChangeID(ctx)
				if err != nil {
					logger.Log.Infof("Failed to get VM change ID: %v", err)
					return "", err
				}
				logger.Log.Infof("OS Change ID: %s, VM Change ID: %s", osChangeID, vmChangeID)
				if osChangeID != vmChangeID {
					logger.Log.Infof("Change ID mismatch, syncing volume..")
					err = c.NbdkitConfig.VddkConfig.SyncChangedDiskData(ctx, devPath, osChangeID, sock)
					if err != nil {
						logger.Log.Infof("Failed to sync volume: %v", err)
						return "", err
					}
					logger.Log.Infof("Volume synced successfully")
				} else {
					logger.Log.Infof("No change in VM, skipping volume sync")
				}
			} else {
				err = nbdkit.NbdCopy(sock, devPath, c.AssumeZero)
				if err != nil {
					logger.Log.Infof("Failed to copy disk: %v", err)
					if err := nbdSrv.Stop(); err != nil {
						logger.Log.Infof("Failed to stop NBD server during error handling: %v", err)
					}
					return "", err
				}
			}
			if runV2V {
				logger.Log.Infof("Running V2V conversion with %v", volume.ID)
				var netConfScript string
				if ok, _ := c.NbdkitConfig.VddkConfig.IsLinuxFamily(ctx); ok && c.RunScript != "" {
					netConfScript = c.RunScript
				} else {
					netConfScript = ""
				}
				err = nbdkit.V2VConversion(devPath, netConfScript, c.BootScript, c.ExtraOpts, c.Debug)
				if err != nil {
					logger.Log.Infof("Failed to convert disk: %v", err)
					return "V2VFail", err
				}
				err = c.NbdkitConfig.VddkConfig.PowerOffVM(ctx)
				if err != nil {
					logger.Log.Infof("Warning: Failed to power off vm %v", err)
					logger.Log.Infof("You will have to power off the vm manually...")
				}
				volMetadata = map[string]string{
					"osm":       "true",
					"converted": "true",
				}
				err = osm_os.UpdateVolumeMetadata(c.OSClient, volume.ID, volMetadata)
				if err != nil {
					logger.Log.Infof("Failed to set volume metadata: %v, ignoring ...", err)
				}
			} else {
				logger.Log.Infof("Skipping V2V conversion...")
			}
		}
	}
	if devPath == "" {
		logger.Log.Infof("No disk found")
		return "", errors.New("no disk found")
	}
	logger.Log.Infof("Disk copied and converted successfully: %s", devPath)
	return volume.ID, nil
}

func main() {
	var response ansible.Response
	if len(os.Args) != 2 {
		response.Msg = "No argument file provided"
		ansible.FailJson(response)
	}

	argsFile := os.Args[1]
	text, err := os.ReadFile(argsFile)
	if err != nil {
		response.Msg = "Could not read configuration file: " + argsFile + ", error: " + err.Error()
		ansible.FailJson(response)
	}

	var moduleArgs ModuleArgs
	err = json.Unmarshal(text, &moduleArgs)
	if err != nil {
		response.Msg = "Configuration file not valid JSON: " + argsFile + ", error: " + err.Error()
		ansible.FailJson(response)
	}
	// Set parameters
	user := ansible.RequireField(moduleArgs.User, "User is required")
	password := ansible.RequireField(moduleArgs.Password, "Password is required")
	server := ansible.RequireField(moduleArgs.Server, "Server is required")
	vmname := ansible.RequireField(moduleArgs.VmName, "VM name is required")
	libdir := ansible.DefaultIfEmpty(moduleArgs.Libdir, "/usr/lib/vmware-vix-disklib")
	vddkpath := ansible.DefaultIfEmpty(moduleArgs.VddkPath, "/ha-datacenter/vm/")
	osmdatadir := ansible.DefaultIfEmpty(moduleArgs.OSMDataDir, "/tmp/")
	convHostName := ansible.DefaultIfEmpty(moduleArgs.ConvHostName, "")
	compression := ansible.DefaultIfEmpty(moduleArgs.Compression, "fastlz")
	runScript := ansible.DefaultIfEmpty(moduleArgs.RunScript, "")
	bootScript := ansible.DefaultIfEmpty(moduleArgs.BootScript, "")
	extraOpts := ansible.DefaultIfEmpty(moduleArgs.ExtraOpts, "")
	volAz := ansible.DefaultIfEmpty(moduleArgs.VolumeAz, "")
	volType := ansible.DefaultIfEmpty(moduleArgs.VolumeType, "")
	assumeZero := moduleArgs.AssumeZero
	cbtsync := moduleArgs.CBTSync
	cutover := moduleArgs.CutOver
	skipV2V := moduleArgs.SkipConversion
	socks := moduleArgs.UseSocks
	instanceUUid := moduleArgs.InstanceUUID
	debug := moduleArgs.Debug
	localDisk := moduleArgs.LocalDiskPath
	// Cinder manage options
	externalVolume := moduleArgs.ExternalVolume
	volumeName := moduleArgs.VolumeName
	hostPool := moduleArgs.HostPool

	// Handle logging
	r, err := moduleutils.GenRandom(8)
	if err != nil {
		response.Msg = "Failed to generate random string"
		ansible.FailJson(response)
	}
	safeVmName := moduleutils.SafeVmName(vmname)
	LogFile := "/tmp/osm-nbdkit-" + safeVmName + "-" + r + ".log"
	logger.InitLogger(LogFile)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, err := vmware.VMWareAuth(ctx, server, user, password)
	if err != nil {
		logger.Log.Infof("Failed to initiate Vmware client: %v", err)
		response.Msg = "Failed to initiate Vmware client: " + err.Error()
		ansible.FailJson(response)
	}

	provider, err := osm_os.OpenstackAuth(ctx, moduleArgs.DstCloud)
	if err != nil {
		logger.Log.Infof("Failed to authenticate Openstack client: %v", err)
		response.Msg = "Failed to authenticate Openstack client: " + err.Error()
		ansible.FailJson(response)
	}

	vmpath := vddkpath + "/" + vmname
	finder := find.NewFinder(c.Client)
	vm, err := connectivity.CheckVCenterConnectivity(ctx, finder, vmpath)
	if err != nil {
		logger.Log.Infof("Failed to check vCenter connectivity: %v", err)
		response.Msg = "Failed to check vCenter connectivity: " + err.Error()
		ansible.FailJson(response)
	}

	// Handle Volume type mapping
	var getVolumeTypeForDatastore = func(dstoreName string) string {
		volumeType := "__DEFAULT__"
		if volType != "" {
			volumeType = volType
		}
		for _, aMap := range moduleArgs.VolumeTypeMapping {
			if aMap.DatastoreName == dstoreName {
				volumeType = aMap.VolumeType
				logger.Log.Infof("Found match for datastore name and volume type. %s ---> %s", dstoreName, volumeType)
				break
			}
		}
		return volumeType
	}

	var disks []int32
	var volume []string
	var forceV2V = false
	runV2V := !skipV2V
	disks, err = vmware.GetDiskKeys(ctx, vm)
	if err != nil {
		logger.Log.Infof("Failed to get disks: %v", err)
		response.Msg = "Failed to get disks: " + err.Error() + ". Check logs: " + LogFile
		ansible.FailJson(response)
	}
	for k, d := range disks {
		if k != 0 && !forceV2V {
			runV2V = false
		} else if forceV2V {
			runV2V = true
		}
		dstoreName, err := vmware.GetDatastoreNameForDiskKey(ctx, vm, d)
		if err != nil {
			logger.Log.Infof("Could not find the datastore name for disk with key %d, using default volume type...", d)
		}
		volType = getVolumeTypeForDatastore(dstoreName)
		logger.Log.Infof("Selected volume type %s for disk with key %d as per volume mapping configured (datastore: %s)", volType, d, dstoreName)

		logger.Log.Infof("Migrating disk with key: %d", d)
		VMMigration := MigrationConfig{
			NbdkitConfig: &nbdkit.NbdkitConfig{
				User:        user,
				Password:    password,
				Server:      server,
				Libdir:      libdir,
				VmName:      vmname,
				Compression: compression,
				UUID:        r,
				UseSocks:    socks,
				VddkConfig: &vmware.VddkConfig{
					VirtualMachine:    vm,
					SnapshotReference: types.ManagedObjectReference{},
					DiskKey:           d,
				},
			},
			CinderManageConfig: &osm_os.CinderManageConfig{
				VolumeName: volumeName,
				HostPool:   hostPool,
			},
			ManageExtVol:  externalVolume,
			OSMDataDir:    osmdatadir,
			OSClient:      provider,
			CBTSync:       cbtsync,
			CutOver:       cutover,
			ConvHostName:  convHostName,
			Compression:   compression,
			RunScript:     runScript,
			BootScript:    bootScript,
			ExtraOpts:     extraOpts,
			InstanceUUID:  instanceUUid,
			Debug:         debug,
			LocalDiskPath: localDisk,
			CloudOpts:     moduleArgs.DstCloud,
			VolumeType:    volType,
			VolumeAz:      volAz,
			AssumeZero:    assumeZero,
		}
		volUUID, err := VMMigration.VMMigration(ctx, runV2V)
		if err != nil {
			if volUUID == "V2VFail" && len(disks) > 1 && k+1 < len(disks) {
				logger.Log.Infof("V2V conversion failed for disk %d, skipping to next disk...", d)
				forceV2V = true
				continue
			} else {
				logger.Log.Infof("Failed to migrate VM: %v", err)
				response.Msg = "Failed to migrate VM: " + err.Error() + ". Check logs: " + LogFile
				ansible.FailJson(response)
			}
		}
		volume = append(volume, volUUID)
	}
	response.Changed = true
	response.Msg = "VM migrated successfully"
	response.ID = volume
	ansible.ExitJson(response)
}
