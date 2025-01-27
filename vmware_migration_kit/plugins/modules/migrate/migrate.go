package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
	moduleutils "vmware-migration-kit/vmware_migration_kit/plugins/module_utils"
	"vmware-migration-kit/vmware_migration_kit/plugins/module_utils/ansible"
	"vmware-migration-kit/vmware_migration_kit/plugins/module_utils/nbdkit"
	osm_os "vmware-migration-kit/vmware_migration_kit/plugins/module_utils/openstack"
	"vmware-migration-kit/vmware_migration_kit/plugins/module_utils/vmware"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
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

type VddkConfig struct {
	VirtualMachine    *object.VirtualMachine
	SnapshotReference types.ManagedObjectReference
}

type MigrationConfig struct {
	User         string
	Password     string
	Server       string
	Libdir       string
	VmName       string
	OSMDataDir   string
	VddkConfig   *vmware.VddkConfig
	CBTSync      bool
	OSClient     *gophercloud.ProviderClient
	ConvHostName string
	Compression  string
	FirstBoot    string
}

type NbdkitServer struct {
	cmd *exec.Cmd
}

// Ansible
type ModuleArgs struct {
	DstCloud     osm_os.DstCloud `json:"dst_cloud"`
	User         string
	Password     string
	Server       string
	Libdir       string
	VmName       string
	VddkPath     string
	OSMDataDir   string
	CBTSync      bool
	ConvHostName string
	Compression  string
	FirstBoot    string
}

var logger *log.Logger
var logFile string = "/tmp/osm-nbdkit.log"

func init() {
	logFile, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	logger = log.New(logFile, "osm-nbdkit: ", log.LstdFlags|log.Lshortfile)
}

// Migration Cycle
func (c *MigrationConfig) RunNbdKit(diskName string) (*NbdkitServer, error) {
	thumbprint, err := vmware.GetThumbprint(c.Server, "443")
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(
		"nbdkit",
		"--readonly",
		"--exit-with-parent",
		"--foreground",
		"vddk",
		fmt.Sprintf("server=%s", c.Server),
		fmt.Sprintf("user=%s", c.User),
		fmt.Sprintf("password=%s", c.Password),
		fmt.Sprintf("thumbprint=%s", thumbprint),
		fmt.Sprintf("libdir=%s", c.Libdir),
		fmt.Sprintf("vm=moref=%s", c.VddkConfig.VirtualMachine.Reference().Value),
		fmt.Sprintf("snapshot=%s", c.VddkConfig.SnapshotReference.Value),
		fmt.Sprintf("compression=%s", c.Compression),
		"transports=file:nbdssl:nbd",
		diskName,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		logger.Printf("Failed to start nbdkit: %v", err)
		return nil, err
	}
	logger.Printf("nbdkit started...")
	logger.Printf("Command: %v", cmd)

	time.Sleep(100 * time.Millisecond)
	err = nbdkit.WaitForNbdkit("localhost", "10809", 30*time.Second)
	if err != nil {
		logger.Printf("Failed to wait for nbdkit: %v", err)
		if cmd.Process != nil {
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil, err
	}

	return &NbdkitServer{
		cmd: cmd,
	}, nil
}

func (s *NbdkitServer) Stop() error {
	if err := syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL); err != nil {
		logger.Printf("Failed to stop nbdkit server: %v", err)
		return fmt.Errorf("failed to stop nbdkit server: %w", err)
	}
	logger.Printf("Nbdkit server stopped.")
	return nil
}

