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
	"fmt"
	"io"
	"math"
	"os"
	"vmware-migration-kit/plugins/module_utils/logger"
	osm_os "vmware-migration-kit/plugins/module_utils/openstack"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/flavors"
)

// Ansible
type ModuleArgs struct {
	Cloud         osm_os.DstCloud `json:"cloud"`
	GuestInfoPath string          `json:"guest_info_path"`
	DiskInfoPath  string          `json:"disk_info_path"`
	UseDiskInfo   bool            `json:"use_disk_info"`
}

type GuestInfo struct {
	HwProcessorCount int    `json:"hw_processor_count"`
	HwMemtotalMb     int    `json:"hw_memtotal_mb"`
	HwFolder         string `json:"hw_folder,omitempty"`
}

type Disk struct {
	Capacity int `json:"capacity"`
}

type DiskInfo struct {
	Disks []Disk `json:"disks"`
}

type Response struct {
	Msg                 string `json:"msg"`
	Changed             bool   `json:"changed"`
	Failed              bool   `json:"failed"`
	OpenstackFlavorUuid string `json:"openstack_flavor_uuid"`
}

func ExitJson(responseBody Response) {
	returnResponse(responseBody)
}

func FailJson(responseBody Response) {
	responseBody.Failed = true
	returnResponse(responseBody)
}

func FailWithMessage(msg string) {
	response := Response{Msg: msg}
	FailJson(response)
}

func returnResponse(responseBody Response) {
	var response []byte
	var err error
	response, err = json.Marshal(responseBody)
	if err != nil {
		response, _ = json.Marshal(Response{Msg: "Invalid response object"})
	}
	fmt.Println(string(response))
	if responseBody.Failed {
		os.Exit(1)
	} else {
		os.Exit(0)
	}
}

func getTotalDiskCapacity(diskInfo *DiskInfo) int {
	totalCapacityKb := 0
	for _, disk := range diskInfo.Disks {
		totalCapacityKb += disk.Capacity
	}
	return totalCapacityKb / 1024 // Convert KB to MB
}

func flavorDistance(flavor *flavors.Flavor, guestInfo *GuestInfo, diskCapacityMb int) int {
	vcpuDiff := int(math.Abs(float64(flavor.VCPUs - guestInfo.HwProcessorCount)))
	ramDiff := int(math.Abs(float64(flavor.RAM - guestInfo.HwMemtotalMb)))
	diskDiff := int(math.Abs(float64(flavor.Disk - diskCapacityMb)))
	return vcpuDiff + ramDiff + diskDiff
}

func loadJSONFile(filePath string, target interface{}) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %v", filePath, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			// Log the close error but don't fail the operation
			logger.Log.Infof("Warning: failed to close file %s: %v", filePath, closeErr)
		}
	}()

	data, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %v", filePath, err)
	}

	err = json.Unmarshal(data, target)
	if err != nil {
		return fmt.Errorf("failed to parse JSON from file %s: %v", filePath, err)
	}

	return nil
}

func findBestMatchingFlavor(provider *gophercloud.ProviderClient, guestInfo *GuestInfo, diskCapacityMb int) (*flavors.Flavor, error) {
	client, err := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		logger.Log.Infof("Failed to create compute client: %v", err)
		return nil, err
	}

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

	if len(allFlavors) == 0 {
		return nil, fmt.Errorf("no flavors found")
	}

	bestFlavor := &allFlavors[0]
	minDistance := flavorDistance(bestFlavor, guestInfo, diskCapacityMb)

	for i := 1; i < len(allFlavors); i++ {
		distance := flavorDistance(&allFlavors[i], guestInfo, diskCapacityMb)
		if distance < minDistance {
			minDistance = distance
			bestFlavor = &allFlavors[i]
		}
	}

	return bestFlavor, nil
}

func main() {
	var response Response
	if len(os.Args) != 2 {
		response.Msg = "No argument file provided"
		FailJson(response)
	}

	argsFile := os.Args[1]
	text, err := os.ReadFile(argsFile)
	if err != nil {
		response.Msg = "Could not read configuration file: " + argsFile
		FailJson(response)
	}

	var moduleArgs ModuleArgs
	err = json.Unmarshal(text, &moduleArgs)
	if err != nil {
		response.Msg = "Configuration file not valid JSON: " + argsFile
		FailJson(response)
	}

	// Load guest info
	var guestInfo GuestInfo
	err = loadJSONFile(moduleArgs.GuestInfoPath, &guestInfo)
	if err != nil {
		response.Msg = "Failed to load guest info: " + err.Error()
		FailJson(response)
	}

	// Load disk info
	var diskInfo DiskInfo
	if moduleArgs.UseDiskInfo {
		err = loadJSONFile(moduleArgs.DiskInfoPath, &diskInfo)
		if err != nil {
			response.Msg = "Failed to load disk info: " + err.Error()
			FailJson(response)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	provider, err := osm_os.OpenstackAuth(ctx, moduleArgs.Cloud)
	if err != nil {
		response.Msg = "Failed to authenticate Openstack client: " + err.Error()
		FailJson(response)
	}

	var diskCapacityMb int
	if !moduleArgs.UseDiskInfo {
		// If not using disk info, set disk capacity to 0
		diskCapacityMb = 0
	} else {
		// Calculate total disk capacity
		diskCapacityMb = getTotalDiskCapacity(&diskInfo)
	}
	// Find best matching flavor
	bestFlavor, err := findBestMatchingFlavor(provider, &guestInfo, diskCapacityMb)
	if err != nil {
		response.Msg = "Failed to find best matching flavor: " + err.Error()
		FailJson(response)
	}

	response.Changed = true
	response.Msg = "Best matching flavor found"
	response.OpenstackFlavorUuid = bestFlavor.ID
	ExitJson(response)
}
