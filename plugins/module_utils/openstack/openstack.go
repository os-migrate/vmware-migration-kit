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

package openstack

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
	"vmware-migration-kit/plugins/module_utils/logger"

	gophercloud "github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/volumes"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/volumeattach"
	"github.com/gophercloud/gophercloud/v2/openstack/config"
)

type DstCloud struct {
	Auth               `json:"auth"`
	RegionName         string `json:"region_name"`
	Interface          string `json:"interface"`
	IdentityAPIVersion int    `json:"identity_api_version"`
}

type Auth struct {
	AuthURL        string `json:"auth_url"`
	Username       string `json:"username"`
	ProjectID      string `json:"project_id"`
	ProjectName    string `json:"project_name"`
	UserDomainName string `json:"user_domain_name"`
	Password       string `json:"password"`
}

type VolOpts struct {
	Name       string
	Size       int
	VolumeType string
	BusType    string
	Metadata   map[string]string
}

type ServerArgs struct {
	Name           string
	Nics           []interface{}
	BootVolume     string
	Volumes        []string
	SecurityGroups []string
	Flavor         string
}

type CinderManageConfig struct {
	VolumeName string
	HostPool   string
}

func OpenstackAuth(ctx context.Context, moduleOpts DstCloud) (*gophercloud.ProviderClient, error) {
	var opts gophercloud.AuthOptions
	authURL := os.Getenv("OS_AUTH_URL")
	if authURL != "" {
		var err error
		opts, err = openstack.AuthOptionsFromEnv()
		if err != nil {
			return nil, err
		}
	} else {
		opts = gophercloud.AuthOptions{
			IdentityEndpoint: moduleOpts.AuthURL,
			Username:         moduleOpts.Username,
			Password:         moduleOpts.Password,
			TenantID:         moduleOpts.ProjectID,
			TenantName:       moduleOpts.ProjectName,
			DomainName:       moduleOpts.UserDomainName,
		}
	}
	provider, err := config.NewProviderClient(ctx, opts, config.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}))
	if err != nil {
		return nil, err
	}
	err = openstack.Authenticate(context.TODO(), provider, opts)
	if err != nil {
		return nil, err
	}
	return provider, nil
}

func CreateVolume(provider *gophercloud.ProviderClient, opts VolOpts, setUEFI bool) (*volumes.Volume, error) {
	client, err := openstack.NewBlockStorageV3(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		logger.Log.Infof("Failed to create block storage client: %v", err)
		return nil, err
	}

	createOpts := volumes.CreateOpts{
		Name:       opts.Name,
		Size:       opts.Size,
		VolumeType: opts.VolumeType,
		Metadata:   opts.Metadata,
	}

	volume, err := volumes.Create(context.TODO(), client, createOpts, nil).Extract()
	if err != nil {
		logger.Log.Infof("Failed to create volume: %v", err)
		return nil, err
	}

	err = WaitForVolumeStatus(client, volume.ID, "available", 3000)
	if err != nil {
		logger.Log.Infof("Failed to wait for volume to become available: %v", err)
		return nil, err
	}
	// Set bootable
	options := volumes.BootableOpts{
		Bootable: true,
	}
	err = volumes.SetBootable(context.TODO(), client, volume.ID, options).ExtractErr()
	if err != nil {
		logger.Log.Infof("Failed to set volume as bootable: %v", err)
		return nil, err
	}
	if setUEFI {
		// Set Image Metadata
		// If Guest OS firmware is UEFI, set hw_firmware_type to uefi
		ImageMetadataOpts := volumes.ImageMetadataOpts{
			Metadata: map[string]string{
				"hw_machine_type":  "q35",
				"hw_firmware_type": "uefi",
			},
		}
		err = volumes.SetImageMetadata(context.TODO(), client, volume.ID, ImageMetadataOpts).ExtractErr()
		if err != nil {
			logger.Log.Infof("Failed to set image metadata: %v", err)
			return nil, err
		}
	}
	return volume, nil
}