func (c *MigrationConfig) VMMigration(ctx context.Context, runV2V bool) (string, error) {
	var syncVol bool = false
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Create or update volume.
	vmName, err := c.VddkConfig.VirtualMachine.ObjectName(ctx)
	if err != nil {
		logger.Printf("Failed to get VM name: %v", err)
		return "", err
	}
	diskSize, err := c.VddkConfig.GetDiskSizes(ctx)
	if err != nil {
		logger.Printf("Failed to get disks key: %v", err)
		return "", err
	}
	diskNameStr := strconv.Itoa(int(c.VddkConfig.DiskKey))
	volume, err := osm_os.GetVolumeID(c.OSClient, vmName, diskNameStr)
	if err != nil {
		logger.Printf("Failed to get volume: %v", err)
		return "", err
	}
	if volume != nil {
		if c.CBTSync {
			logger.Printf("Volume exists, syncing volume..")
			syncVol = true
		} else {
			logger.Printf("Volume already exists and CBT sync is disabled, skipping migration..")
			return volume.ID, fmt.Errorf("volume already exists")
		}
	}
	isWin, err := c.VddkConfig.IsWindowsFamily(ctx)
	if err != nil {
		return "", err
	}
	// If syncVol is true, it means that CBT is enable and the VM should be shutting down before
	// running V2V conversion
	// Also, shutdown if OS is Windows, (@TODO) otherwise make it optional
	if syncVol || isWin {
		err = c.VddkConfig.PowerOffVM(ctx)
		if err != nil {
			logger.Printf("Failed to power off vm %v", err)
			return "", err
		}
	}
	// Create snapshot
	err = c.VddkConfig.CreateSnapshot(ctx)
	if err != nil {
		return "", err
	}
	defer c.VddkConfig.RemoveSnapshot(ctx)
	var snapshot mo.VirtualMachineSnapshot
	err = c.VddkConfig.VirtualMachine.Properties(ctx, c.VddkConfig.SnapshotReference, []string{"config.hardware"}, &snapshot)
	if err != nil {
		return "", err
	}

	var volMetadata map[string]string
	if volume == nil && err == nil {
		if c.CBTSync {
			runV2V = false
		}
		if changeID, _ := c.VddkConfig.GetCBTChangeID(ctx); changeID != "" {
			logger.Printf("CBT enabled, creating new volume and set changeID: %s", changeID)
			volMetadata = map[string]string{
				"osm":      "true",
				"changeID": changeID,
			}
		} else {
			logger.Printf("Volume not found, creating new volume")
			volMetadata = map[string]string{
				"osm": "true",
			}
		}
		// TODO:
		// remove hardcoded BuSType:
		// if busType == "scsi" {
		// 	volumeMetadata["hw_disk_bus"] = "scsi"
		// 	volumeMetadata["hw_scsi_model"] = "virtio-scsi"
		// }
		volOpts := osm_os.VolOpts{
			Name:       vmName + "-" + diskNameStr,
			Size:       int(diskSize[diskNameStr] / 1024 / 1024),
			VolumeType: "",
			BusType:    "virtio",
			Metadata:   volMetadata,
		}
		var fw mo.VirtualMachine
		var uefi bool
		uefi = false
		err = c.VddkConfig.VirtualMachine.Properties(ctx, c.VddkConfig.VirtualMachine.Reference(), []string{"config.firmware"}, &fw)
		if err != nil {
			return "", err
		}
		if types.GuestOsDescriptorFirmwareType(fw.Config.Firmware) == types.GuestOsDescriptorFirmwareTypeEfi {
			logger.Printf("UEFI firmware detected")
			uefi = true
		}
		volume, err = osm_os.CreateVolume(c.OSClient, volOpts, uefi)
		if err != nil {
			logger.Printf("Failed to create volume: %v", err)
			return "", err
		}
	}
	// Attach volume
	instanceUUID, _ := osm_os.GetInstanceUUID()
	err = osm_os.AttachVolume(c.OSClient, volume.ID, c.ConvHostName, instanceUUID)
	if err != nil {
		logger.Printf("Failed to attach volume: %v", err)
		return "", err
	}
	defer osm_os.DetachVolume(c.OSClient, volume.ID, "vmware-conv-host", "")
	devPath, err := moduleutils.FindDevName(volume.ID)
	if err != nil {
		logger.Printf("Failed to find device name: %v", err)
		return "", err
	}

	// Start copy
	for _, device := range snapshot.Config.Hardware.Device {
		switch disk := device.(type) {
		case *types.VirtualDisk:
			if device.GetVirtualDevice().Key != c.VddkConfig.DiskKey {
				break
			}
			backing := disk.Backing.(types.BaseVirtualDeviceFileBackingInfo)
			info := backing.GetVirtualDeviceFileBackingInfo()

			nbdSrv, err := c.RunNbdKit(info.FileName)
			defer nbdSrv.Stop()
			if err != nil {
				logger.Printf("Failed to run nbdkit: %v", err)
				return "", err
			}
			if syncVol {
				// Check change id
				osChangeID, err := osm_os.GetOSChangeID(c.OSClient, volume.ID)
				if err != nil {
					logger.Printf("Failed to get OS change ID: %v", err)
					return "", err
				}
				vmChangeID, err := c.VddkConfig.GetCBTChangeID(ctx)
				if err != nil {
					logger.Printf("Failed to get VM change ID: %v", err)
					return "", err
				}
				logger.Printf("OS Change ID: %s, VM Change ID: %s", osChangeID, vmChangeID)
				if osChangeID != vmChangeID {
					logger.Printf("Change ID mismatch, syncing volume..")
					err = c.VddkConfig.SyncChangedDiskData(ctx, devPath, osChangeID)
					if err != nil {
						logger.Printf("Failed to sync volume: %v", err)
						return "", err
					}
					logger.Printf("Volume synced successfully")
				} else {
					logger.Printf("No change in VM, skipping volume sync")
				}
			} else {
				err = nbdkit.NbdCopy(devPath)
				if err != nil {
					logger.Printf("Failed to copy disk: %v", err)
					nbdSrv.Stop()
					return "", err
				}
			}
			if runV2V {
				logger.Printf("Running V2V conversion with %v", volume.ID)
				var netConfScript string
				if ok, _ := c.VddkConfig.IsRhelCentosFamily(ctx); ok {
					netConfScript = c.FirstBoot
				} else {
					netConfScript = ""
				}
				err = nbdkit.V2VConversion(devPath, netConfScript)
				if err != nil {
					logger.Printf("Failed to convert disk: %v", err)
					return "", err
				}
				err = c.VddkConfig.PowerOffVM(ctx)
				if err != nil {
					logger.Printf("Warning: Failed to power off vm %v", err)
					logger.Printf("You will have to power off the vm manually...")
				}
			} else {
				logger.Printf("Skipping V2V conversion...")
			}
		}
	}
	if devPath == "" {
		logger.Printf("No disk found")
		return "", errors.New("No disk found")
	}
	logger.Printf("Disk copied and converted successfully: %s", devPath)
	return volume.ID, nil
}

