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
	"os"
	"vmware-migration-kit/plugins/module_utils/logger"
	osm_os "vmware-migration-kit/plugins/module_utils/openstack"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/ports"
)

// Ansible
type ModuleArgs struct {
	Cloud                 osm_os.DstCloud `json:"cloud"`
	OsMigrateNicsFilePath string          `json:"os_migrate_nics_file_path"`
	VmName                string          `json:"vm_name"`
	UsedMappedNetworks    bool            `json:"used_mapped_networks"`
	SecurityGroups        []string        `json:"security_groups"`
	NetworkName           string          `json:"network_name"`
}

type NicInfo struct {
	Vlan string `json:"vlan"`
	Mac  string `json:"mac"`
}

type PortInfo struct {
	PortID string `json:"port-id"`
}

type Response struct {
	Msg     string     `json:"msg"`
	Changed bool       `json:"changed"`
	Failed  bool       `json:"failed"`
	Ports   []PortInfo `json:"ports"`
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

func loadJSONFile(filePath string, target interface{}) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %v", filePath, err)
	}
	defer file.Close()

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

func getNetworkByName(provider *gophercloud.ProviderClient, networkName string) (*networks.Network, error) {
	client, err := openstack.NewNetworkV2(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		logger.Log.Infof("Failed to create network client: %v", err)
		return nil, err
	}

	listOpts := networks.ListOpts{
		Name: networkName,
	}
	pages, err := networks.List(client, listOpts).AllPages(context.TODO())
	if err != nil {
		logger.Log.Infof("Failed to list networks: %v", err)
		return nil, err
	}

	networkList, err := networks.ExtractNetworks(pages)
	if err != nil {
		logger.Log.Infof("Failed to extract networks: %v", err)
		return nil, err
	}

	if len(networkList) == 0 {
		return nil, fmt.Errorf("network not found: %s", networkName)
	}

	if len(networkList) > 1 {
		return nil, fmt.Errorf("multiple networks found with name: %s", networkName)
	}

	return &networkList[0], nil
}

func createPort(provider *gophercloud.ProviderClient, portName, networkID, macAddress string, securityGroups []string) (*ports.Port, error) {
	client, err := openstack.NewNetworkV2(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		logger.Log.Infof("Failed to create network client: %v", err)
		return nil, err
	}

	createOpts := ports.CreateOpts{
		Name:           portName,
		NetworkID:      networkID,
		MACAddress:     macAddress,
		SecurityGroups: &securityGroups,
		AllowedAddressPairs: []ports.AddressPair{
			{
				IPAddress:  "0.0.0.0/0",
				MACAddress: macAddress,
			},
		},
	}

	port, err := ports.Create(context.TODO(), client, createOpts).Extract()
	if err != nil {
		logger.Log.Infof("Failed to create port: %v", err)
		return nil, err
	}

	return port, nil
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

	// Load the NICs data file
	var vmNics []NicInfo
	err = loadJSONFile(moduleArgs.OsMigrateNicsFilePath, &vmNics)
	if err != nil {
		response.Msg = "Failed to load network data file: " + err.Error()
		FailJson(response)
	}

	// If not mapped networks, use the network name provided
	if !moduleArgs.UsedMappedNetworks {
		for i := range vmNics {
			vmNics[i].Vlan = moduleArgs.NetworkName
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	provider, err := osm_os.OpenstackAuth(ctx, moduleArgs.Cloud)
	if err != nil {
		response.Msg = "Failed to authenticate Openstack client: " + err.Error()
		FailJson(response)
	}

	var portUUIDs []PortInfo
	for nicIndex, nic := range vmNics {
		// Get network ID
		network, err := getNetworkByName(provider, nic.Vlan)
		if err != nil {
			response.Msg = "Failed to get network: " + err.Error()
			FailJson(response)
		}

		portName := fmt.Sprintf("%s-NIC-%d-VLAN-%s", moduleArgs.VmName, nicIndex, nic.Vlan)
		port, err := createPort(provider, portName, network.ID, nic.Mac, moduleArgs.SecurityGroups)
		if err != nil {
			response.Msg = "Failed to create port: " + err.Error()
			FailJson(response)
		}

		portUUIDs = append(portUUIDs, PortInfo{PortID: port.ID})
		response.Changed = true
	}

	response.Msg = "Network ports created successfully"
	response.Ports = portUUIDs
	ExitJson(response)
}
