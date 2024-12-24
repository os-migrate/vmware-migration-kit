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
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	gophercloud "github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/volumes"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/volumeattach"
)

type VolOpts struct {
	Name       string
	Size       int
	VolumeType string
	BusType    string
	Metadata   map[string]string
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

func CreateVolume(provider *gophercloud.ProviderClient, opts VolOpts, setUEFI bool) (*volumes.Volume, error) {
	client, err := openstack.NewBlockStorageV3(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		logger.Printf("Failed to create block storage client: %v", err)
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
		logger.Printf("Failed to create volume: %v", err)
		return nil, err
	}

	err = WaitForVolumeStatus(client, volume.ID, "available", 3000)
	if err != nil {
		logger.Printf("Failed to wait for volume to become available: %v", err)
		return nil, err
	}
	// Set bootable
	options := volumes.BootableOpts{
		Bootable: true,
	}
	err = volumes.SetBootable(context.TODO(), client, volume.ID, options).ExtractErr()
	if err != nil {
		panic(err)
	}
	if err != nil {
		logger.Printf("Failed to set volume as bootable: %v", err)
		return nil, err
	}
	if err != nil {
		logger.Printf("Failed to set volume as bootable: %v", err)
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
			logger.Printf("Failed to set image metadata: %v", err)
			return nil, err
		}
	}
	return volume, nil
}

func WaitForVolumeStatus(client *gophercloud.ServiceClient, volumeID, status string, timeout int) error {
	for i := 0; i < timeout; i++ {
		volume, err := volumes.Get(context.TODO(), client, volumeID).Extract()
		if err != nil {
			logger.Printf("Failed to get volume status: %v", err)
			return err
		}

		if volume.Status == status {
			return nil
		}

		time.Sleep(5 * time.Second)
	}
	logger.Printf("Volume %s did not reach status %s within the timeout", volumeID, status)
	return fmt.Errorf("volume %s did not reach status %s within the timeout", volumeID, status)
}

func GetVolumeID(client *gophercloud.ProviderClient, vm string, disk string) (*volumes.Volume, error) {
	blockStorageClient, err := openstack.NewBlockStorageV3(client, gophercloud.EndpointOpts{})
	if err != nil {
		logger.Printf("Failed to create block storage client: %w", err)
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
		logger.Printf("Failed to list volumes: %v", err)
		return nil, err
	}
	volumeList, err := volumes.ExtractVolumes(pages)
	if err != nil {
		logger.Printf("Failed to extract volumes: %v", err)
		return nil, err
	}

	// Filter volumes
	if len(volumeList) == 0 {
		logger.Printf("No volumes found")
		return nil, nil
	}
	if len(volumeList) > 1 {
		logger.Printf("More than one volumes found")
		return nil, errors.New("More than one volumes found")
	}
	return &volumeList[0], nil
}

func GetInstanceUUID() (string, error) {
	const metadataURL = "http://169.254.169.254/openstack/latest/meta_data.json"
	resp, err := http.Get(metadataURL)
	if err != nil {
		logger.Printf("Failed to fetch metadata: %v", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Printf("Unexpected status code: %d", resp.StatusCode)
		return "", err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logger.Printf("Failed to read metadata response: %v", err)
		return "", err
	}
	var metaData struct {
		UUID string `json:"uuid"`
	}
	if err := json.Unmarshal(body, &metaData); err != nil {
		logger.Printf("Failed to parse metadata JSON: %v", err)
		return "", err
	}
	if metaData.UUID == "" {
		logger.Printf("Instance UUID not found in metadata")
		return "", err
	}
	return metaData.UUID, nil
}

func AttachVolume(client *gophercloud.ProviderClient, volumeID string, instanceName string, instanceUUID string) error {
	computeClient, err := openstack.NewComputeV2(client, gophercloud.EndpointOpts{})
	logger.Printf("Volume ID: %s", volumeID)
	if err != nil {
		logger.Printf("Failed to create compute client: %v", err)
		return err
	}
	if instanceUUID == "" {
		// Get conversion host UUID
		logger.Printf("Instance name: %s", instanceName)
		allServers, err := servers.List(computeClient, nil).AllPages(context.TODO())
		if err != nil {
			logger.Printf("Failed to list servers: %v", err)
			return err
		}
		serversList, err := servers.ExtractServers(allServers)
		if err != nil {
			logger.Printf("Failed to extract servers: %v", err)
			return err
		}
		for _, server := range serversList {
			if server.Name == instanceName {
				fmt.Printf("Found instance UUID: %s\n", server.ID)
				instanceUUID = server.ID
			}
		}
	}
	createOpts := volumeattach.CreateOpts{
		VolumeID: volumeID,
	}
	result, err := volumeattach.Create(context.TODO(), computeClient, instanceUUID, createOpts).Extract()
	logger.Printf("Volume attached: %v", result)
	if err != nil {
		logger.Printf("Failed to attach volume: %v", err)
		return err
	}
	volumeClient, err := openstack.NewBlockStorageV3(client, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	err = WaitForVolumeStatus(volumeClient, volumeID, "in-use", 3000)
	if err != nil {
		logger.Printf("Failed to wait for volume to become in-use: %v", err)
		return err
	}
	return nil
}

func DetachVolume(client *gophercloud.ProviderClient, volumeID string, instanceName string, instanceUUID string) error {
	computeClient, err := openstack.NewComputeV2(client, gophercloud.EndpointOpts{})
	if err != nil {
		logger.Printf("Failed to create compute client: %v", err)
		return err
	}
	if instanceUUID == "" {
		// Get conversion host UUID
		allServers, err := servers.List(computeClient, nil).AllPages(context.TODO())
		if err != nil {
			logger.Printf("Failed to list servers: %v", err)
			return err
		}
		serversList, err := servers.ExtractServers(allServers)
		if err != nil {
			logger.Printf("Failed to extract servers: %v", err)
			return err
		}
		for _, server := range serversList {
			if server.Name == instanceName {
				fmt.Printf("Found instance UUID: %s\n", server.ID)
				instanceUUID = server.ID
			}
		}
	}

	err = volumeattach.Delete(context.TODO(), computeClient, instanceUUID, volumeID).ExtractErr()
	if err != nil {
		logger.Printf("Failed to detach volume: %v", err)
		return err
	}
	volumeClient, err := openstack.NewBlockStorageV3(client, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	err = WaitForVolumeStatus(volumeClient, volumeID, "available", 3000)
	if err != nil {
		logger.Printf("Failed to wait for volume to become available: %v", err)
		return err
	}
	return nil
}
