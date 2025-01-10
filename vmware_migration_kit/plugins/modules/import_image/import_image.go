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
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"vmware-migration-kit/vmware_migration_kit/plugins/module_utils/ansible"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/imageservice/v2/imagedata"
	"github.com/gophercloud/gophercloud/openstack/imageservice/v2/images"
	"github.com/gophercloud/utils/openstack/clientconfig"
)

var args struct {
	Name     string `json:"name"`
	DiskPath string `json:"disk_path"`
}

var logger *log.Logger
var logFile string = "/tmp/osm-import-volume-as-image.log"

func init() {
	logFile, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	logger = log.New(logFile, "osm-image: ", log.LstdFlags|log.Lshortfile)
}

func UploadImage(provider *gophercloud.ProviderClient, imageName, diskPath string) (string, error) {
	client, err := openstack.NewImageServiceV2(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		return "", fmt.Errorf("failed to create image service client: %v", err)
	}

	createOpts := images.CreateOpts{
		Name:            imageName,
		DiskFormat:      "qcow2",
		ContainerFormat: "bare",
	}

	image, err := images.Create(client, createOpts).Extract()
	if err != nil {
		return "", fmt.Errorf("failed to create image: %v", err)
	}

	file, err := os.Open(diskPath)
	if err != nil {
		return "", fmt.Errorf("failed to open disk file: %v", err)
	}
	defer file.Close()

	err = imagedata.Upload(client, image.ID, file).ExtractErr()
	if err != nil {
		return "", fmt.Errorf("failed to upload image: %v", err)
	}

	return image.ID, nil
}

func main() {
	var response ansible.Response

	if len(os.Args) != 2 {
		response.Msg = "No argument file provided"
		ansible.FailJson(response)
	}

	argsFile := os.Args[1]
	argsData, err := os.ReadFile(argsFile)
	if err != nil {
		response.Msg = fmt.Sprintf("Failed to read argument file: %v", err)
		ansible.FailJson(response)
	}

	if err := json.Unmarshal(argsData, &args); err != nil {
		response.Msg = fmt.Sprintf("Failed to parse argument file: %v", err)
		ansible.FailJson(response)
	}
	opts, err := clientconfig.AuthOptions(nil)
	if err != nil {
		response.Msg = fmt.Sprintf("Failed to get auth options: %v", err)
		ansible.FailJson(response)
	}
	provider, err := openstack.AuthenticatedClient(*opts)
	if err != nil {
		response.Msg = fmt.Sprintf("Failed to authenticate: %v", err)
		ansible.FailJson(response)
	}
	var ID []string
	imageID, err := UploadImage(provider, args.Name, args.DiskPath)
	if err != nil {
		response.Msg = fmt.Sprintf("Failed to upload image: %v", err)
		ansible.FailJson(response)
	}

	response.ID = append(ID, imageID)
	response.Msg = fmt.Sprintf("Image created successfully: %s", imageID)
	ansible.ExitJson(response)
}
