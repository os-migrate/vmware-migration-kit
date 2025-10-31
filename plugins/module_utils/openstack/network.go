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
	"time"
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

// WaitForPortStatus waits for a port to reach a specific status
func WaitForPortStatus(client *gophercloud.ServiceClient, portID, status string, timeout int) error {
	for i := 0; i < timeout; i++ {
		port, err := ports.Get(context.TODO(), client, portID).Extract()
		if err != nil {
			// If port is not found, it might be deleted (which is what we want)
			if status == "deleted" {
				return nil
			}
			logger.Log.Infof("Failed to get port status: %v", err)
			return err
		}
		if port.Status == status {
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	logger.Log.Infof("Port %s did not reach status %s within the timeout", portID, status)
	return fmt.Errorf("port %s did not reach status %s within the timeout", portID, status)
}

// DeletePort deletes a network port by ID
func DeletePort(provider *gophercloud.ProviderClient, portID string) error {
	client, err := openstack.NewNetworkV2(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		logger.Log.Infof("Failed to create network client: %v", err)
		return err
	}

	logger.Log.Infof("Deleting port %s...", portID)
	err = ports.Delete(context.TODO(), client, portID).ExtractErr()
	if err != nil {
		logger.Log.Infof("Failed to delete port: %v", err)
		return err
	}

	// Wait for port to be fully deleted to avoid MAC address conflicts
	logger.Log.Infof("Waiting for port %s to be fully deleted...", portID)
	err = WaitForPortStatus(client, portID, "deleted", 12) // 60 seconds timeout
	if err != nil {
		logger.Log.Infof("Port %s did not reach deleted status within timeout: %v", portID, err)
		// Don't return error here as the port deletion might have succeeded
		// but the status check failed due to timing issues
	}

	logger.Log.Infof("Port %s deleted successfully", portID)
	return nil
}