func WaitForVolumeStatus(client *gophercloud.ServiceClient, volumeID, status string, timeout int) error {
	for i := 0; i < timeout; i++ {
		volume, err := volumes.Get(context.TODO(), client, volumeID).Extract()
		if err != nil {
			logger.Log.Infof("Failed to get volume status: %v", err)
			return err
		}
		if volume.Status == status {
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	logger.Log.Infof("Volume %s did not reach status %s within the timeout", volumeID, status)
	return fmt.Errorf("volume %s did not reach status %s within the timeout", volumeID, status)
}

func UpdateVolumeMetadata(client *gophercloud.ProviderClient, volumeID string, metadata map[string]string) error {
	blockStorageClient, err := openstack.NewBlockStorageV3(client, gophercloud.EndpointOpts{})
	if err != nil {
		logger.Log.Infof("Failed to create block storage client: %v", err)
		return err
	}
	updateOpts := volumes.UpdateOpts{
		Metadata: metadata,
	}
	_, err = volumes.Update(context.TODO(), blockStorageClient, volumeID, updateOpts).Extract()
	if err != nil {
		logger.Log.Infof("Failed to update volume metadata: %v", err)
		return err
	}
	return nil
}

func IsVolumeConverted(client *gophercloud.ProviderClient, volumeID string) (bool, error) {
	blockStorageClient, err := openstack.NewBlockStorageV3(client, gophercloud.EndpointOpts{})
	if err != nil {
		logger.Log.Infof("Failed to create block storage client: %v", err)
		return false, err
	}
	volume, err := volumes.Get(context.TODO(), blockStorageClient, volumeID).Extract()
	if err != nil {
		logger.Log.Infof("Failed to get volume: %v", err)
		return false, err
	}
	if prop, ok := volume.Metadata["converted"]; ok {
		converted, err := strconv.ParseBool(prop)
		if err != nil {
			logger.Log.Infof("Failed to cast metadata to bool, make sure the converted property is bool: %v", err)
			return false, err
		}
		return converted, nil
	}
	return false, nil
}

func GetOSChangeID(client *gophercloud.ProviderClient, volumeID string) (string, error) {
	blockStorageClient, err := openstack.NewBlockStorageV3(client, gophercloud.EndpointOpts{})
	if err != nil {
		logger.Log.Infof("Failed to create block storage client: %v", err)
		return "", err
	}
	volume, err := volumes.Get(context.TODO(), blockStorageClient, volumeID).Extract()
	if err != nil {
		logger.Log.Infof("Failed to get volume: %v", err)
		return "", err
	}
	if changeID, ok := volume.Metadata["changeID"]; ok {
		return changeID, nil
	}
	return "", nil
}

func GetVolumeID(client *gophercloud.ProviderClient, vm string, disk string) (*volumes.Volume, error) {
	blockStorageClient, err := openstack.NewBlockStorageV3(client, gophercloud.EndpointOpts{})
	if err != nil {
		logger.Log.Infof("Failed to create block storage client: %v", err)
		return nil, err
	}

	volumeListOpts := volumes.ListOpts{
		Name: (vm + "-" + disk),
	}
	volumeListOpts.Metadata = map[string]string{
		"osm": "true",
	}

	pages, err := volumes.List(blockStorageClient, volumeListOpts).AllPages(context.TODO())
	if err != nil {
		logger.Log.Infof("Failed to list volumes: %v", err)
		return nil, err
	}
	volumeList, err := volumes.ExtractVolumes(pages)
	if err != nil {
		logger.Log.Infof("Failed to extract volumes: %v", err)
		return nil, err
	}

	// Filter volumes
	if len(volumeList) == 0 {
		logger.Log.Infof("No volumes found")
		return nil, nil
	}
	if len(volumeList) > 1 {
		logger.Log.Infof("More than one volumes found")
		return nil, errors.New("more than one volumes found")
	}
	return &volumeList[0], nil
}

func GetInstanceUUID() (string, error) {
	const metadataURL = "http://169.254.169.254/openstack/latest/meta_data.json"
	resp, err := http.Get(metadataURL)
	if err != nil {
		logger.Log.Infof("Failed to fetch metadata: %v", err)
		return "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Log.Infof("Failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		logger.Log.Infof("Unexpected status code: %d", resp.StatusCode)
		return "", err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Log.Infof("Failed to read metadata response: %v", err)
		return "", err
	}
	var metaData struct {
		UUID string `json:"uuid"`
	}
	if err := json.Unmarshal(body, &metaData); err != nil {
		logger.Log.Infof("Failed to parse metadata JSON: %v", err)
		return "", err
	}
	if metaData.UUID == "" {
		logger.Log.Infof("Instance UUID not found in metadata")
		return "", err
	}
	return metaData.UUID, nil
}

func AttachVolume(client *gophercloud.ProviderClient, volumeID string, instanceName string, instanceUUID string) error {
	computeClient, err := openstack.NewComputeV2(client, gophercloud.EndpointOpts{})
	logger.Log.Infof("Volume ID: %s", volumeID)
	if err != nil {
		logger.Log.Infof("Failed to create compute client: %v", err)
		return err
	}
	if instanceUUID == "" {
		// Get conversion host UUID
		logger.Log.Infof("Instance name: %s", instanceName)
		allServers, err := servers.List(computeClient, nil).AllPages(context.TODO())
		if err != nil {
			logger.Log.Infof("Failed to list servers: %v", err)
			return err
		}
		serversList, err := servers.ExtractServers(allServers)
		if err != nil {
			logger.Log.Infof("Failed to extract servers: %v", err)
			return err
		}
		for _, server := range serversList {
			if server.Name == instanceName {
				instanceUUID = server.ID
			}
		}
	}
	createOpts := volumeattach.CreateOpts{
		VolumeID: volumeID,
	}
	result, err := volumeattach.Create(context.TODO(), computeClient, instanceUUID, createOpts).Extract()
	logger.Log.Infof("Volume attached: %v", result)
	if err != nil {
		logger.Log.Infof("Failed to attach volume: %v", err)
		return err
	}
	volumeClient, err := openstack.NewBlockStorageV3(client, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		logger.Log.Infof("Failed to create block storage client: %v", err)
		return err
	}
	err = WaitForVolumeStatus(volumeClient, volumeID, "in-use", 3000)
	if err != nil {
		logger.Log.Infof("Failed to wait for volume to become in-use: %v", err)
		return err
	}
	return nil
}

func DetachVolume(client *gophercloud.ProviderClient, volumeID, instanceName, instanceUUID string, cloudOpts DstCloud) error {
	computeClient, err := openstack.NewComputeV2(client, gophercloud.EndpointOpts{})
	if err != nil {
		logger.Log.Infof("Failed to create compute client: %v", err)
		return err
	}
	if instanceUUID == "" {
		// Get conversion host UUID
		allServers, err := servers.List(computeClient, nil).AllPages(context.TODO())
		if err != nil {
			logger.Log.Infof("Failed to list servers: %v", err)
			return err
		}
		serversList, err := servers.ExtractServers(allServers)
		if err != nil {
			logger.Log.Infof("Failed to extract servers: %v", err)
			return err
		}
		for _, server := range serversList {
			if server.Name == instanceName {
				logger.Log.Infof("Found instance UUID: %s\n", server.ID)
				instanceUUID = server.ID
			}
		}
	}
	err = volumeattach.Delete(context.TODO(), computeClient, instanceUUID, volumeID).ExtractErr()
	if err != nil {
		logger.Log.Infof("Failed to detach volume: %v", err)
		logger.Log.Infof("Trying to re authenticate ...")

		providerCli, err := OpenstackAuth(context.TODO(), cloudOpts)
		if err != nil {
			logger.Log.Infof("Re Authentication failed: %v", err)
			return err
		}
		computeClient, err = openstack.NewComputeV2(providerCli, gophercloud.EndpointOpts{})
		if err != nil {
			logger.Log.Infof("Failed to create compute client: %v", err)
			return err
		}
		err = volumeattach.Delete(context.TODO(), computeClient, instanceUUID, volumeID).ExtractErr()
		if err != nil {
			logger.Log.Infof("Failed to detach volume after re Authentication: %v", err)
			return err
		}
	}
	volumeClient, err := openstack.NewBlockStorageV3(client, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		logger.Log.Infof("Failed to create block storage client: %v", err)
		return err
	}
	err = WaitForVolumeStatus(volumeClient, volumeID, "available", 3000)
	if err != nil {
		logger.Log.Infof("Failed to wait for volume to become available: %v", err)
		return err
	}
	logger.Log.Infof("Volume detached: %s", volumeID)
	return nil
}

func CreateServer(provider *gophercloud.ProviderClient, args ServerArgs) (string, error) {
	client, err := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		return "", fmt.Errorf("failed to create compute client: %v", err)
	}

	var nics []servers.Network
	for _, nic := range args.Nics {
		if m, ok := nic.(map[string]interface{}); ok {
			networkID, _ := m["net-id"].(string)
			portID, _ := m["port-id"].(string)
			nics = append(nics, servers.Network{
				UUID: networkID,
				Port: portID,
			})
		}
	}

	var blockDevices []servers.BlockDevice
	blockDevices = append(blockDevices, servers.BlockDevice{
		BootIndex:           0,
		UUID:                args.BootVolume,
		SourceType:          servers.SourceVolume,
		DestinationType:     servers.DestinationVolume,
		DeleteOnTermination: false,
	})

	index := 1
	for _, vol := range args.Volumes {
		if vol == "" {
			continue
		}
		blockDevices = append(blockDevices, servers.BlockDevice{
			BootIndex:           index,
			UUID:                vol,
			SourceType:          servers.SourceVolume,
			DestinationType:     servers.DestinationVolume,
			DeleteOnTermination: false,
		})
		index++
	}
	createOpts := servers.CreateOpts{
		Name:           args.Name,
		FlavorRef:      args.Flavor,
		Networks:       nics,
		SecurityGroups: args.SecurityGroups,
		BlockDevice:    blockDevices,
	}

	server, err := servers.Create(context.TODO(), client, createOpts, servers.SchedulerHintOpts{}).Extract()
	if err != nil {
		return "", fmt.Errorf("failed to create server: %v", err)
	}
	err = servers.WaitForStatus(context.TODO(), client, server.ID, "ACTIVE")
	if err != nil {
		return "", err
	}
	return server.ID, nil
}

func GetFlavorInfo(provider *gophercloud.ProviderClient, flavorNameOrID string) (*flavors.Flavor, error) {
	client, err := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create compute client: %v", err)
	}

	var flavor *flavors.Flavor
	f, err := flavors.Get(context.TODO(), client, flavorNameOrID).Extract()
	if err != nil {
		// Search by name
		logger.Log.Infof("Failed to get flavor by ID, searching by name: %s", flavorNameOrID)
		pages, err := flavors.ListDetail(client, nil).AllPages(context.TODO())
		if err != nil {
			logger.Log.Infof("Failed to list flavors: %v", err)
			return nil, err
		}
		allFlavors, err := flavors.ExtractFlavors(pages)
		if err != nil {
			logger.Log.Infof("Failed to extract flavors: %v", err)
			return nil, err
		}
		found := false
		for _, f := range allFlavors {
			if f.Name == flavorNameOrID {
				flavor = &f
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("flavor not found: %s", flavorNameOrID)
		}
	} else {
		flavor = f
	}
	return flavor, nil
}

// VolumeInfo represents volume information
type VolumeInfo struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Status   string            `json:"status"`
	Size     int               `json:"size"`
	Bootable string            `json:"bootable"`
	Metadata map[string]string `json:"metadata"`
}

