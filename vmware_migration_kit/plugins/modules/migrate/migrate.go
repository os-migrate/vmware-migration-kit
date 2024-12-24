package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	moduleutils "vmware-migration-kit/vmware_migration_kit/plugins/module_utils"
	"vmware-migration-kit/vmware_migration_kit/plugins/module_utils/ansible"
	"vmware-migration-kit/vmware_migration_kit/plugins/module_utils/nbdkit"
	osm_os "vmware-migration-kit/vmware_migration_kit/plugins/module_utils/openstack"
	"vmware-migration-kit/vmware_migration_kit/plugins/module_utils/vmware"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

/*
example:
{"user": "root", "password": "xxxx", "server": "10.9.113.7", "vmname": "ubuntu-2"}
*/

// type VolOpts struct {
// 	Name       string
// 	Size       int
// 	VolumeType string
// 	BusType    string
// 	Metadata   map[string]string
// }

type VddkConfig struct {
	VirtualMachine *object.VirtualMachine
	SnapshotRef    types.ManagedObjectReference
}

type MigrationConfig struct {
	User         string
	Password     string
	Server       string
	Libdir       string
	VmName       string
	OSMDataDir   string
	VddkConfig   *VddkConfig
	CBTSync      bool
	OSClient     *gophercloud.ProviderClient
	ConvHostName string
}

type NbdkitServer struct {
	cmd *exec.Cmd
}