func main() {
	var response ansible.Response
	if len(os.Args) != 2 {
		response.Msg = "No argument file provided"
		ansible.FailJson(response)
	}

	argsFile := os.Args[1]
	text, err := ioutil.ReadFile(argsFile)
	if err != nil {
		response.Msg = "Could not read configuration file: " + argsFile
		ansible.FailJson(response)
	}

	var moduleArgs ModuleArgs
	err = json.Unmarshal(text, &moduleArgs)
	if err != nil {
		response.Msg = "Configuration file not valid JSON: " + argsFile
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
	compression := ansible.DefaultIfEmpty(moduleArgs.Compression, "skipz")
	firsBoot := ansible.DefaultIfEmpty(moduleArgs.FirstBoot, "/opt/os-migrate/network_config.sh")
	cbtsync := moduleArgs.CBTSync

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, err := vmware.VMWareAuth(ctx, server, user, password)
	if err != nil {
		logger.Printf("Failed to initiate Vmware client: %v", err)
		response.Msg = "Failed to initiate Vmware client: " + err.Error()
		ansible.FailJson(response)
	}

	provider, err := osm_os.OpenstackAuth(ctx, moduleArgs.DstCloud)
	if err != nil {
		logger.Printf("Failed to authenticate Openstack client: %v", err)
		response.Msg = "Failed to authenticate Openstack client: " + err.Error()
		ansible.FailJson(response)
	}

	vmpath := vddkpath + "/" + vmname
	finder := find.NewFinder(c.Client)
	vm, _ := finder.VirtualMachine(ctx, vmpath)
	var disks []int32
	var volume []string
	var runV2V = true
	disks, err = vmware.GetDiskKeys(ctx, vm)
	if err != nil {
		logger.Printf("Failed to get disks: %v", err)
		response.Msg = "Failed to get disks: " + err.Error() + ". Check logs: " + logFile
		ansible.FailJson(response)
	}
	for k, d := range disks {
		if k != 0 {
			runV2V = false
		}
		VMMigration := MigrationConfig{
			User:         user,
			Password:     password,
			Server:       server,
			Libdir:       libdir,
			VmName:       vmname,
			OSMDataDir:   osmdatadir,
			OSClient:     provider,
			CBTSync:      cbtsync,
			ConvHostName: convHostName,
			Compression:  compression,
			FirstBoot:    firsBoot,
			VddkConfig: &vmware.VddkConfig{
				VirtualMachine:    vm,
				SnapshotReference: types.ManagedObjectReference{},
				DiskKey:           d,
			},
		}
		volUUID, err := VMMigration.VMMigration(ctx, runV2V)
		if err != nil {
			logger.Printf("Failed to migrate VM: %v", err)
			response.Msg = "Failed to migrate VM: " + err.Error() + ". Check logs: " + logFile
			ansible.FailJson(response)
		}
		volume = append(volume, volUUID)
	}
	response.Changed = true
	response.Msg = "VM migrated successfully"
	response.ID = volume
	ansible.ExitJson(response)
}
