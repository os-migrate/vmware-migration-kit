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
)

// Ansible
type ModuleArgs struct {
	Cloud                 osm_os.DstCloud `json:"cloud"`
	OsMigrateNicsFilePath string          `json:"os_migrate_nics_file_path"`
	VmName                string          `json:"vm_name"`
	UsedMappedNetworks    bool            `json:"used_mapped_networks"`
	SecurityGroups        []string        `json:"security_groups"`
	NetworkName           string          `json:"network_name"`
	UseFixedIPs           bool            `json:"use_fixed_ips"`
	SubnetUUID            string          `json:"subnet_uuid"`
}

type NicInfo struct {
	Vlan     string   `json:"vlan"`
	Mac      string   `json:"mac"`
	FixedIPs []string `json:"ipaddresses"`
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
		network, err := osm_os.GetNetwork(provider, nic.Vlan)
		if err != nil {
			response.Msg = "Failed to get network: " + err.Error()
			FailJson(response)
		}
		if !moduleArgs.UseFixedIPs {
			nic.FixedIPs = nil
		}
		portName := fmt.Sprintf("%s-NIC-%d-VLAN-%s", moduleArgs.VmName, nicIndex, nic.Vlan)
		port, err := osm_os.CreatePort(provider, portName, network.ID, nic.Mac, moduleArgs.SubnetUUID,
			moduleArgs.SecurityGroups, nic.FixedIPs)
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