// Ansible
type ModuleArgs struct {
	User         string
	Password     string
	Server       string
	Libdir       string
	VmName       string
	VddkPath     string
	OSMDataDir   string
	CBTSync      bool
	ConvHostName string
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

func (c *MigrationConfig) CreateSnapshot(ctx context.Context) error {
	logger.Printf("Creating snapshot for VM %s", c.VmName)
	task, err := c.VddkConfig.VirtualMachine.CreateSnapshot(ctx, "osm-snap", "OS Migrate snapshot.", false, false)
	if err != nil {
		logger.Printf("Failed to create snapshot: %v", err)
		return err
	}
	info, err := task.WaitForResult(ctx)
	if err != nil {
		logger.Printf("Timeout to create snapshot: %v", err)
		return err
	}

	c.VddkConfig.SnapshotRef = info.Result.(types.ManagedObjectReference)
	logger.Printf("Snapshot created: %s", c.VddkConfig.SnapshotRef.Value)
	return nil
}

func (c *MigrationConfig) RemoveSnapshot(ctx context.Context) error {
	logger.Printf("Removing snapshot for VM %s", c.VmName)
	consolidate := true
	task, err := c.VddkConfig.VirtualMachine.RemoveSnapshot(ctx, c.VddkConfig.SnapshotRef.Value, false, &consolidate)
	if err != nil {
		logger.Printf("Failed to remove snapshot: %v", err)
		return err
	}
	_, err = task.WaitForResult(ctx)
	if err != nil {
		logger.Printf("Timeout to remove snapshot: %v", err)
		return err
	}
	logger.Printf("Snapshot removed: %s", c.VddkConfig.SnapshotRef.Value)
	return nil
}

// Migration Cycle
func (c *MigrationConfig) RunNbdKit(diskName string) (*NbdkitServer, error) {
	thumbprint, err := vmware.GetThumbprint(c.Server, "443")
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(
		"nbdkit",
		"vddk",
		fmt.Sprintf("server=%s", c.Server),
		fmt.Sprintf("user=%s", c.User),
		fmt.Sprintf("password=%s", c.Password),
		fmt.Sprintf("thumbprint=%s", thumbprint),
		fmt.Sprintf("libdir=%s", c.Libdir),
		fmt.Sprintf("vm=moref=%s", c.VddkConfig.VirtualMachine.Reference().Value),
		fmt.Sprintf("snapshot=%s", c.VddkConfig.SnapshotRef.Value),
		"compression=zlib",
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
	return nil
}

func (c *MigrationConfig) VMMigration(ctx context.Context) (string, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Create or update volume.
	vmName, err := c.VddkConfig.VirtualMachine.ObjectName(ctx)
	if err != nil {
		logger.Printf("Failed to get VM name: %v", err)
		return "", err
	}

	diskName, err := vmware.GetDiskKey(ctx, c.VddkConfig.VirtualMachine)
	if err != nil {
		logger.Printf("Failed to get disk key: %v", err)
		return "", err
	}
	diskNameStr := strings.Trim(strings.Join(strings.Fields(fmt.Sprint(diskName)), ","), "[]")
	volume, err := osm_os.GetVolumeID(c.OSClient, vmName, diskNameStr)
	if err != nil {
		logger.Printf("Failed to get volume: %v", err)
		return "", err
	}
	if volume == nil && err == nil {
		logger.Printf("Volume not found, creating new volume")
		volMetadata := map[string]string{
			"osm": "true",
		}
		// TODO:
		// remove hardcoded BuSType:
		// if opts.BusType == "scsi" {
		// 	volumeMetadata["hw_disk_bus"] = "scsi"
		// 	volumeMetadata["hw_scsi_model"] = "virtio-scsi"
		// }
		volOpts := osm_os.VolOpts{
			Name:       vmName + "-" + diskNameStr,
			Size:       20,
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
	if volume != nil {
		if c.CBTSync {
			// Sync volume
			logger.Printf("Syncing volume")

			var conf mo.VirtualMachine
			c.VddkConfig.VirtualMachine.Properties(ctx, c.VddkConfig.VirtualMachine.Reference(), []string{"config.hardware"}, &conf)
			if conf.Config.ChangeTrackingEnabled != nil && *conf.Config.ChangeTrackingEnabled {
				logger.Printf("CBT enabled")
				// Sync volume
				logger.Printf("Syncing volume... (not yet implemented)")
				logger.Printf("Volume synced")
			} else {
				logger.Printf("CBT not enabled")
				return "", nil
			}
		}
	}
	// Create snapshot
	err = c.CreateSnapshot(ctx)
	if err != nil {
		return "", err
	}
	defer c.RemoveSnapshot(ctx)
	var snapshot mo.VirtualMachineSnapshot
	err = c.VddkConfig.VirtualMachine.Properties(ctx, c.VddkConfig.SnapshotRef, []string{"config.hardware"}, &snapshot)
	if err != nil {
		return "", err
	}
	// Attach volume
	instanceUUID, _ := osm_os.GetInstanceUUID()
	err = osm_os.AttachVolume(c.OSClient, volume.ID, c.ConvHostName, instanceUUID)
	if err != nil {
		logger.Printf("Failed to attach volume: %v", err)
		return "", err
	}
	defer osm_os.DetachVolume(c.OSClient, volume.ID, "vmware-conv-host", "")

	// Start nbdkit
	devPath, err := moduleutils.FindDevName(volume.ID)
	if err != nil {
		logger.Printf("Failed to find device name: %v", err)
		return "", err
	}
	for _, device := range snapshot.Config.Hardware.Device {
		switch disk := device.(type) {
		case *types.VirtualDisk:
			backing := disk.Backing.(types.BaseVirtualDeviceFileBackingInfo)
			info := backing.GetVirtualDeviceFileBackingInfo()

			nbdSrv, err := c.RunNbdKit(info.FileName)
			if err != nil {
				logger.Printf("Failed to run nbdkit: %v", err)
				return "", err
			}
			err = nbdkit.NbdCopy(devPath)

			if err != nil {
				logger.Printf("Failed to copy disk: %v", err)
				nbdSrv.Stop()
				return "", err
			}
			nbdSrv.Stop()
			err = nbdkit.V2VConversion(c.OSMDataDir, devPath)
			nbdSrv.Stop()
			if err != nil {
				logger.Printf("Failed to convert disk: %v", err)
				return "", err
			}
		}
	}
	if devPath == "" {
		logger.Printf("No disk found")
		return "", errors.New("No disk found")
	}
	logger.Printf("Disk copied and converted successfully: %s", devPath)
	return devPath, nil
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
	var user string
	var password string
	var server string
	var vmname string
	// Default parameters
	var libdir string = "/usr/lib/vmware-vix-disklib"
	var vddkpath string = "/ha-datacenter/vm/"
	var osmdatadir string = "/tmp/"
	var convHostName string = ""
	// Use CBT for incremental sync
	var cbtsync bool = false

	if moduleArgs.User != "" {
		user = moduleArgs.User
	} else {
		response.Msg = "User is required"
		ansible.FailJson(response)
	}
	if moduleArgs.Password != "" {
		password = moduleArgs.Password
	} else {
		response.Msg = "Password is required"
		ansible.FailJson(response)
	}
	if moduleArgs.Server != "" {
		server = moduleArgs.Server
	} else {
		response.Msg = "Server is required"
		ansible.FailJson(response)
	}
	if moduleArgs.VmName != "" {
		vmname = moduleArgs.VmName
	} else {
		response.Msg = "VM name is required"
		ansible.FailJson(response)
	}
	if moduleArgs.VddkPath != "" {
		vddkpath = moduleArgs.VddkPath
	}
	if moduleArgs.OSMDataDir != "" {
		osmdatadir = moduleArgs.OSMDataDir
	}

	if moduleArgs.Libdir != "" {
		libdir = moduleArgs.Libdir
	}
	if moduleArgs.CBTSync {
		cbtsync = moduleArgs.CBTSync
	}
	if moduleArgs.ConvHostName != "" {
		convHostName = moduleArgs.ConvHostName
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	u, _ := url.Parse("https://" + server + "/sdk")
	vmware.ProcessUrl(u, user, password)
	c, err := govmomi.NewClient(ctx, u, true)
	if err != nil {
		logger.Printf("Failed to initiate Vmware client: %v", err)
		response.Msg = "Failed to initiate Vmware client: " + err.Error()
		ansible.FailJson(response)
	}

	// Connect to OpenStack
	opts, err := openstack.AuthOptionsFromEnv()
	if err != nil {
		response.Msg = fmt.Sprintf("Failed to get auth options: %v", err)
		ansible.FailJson(response)
	}
	provider, err := openstack.NewClient(opts.IdentityEndpoint)
	if err != nil {
		response.Msg = fmt.Sprintf("Failed to authenticate: %v", err)
		ansible.FailJson(response)
	}
	provider.HTTPClient.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	err = openstack.Authenticate(context.TODO(), provider, opts)
	if err != nil {
		response.Msg = fmt.Sprintf("Failed to get auth options: %v", err)
		ansible.FailJson(response)
	}

	vmpath := vddkpath + "/" + vmname
	finder := find.NewFinder(c.Client)
	vm, _ := finder.VirtualMachine(ctx, vmpath)
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
		VddkConfig: &VddkConfig{
			VirtualMachine: vm,
			SnapshotRef:    types.ManagedObjectReference{},
		},
	}

	disk, err := VMMigration.VMMigration(ctx)
	if err != nil {
		logger.Printf("Failed to migrate VM: %v", err)
		response.Msg = "Failed to migrate VM: " + err.Error() + ". Check logs: " + logFile
		ansible.FailJson(response)
	}
	response.Changed = true
	response.Msg = "VM migrated successfully"
	response.ID = disk
	ansible.ExitJson(response)
}