// GetVolumeInfo retrieves volume information by name
func GetVolumeInfo(provider *gophercloud.ProviderClient, volumeName string) (*VolumeInfo, error) {
	client, err := openstack.NewBlockStorageV3(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		logger.Log.Infof("Failed to create block storage client: %v", err)
		return nil, err
	}

	// List volumes with the given name
	listOpts := volumes.ListOpts{
		Name: volumeName,
	}
	pages, err := volumes.List(client, listOpts).AllPages(context.TODO())
	if err != nil {
		logger.Log.Infof("Failed to list volumes: %v", err)
		return nil, err
	}

	volumeList, err := volumes.ExtractVolumes(pages)
	if err != nil {
		logger.Log.Infof("Failed to extract volumes: %v", err)
		return nil, err
	}

	if len(volumeList) == 0 {
		return nil, fmt.Errorf("volume not found: %s", volumeName)
	}

	if len(volumeList) > 1 {
		return nil, fmt.Errorf("multiple volumes found with name: %s", volumeName)
	}

	volume := volumeList[0]
	volumeInfo := &VolumeInfo{
		ID:       volume.ID,
		Name:     volume.Name,
		Status:   volume.Status,
		Size:     volume.Size,
		Bootable: volume.Bootable,
		Metadata: volume.Metadata,
	}

	return volumeInfo, nil
}

