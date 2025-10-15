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
 */

package main

import (
	"context"
	"encoding/json"
	"os"

	"vmware-migration-kit/plugins/module_utils/ansible"
	"vmware-migration-kit/plugins/module_utils/logger"
	osm_os "vmware-migration-kit/plugins/module_utils/openstack"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/flavors"
	"gopkg.in/yaml.v3"
)

// FlavorResource represents only the essential flavor parameters
type FlavorResource struct {
	Name        string  `yaml:"name"`
	RAM         int     `yaml:"ram"`
	VCPUs       int     `yaml:"vcpus"`
	Disk        int     `yaml:"disk"`
	Ephemeral   int     `yaml:"ephemeral"`
	Swap        int     `yaml:"swap"`
	RxTxFactor  float64 `yaml:"rxtx_factor"`
	IsPublic    bool    `yaml:"is_public"`
	Description string  `yaml:"description"`
}

// ModuleArgs defines JSON arguments passed to the module
type ModuleArgs struct {
	Cloud       osm_os.DstCloud `json:"cloud"`
	FlavorsFile string          `json:"flavors_file"`
}

// CreatedFlavor represents the flavor that was created
type CreatedFlavor struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

// ModuleResponse is returned to Ansible
type ModuleResponse struct {
	Changed       bool           `json:"changed"`
	Failed        bool           `json:"failed"`
	Msg           string         `json:"msg,omitempty"`
	CreatedFlavor *CreatedFlavor `json:"created_flavor,omitempty"`
}

// success returns a successful JSON response
func success(changed bool, flavor *CreatedFlavor) {
	res := ModuleResponse{
		Changed:       changed,
		Failed:        false,
		CreatedFlavor: flavor,
	}
	_ = json.NewEncoder(os.Stdout).Encode(res)
	os.Exit(0)
}

// fail returns a failure JSON response
func fail(msg string) {
	res := ModuleResponse{
		Changed: false,
		Failed:  true,
		Msg:     msg,
	}
	_ = json.NewEncoder(os.Stdout).Encode(res)
	os.Exit(1)
}

func main() {
	var response ansible.Response

	// Check if argument file is provided
	if len(os.Args) != 2 {
		response.Msg = "No argument file provided"
		ansible.FailJson(response)
	}

	argsFile := os.Args[1]

	// Read module arguments
	text, err := os.ReadFile(argsFile)
	if err != nil {
		response.Msg = "Could not read configuration file: " + argsFile
		ansible.FailJson(response)
	}

	var moduleArgs ModuleArgs
	if err := json.Unmarshal(text, &moduleArgs); err != nil {
		response.Msg = "Configuration file not valid JSON: " + err.Error()
		ansible.FailJson(response)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Authenticate to OpenStack
	provider, err := osm_os.OpenstackAuth(ctx, moduleArgs.Cloud)
	if err != nil {
		logger.Log.Infof("Failed to authenticate OpenStack client: " + err.Error())
		ansible.FailJson(ansible.Response{Msg: "Failed to authenticate OpenStack client: " + err.Error()})
	}

	// Create Compute client
	computeClient, err := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{
		Region: moduleArgs.Cloud.RegionName,
	})
	if err != nil {
		logger.Log.Infof("Failed to create Compute client: " + err.Error())
		ansible.FailJson(ansible.Response{Msg: "Failed to create Compute client: " + err.Error()})
	}

	// --- Read YAML ---
	yamlText, err := os.ReadFile(moduleArgs.FlavorsFile)
	if err != nil {
		logger.Log.Infof("Failed to read flavors file: " + err.Error())
		fail("Failed to read flavors file: " + err.Error())
	}

	// Parse only the 'params' of the first resource
	var raw struct {
		Resources []struct {
			Params FlavorResource `yaml:"params"`
		} `yaml:"resources"`
	}
	if err := yaml.Unmarshal(yamlText, &raw); err != nil {
		logger.Log.Infof("Failed to parse flavors YAML: " + err.Error())
		fail("Failed to parse flavors YAML: " + err.Error())
	}

	if len(raw.Resources) == 0 {
		fail("No flavors found in the YAML file")
	}

	flavorRes := raw.Resources[0].Params
	name := flavorRes.Name

	// List existing flavors
	allPages, err := flavors.ListDetail(computeClient, nil).AllPages(ctx)
	if err != nil {
		logger.Log.Infof("Failed to list flavors: " + err.Error())
		fail("Failed to list flavors: " + err.Error())
	}

	allFlavors, _ := flavors.ExtractFlavors(allPages)
	existing := make(map[string]string)
	for _, f := range allFlavors {
		existing[f.Name] = f.ID
	}

	// If flavor exists, return success with changed=false
	if id, ok := existing[name]; ok {
		success(false, &CreatedFlavor{Name: name, ID: id})
	}

	// Create the flavor
	createOpts := flavors.CreateOpts{
		Name:       flavorRes.Name,
		RAM:        flavorRes.RAM,
		VCPUs:      flavorRes.VCPUs,
		Disk:       &flavorRes.Disk,
		Ephemeral:  &flavorRes.Ephemeral,
		Swap:       &flavorRes.Swap,
		RxTxFactor: flavorRes.RxTxFactor,
		IsPublic:   &flavorRes.IsPublic,
	}

	flavor, err := flavors.Create(ctx, computeClient, createOpts).Extract()
	if err != nil {
		logger.Log.Infof("Failed to create flavor %s: %v", name, err)
		fail("Failed to create flavor " + name + ": " + err.Error())
	}

	success(true, &CreatedFlavor{
		Name: flavor.Name,
		ID:   flavor.ID,
	})
}
