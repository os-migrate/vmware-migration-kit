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

package openstack

import (
	"context"
	"fmt"
	"os"
	"vmware-migration-kit/plugins/module_utils/logger"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/ports"
)

// GetNetwork retrieves a network by name or ID
func GetNetwork(provider *gophercloud.ProviderClient, networkNameOrID string) (*networks.Network, error) {
	client, err := openstack.NewNetworkV2(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		logger.Log.Infof("Failed to create network client: %v", err)
		return nil, err
	}

	// First try to get by ID (UUID)
	network, err := networks.Get(context.TODO(), client, networkNameOrID).Extract()
	if err == nil {
		return network, nil
	}

	// If that fails, try to get by name
	listOpts := networks.ListOpts{
		Name: networkNameOrID,
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
		return nil, fmt.Errorf("network not found: %s", networkNameOrID)
	}

	if len(networkList) > 1 {
		return nil, fmt.Errorf("multiple networks found with name: %s", networkNameOrID)
	}

	return &networkList[0], nil
}

// CreatePort creates a network port with the specified parameters
func CreatePort(provider *gophercloud.ProviderClient, portName, networkID, macAddress string, securityGroups []string) (*ports.Port, error) {
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
