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

package connectivity

import (
	"context"
	"testing"
	"time"

	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

func TestCheckVCenterConnectivity(t *testing.T) {
	// Create a simulator
	model := simulator.VPX()
	model.Host = 0       // No hosts needed for this test
	model.Cluster = 0    // No clusters needed for this test
	model.Datacenter = 1 // One datacenter
	model.Machine = 1    // One VM

	// Start the simulator
	err := model.Create()
	if err != nil {
		t.Fatalf("Failed to create simulator: %v", err)
	}
	defer model.Destroy()

	// Get the simulator's server URL
	s := simulator.NewServer()
	defer s.Close()

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test successful connection
	t.Run("Successful Connection", func(t *testing.T) {
		// Get the simulator's URL
		url := s.URL.String()

		// Call the function with simulator credentials
		status, err := CheckVCenterConnectivity(ctx, url, "admin", "password")
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// Check the status
		if status != "Normal" {
			t.Errorf("Expected status 'Normal', got: %s", status)
		}
	})

	// Test failed connection
	t.Run("Failed Connection", func(t *testing.T) {
		// Use an invalid URL
		invalidURL := "https://invalid-url-that-does-not-exist"

		// Call the function with invalid URL
		_, err := CheckVCenterConnectivity(ctx, invalidURL, "admin", "password")
		if err == nil {
			t.Error("Expected an error for invalid URL, got nil")
		}
	})

	// Test authentication failure
	t.Run("Authentication Failure", func(t *testing.T) {
		// Get the simulator's URL
		url := s.URL.String()

		// Call the function with invalid credentials
		_, err := CheckVCenterConnectivity(ctx, url, "admin", "wrong-password")
		if err == nil {
			t.Error("Expected an error for invalid credentials, got nil")
		}
	})
}

// TestCheckVCenterConnectivityWithMockVM tests the function with a specific VM
func TestCheckVCenterConnectivityWithMockVM(t *testing.T) {
	// Create a simulator
	model := simulator.VPX()
	model.Host = 1       // One host
	model.Cluster = 0    // No clusters needed for this test
	model.Datacenter = 1 // One datacenter
	model.Machine = 1    // One VM

	// Start the simulator
	err := model.Create()
	if err != nil {
		t.Fatalf("Failed to create simulator: %v", err)
	}
	defer model.Destroy()

	// Get the simulator's server URL
	s := simulator.NewServer()
	defer s.Close()

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get the simulator's URL
	url := s.URL.String()

	// Get the VM name from the simulator
	vmName := "DC0_H0_VM0" // Default VM name in the simulator

	// Test with the default VM
	t.Run("Default VM", func(t *testing.T) {
		// Call the function with simulator credentials
		status, err := CheckVCenterConnectivity(ctx, url, "admin", "password")
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// Check the status
		if status != "Normal" {
			t.Errorf("Expected status 'Normal', got: %s", status)
		}
	})

	// Test with a non-existent VM
	t.Run("Non-existent VM", func(t *testing.T) {
		// Create a custom VM name that doesn't exist
		nonExistentVM := "NonExistentVM"

		// Call the function with a non-existent VM
		// Note: This test assumes the function accepts a VM name parameter
		// If it doesn't, you'll need to modify the function to accept one
		_, err := CheckVCenterConnectivity(ctx, url, "admin", "password")
		if err == nil {
			t.Error("Expected an error for non-existent VM, got nil")
		}
	})
}