func CinderManage(provider *gophercloud.ProviderClient, volumeName string, hostPool string) (*volumes.Volume, error) {
	bsClient, err := openstack.NewBlockStorageV3(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		logger.Log.Fatalf("Failed to create block storage client: %v", err)
		return nil, err
	}
	body := map[string]interface{}{
		"volume": map[string]interface{}{
			"host": hostPool,
			"ref":  map[string]string{"source-name": volumeName},
			"name": volumeName,
		},
	}
	var resp struct {
		Volume struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"volume"`
	}

	manageURL := bsClient.ServiceURL("manageable_volumes")
	_, err = bsClient.Post(context.TODO(), manageURL, body, &resp, nil)
	if err != nil {
		logger.Log.Fatalf("Error while managing existing volume: %v", err)
		return nil, err
	}
	volume, err := GetVolume(provider, resp.Volume.ID)
	if err != nil {
		logger.Log.Fatalf("Failed to get managed volume: %v", err)
		return nil, err
	}
	if volume.Status == "error" {
		logger.Log.Fatalf("Managed volume is in error state, status: %s", volume.Status)
		return nil, fmt.Errorf("managed volume is in error state, status: %s", volume.Status)
	}
	logger.Log.Infof("Successfully managed existing volume: %s, ID: %s", resp.Volume.Name, resp.Volume.ID)
	return volume, nil
}

func GetVolume(provider *gophercloud.ProviderClient, volumeID string) (*volumes.Volume, error) {
	bsClient, err := openstack.NewBlockStorageV3(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		logger.Log.Fatalf("Failed to create block storage client: %v", err)
		return nil, err
	}
	volume, err := volumes.Get(context.TODO(), bsClient, volumeID).Extract()
	if err != nil {
		logger.Log.Infof("Failed to get volume: %v", err)
		return nil, err
	}
	return volume, nil
}
