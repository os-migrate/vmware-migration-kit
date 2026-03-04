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
package moduleutils

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	osm_os "vmware-migration-kit/plugins/module_utils/openstack"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	th "github.com/gophercloud/gophercloud/v2/testhelper"
	fake "github.com/gophercloud/gophercloud/v2/testhelper/client"
)

// Helper function to create a mock provider
func createMockProvider() *gophercloud.ProviderClient {
	provider := &gophercloud.ProviderClient{TokenID: "dummy"}
	provider.EndpointLocator = func(_ gophercloud.EndpointOpts) (string, error) {
		return fake.ServiceClient().Endpoint, nil
	}
	return provider
}

// Helper function to create a failing provider
func createFailingProvider() *gophercloud.ProviderClient {
	provider := &gophercloud.ProviderClient{}
	provider.EndpointLocator = func(_ gophercloud.EndpointOpts) (string, error) {
		return "", gophercloud.ErrEndpointNotFound{}
	}
	return provider
}

// Test 1: GetVolume success
func TestGetVolumeSuccess(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/volumes/vol-123", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("Expected GET but got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			 "volume": {
				 "id": "vol-123",
				 "name": "test-volume",
				 "status": "available",
				 "size": 10
			 }
		 }`))
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	volume, err := osm_os.GetVolume(createMockProvider(), "vol-123")
	if err != nil {
		t.Fatalf("GetVolume returned error: %v", err)
	}
	if volume.ID != "vol-123" {
		t.Errorf("expected volume ID 'vol-123', got %s", volume.ID)
	}
}

// Test 2: GetVolume not found
func TestGetVolumeNotFound(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/volumes/nonexistent", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	_, err := osm_os.GetVolume(createMockProvider(), "nonexistent")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

// Test 3: GetVolume client init failure
func TestGetVolumeClientInitFailure(t *testing.T) {
	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	_, err := osm_os.GetVolume(createFailingProvider(), "vol-123")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

// Test 4: GetVolumeInfo success
func TestGetVolumeInfoSuccess(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/volumes/detail", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			 "volumes": [{
				 "id": "vol-456",
				 "name": "my-volume",
				 "status": "available",
				 "size": 20,
				 "bootable": "true",
				 "metadata": {"key": "value"}
			 }]
		 }`))
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	info, err := osm_os.GetVolumeInfo(createMockProvider(), "my-volume")
	if err != nil {
		t.Fatalf("GetVolumeInfo returned error: %v", err)
	}
	if info.ID != "vol-456" {
		t.Errorf("expected ID 'vol-456', got %s", info.ID)
	}
}

// Test 5: GetVolumeInfo not found
func TestGetVolumeInfoNotFound(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/volumes/detail", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"volumes": []}`))
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	_, err := osm_os.GetVolumeInfo(createMockProvider(), "nonexistent")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

// Test 6: GetVolumeInfo multiple volumes error
func TestGetVolumeInfoMultipleFound(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/volumes/detail", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			 "volumes": [
				 {"id": "vol-1", "name": "dup", "status": "available", "size": 10},
				 {"id": "vol-2", "name": "dup", "status": "available", "size": 10}
			 ]
		 }`))
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	_, err := osm_os.GetVolumeInfo(createMockProvider(), "dup")
	if err == nil {
		t.Fatal("expected error for multiple volumes")
	}
}

// Test 7: GetVolumeID success
func TestGetVolumeIDSuccess(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/volumes/detail", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			 "volumes": [{
				 "id": "vol-789",
				 "name": "vm1-disk0",
				 "status": "available",
				 "size": 50,
				 "metadata": {"osm": "true"}
			 }]
		 }`))
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	volume, err := osm_os.GetVolumeID(createMockProvider(), "vm1", "disk0")
	if err != nil {
		t.Fatalf("GetVolumeID returned error: %v", err)
	}
	if volume == nil {
		t.Fatal("expected volume but got nil")
		return
	}
	if volume.ID != "vol-789" {
		t.Errorf("expected ID 'vol-789', got %s", volume.ID)
	}
}

// Test 8: GetVolumeID not found returns nil
func TestGetVolumeIDNotFound(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/volumes/detail", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"volumes": []}`))
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	volume, err := osm_os.GetVolumeID(createMockProvider(), "nonexistent", "disk")
	if err != nil {
		t.Fatalf("GetVolumeID returned error: %v", err)
	}
	if volume != nil {
		t.Error("expected nil volume but got one")
	}
}

// Test 9: GetVolumeID multiple volumes error
func TestGetVolumeIDMultipleFound(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/volumes/detail", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			 "volumes": [
				 {"id": "vol-1", "name": "vm1-disk0", "status": "available", "size": 10},
				 {"id": "vol-2", "name": "vm1-disk0", "status": "available", "size": 10}
			 ]
		 }`))
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	_, err := osm_os.GetVolumeID(createMockProvider(), "vm1", "disk0")
	if err == nil {
		t.Fatal("expected error for multiple volumes")
	}
}

// Test 10: IsVolumeConverted returns true
func TestIsVolumeConvertedTrue(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/volumes/vol-123", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			 "volume": {
				 "id": "vol-123",
				 "name": "test",
				 "status": "available",
				 "size": 10,
				 "metadata": {"converted": "true"}
			 }
		 }`))
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	converted, err := osm_os.IsVolumeConverted(createMockProvider(), "vol-123")
	if err != nil {
		t.Fatalf("IsVolumeConverted returned error: %v", err)
	}
	if !converted {
		t.Error("expected true but got false")
	}
}

// Test 11: IsVolumeConverted returns false
func TestIsVolumeConvertedFalse(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/volumes/vol-123", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			 "volume": {
				 "id": "vol-123",
				 "name": "test",
				 "status": "available",
				 "size": 10,
				 "metadata": {"converted": "false"}
			 }
		 }`))
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	converted, err := osm_os.IsVolumeConverted(createMockProvider(), "vol-123")
	if err != nil {
		t.Fatalf("IsVolumeConverted returned error: %v", err)
	}
	if converted {
		t.Error("expected false but got true")
	}
}

// Test 12: IsVolumeConverted no metadata key
func TestIsVolumeConvertedNoKey(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/volumes/vol-123", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			 "volume": {
				 "id": "vol-123",
				 "name": "test",
				 "status": "available",
				 "size": 10,
				 "metadata": {}
			 }
		 }`))
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	converted, err := osm_os.IsVolumeConverted(createMockProvider(), "vol-123")
	if err != nil {
		t.Fatalf("IsVolumeConverted returned error: %v", err)
	}
	if converted {
		t.Error("expected false but got true")
	}
}

// Test 13: GetOSChangeID success
func TestGetOSChangeIDSuccess(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/volumes/vol-123", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			 "volume": {
				 "id": "vol-123",
				 "name": "test",
				 "status": "available",
				 "size": 10,
				 "metadata": {"changeID": "change-abc-123"}
			 }
		 }`))
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	changeID, err := osm_os.GetOSChangeID(createMockProvider(), "vol-123")
	if err != nil {
		t.Fatalf("GetOSChangeID returned error: %v", err)
	}
	if changeID != "change-abc-123" {
		t.Errorf("expected 'change-abc-123', got '%s'", changeID)
	}
}

// Test 14: GetOSChangeID no changeID key
func TestGetOSChangeIDNoKey(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/volumes/vol-123", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			 "volume": {
				 "id": "vol-123",
				 "name": "test",
				 "status": "available",
				 "size": 10,
				 "metadata": {}
			 }
		 }`))
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	changeID, err := osm_os.GetOSChangeID(createMockProvider(), "vol-123")
	if err != nil {
		t.Fatalf("GetOSChangeID returned error: %v", err)
	}
	if changeID != "" {
		t.Errorf("expected empty string, got '%s'", changeID)
	}
}

// Test 15: DeleteVolume success when already available
func TestDeleteVolumeSuccessAvailable(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	// Mock GET for status check
	th.Mux.HandleFunc("/volumes/vol-123", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"volume": {
					"id": "vol-123",
					"name": "test",
					"status": "available",
					"size": 10
				}
			}`))
		case http.MethodDelete:
			w.WriteHeader(http.StatusAccepted)
		}
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	err := osm_os.DeleteVolume(createMockProvider(), "vol-123")
	if err != nil {
		t.Fatalf("DeleteVolume returned error: %v", err)
	}
}

// Test 16: DeleteVolume success when in error state
func TestDeleteVolumeSuccessError(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/volumes/vol-123", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"volume": {
					"id": "vol-123",
					"name": "test",
					"status": "error",
					"size": 10
				}
			}`))
		case http.MethodDelete:
			w.WriteHeader(http.StatusAccepted)
		}
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	err := osm_os.DeleteVolume(createMockProvider(), "vol-123")
	if err != nil {
		t.Fatalf("DeleteVolume returned error: %v", err)
	}
}

// Test 17: DeleteServer success
func TestDeleteServerSuccess(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/servers/srv-123", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("Expected DELETE but got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	err := osm_os.DeleteServer(createMockProvider(), "srv-123")
	if err != nil {
		t.Fatalf("DeleteServer returned error: %v", err)
	}
}

// Test 18: DeleteServer not found
func TestDeleteServerNotFound(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/servers/nonexistent", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	err := osm_os.DeleteServer(createMockProvider(), "nonexistent")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

// Test 19: DeleteFlavor success
func TestDeleteFlavorSuccess(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/flavors/flv-123", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("Expected DELETE but got %s", r.Method)
		}
		w.WriteHeader(http.StatusAccepted)
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	err := osm_os.DeleteFlavor(createMockProvider(), "flv-123")
	if err != nil {
		t.Fatalf("DeleteFlavor returned error: %v", err)
	}
}

// Test 20: DeleteFlavor not found
func TestDeleteFlavorNotFound(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/flavors/nonexistent", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	err := osm_os.DeleteFlavor(createMockProvider(), "nonexistent")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

// Test 21: GetFlavorInfo success by ID
func TestGetFlavorInfoByIDSuccess(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/flavors/flv-123", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			 "flavor": {
				 "id": "flv-123",
				 "name": "m1.small",
				 "vcpus": 2,
				 "ram": 2048,
				 "disk": 20
			 }
		 }`))
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	flavor, err := osm_os.GetFlavorInfo(createMockProvider(), "flv-123")
	if err != nil {
		t.Fatalf("GetFlavorInfo returned error: %v", err)
	}
	if flavor.ID != "flv-123" {
		t.Errorf("expected ID 'flv-123', got '%s'", flavor.ID)
	}
	if flavor.Name != "m1.small" {
		t.Errorf("expected name 'm1.small', got '%s'", flavor.Name)
	}
}

// Test 22: GetFlavorInfo by name when ID fails
func TestGetFlavorInfoByNameSuccess(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	// First call (by ID) fails
	th.Mux.HandleFunc("/flavors/m1.large", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	// List call succeeds
	th.Mux.HandleFunc("/flavors/detail", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			 "flavors": [{
				 "id": "flv-456",
				 "name": "m1.large",
				 "vcpus": 4,
				 "ram": 8192,
				 "disk": 80
			 }]
		 }`))
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	flavor, err := osm_os.GetFlavorInfo(createMockProvider(), "m1.large")
	if err != nil {
		t.Fatalf("GetFlavorInfo returned error: %v", err)
	}
	if flavor.Name != "m1.large" {
		t.Errorf("expected name 'm1.large', got '%s'", flavor.Name)
	}
}

// Test 23: GetFlavorInfo not found
func TestGetFlavorInfoNotFound(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/flavors/nonexistent", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	th.Mux.HandleFunc("/flavors/detail", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"flavors": []}`))
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	_, err := osm_os.GetFlavorInfo(createMockProvider(), "nonexistent")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

// Test 24: UpdateVolumeMetadata success
func TestUpdateVolumeMetadataSuccess(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/volumes/vol-123", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("Expected PUT but got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			 "volume": {
				 "id": "vol-123",
				 "name": "test",
				 "status": "available",
				 "size": 10,
				 "metadata": {"key": "value"}
			 }
		 }`))
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	metadata := map[string]string{"key": "value"}
	err := osm_os.UpdateVolumeMetadata(createMockProvider(), "vol-123", metadata)
	if err != nil {
		t.Fatalf("UpdateVolumeMetadata returned error: %v", err)
	}
}

// Test 25: UpdateVolumeMetadata volume not found
func TestUpdateVolumeMetadataNotFound(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/volumes/nonexistent", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	metadata := map[string]string{"key": "value"}
	err := osm_os.UpdateVolumeMetadata(createMockProvider(), "nonexistent", metadata)
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

// Test 26: CreateServer success
func TestCreateServerSuccess(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("Expected POST but got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{
			 "server": {
				 "id": "srv-new-123",
				 "name": "test-server",
				 "status": "BUILD"
			 }
		 }`))
	})

	// Mock for WaitForServerStatus
	th.Mux.HandleFunc("/servers/srv-new-123", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			 "server": {
				 "id": "srv-new-123",
				 "name": "test-server",
				 "status": "ACTIVE"
			 }
		 }`))
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	args := osm_os.ServerArgs{
		Name:       "test-server",
		Flavor:     "m1.small",
		BootVolume: "vol-boot",
		Nics:       []interface{}{map[string]interface{}{"net-id": "net-123"}},
	}

	serverID, err := osm_os.CreateServer(createMockProvider(), args)
	if err != nil {
		t.Fatalf("CreateServer returned error: %v", err)
	}
	if serverID != "srv-new-123" {
		t.Errorf("expected 'srv-new-123', got '%s'", serverID)
	}
}

// Test 27: CreateServer API failure
func TestCreateServerAPIFailure(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "invalid request"}`))
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	args := osm_os.ServerArgs{
		Name:       "test-server",
		Flavor:     "invalid",
		BootVolume: "vol-boot",
	}

	_, err := osm_os.CreateServer(createMockProvider(), args)
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

// Test 28: CinderManage success
func TestCinderManageSuccess(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/manageable_volumes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("Expected POST but got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{
			 "volume": {
				 "id": "managed-vol-123",
				 "name": "existing-volume",
				 "status": "available"
			 }
		 }`))
	})

	// Mock for GetVolume after manage
	th.Mux.HandleFunc("/volumes/managed-vol-123", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			 "volume": {
				 "id": "managed-vol-123",
				 "name": "existing-volume",
				 "status": "available",
				 "size": 100
			 }
		 }`))
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	volume, err := osm_os.CinderManage(createMockProvider(), "existing-volume", "host@backend#pool")
	if err != nil {
		t.Fatalf("CinderManage returned error: %v", err)
	}
	if volume.ID != "managed-vol-123" {
		t.Errorf("expected 'managed-vol-123', got '%s'", volume.ID)
	}
}

// Test 29: IsVolumeConverted client init failure
func TestIsVolumeConvertedClientInitFailure(t *testing.T) {
	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	_, err := osm_os.IsVolumeConverted(createFailingProvider(), "vol-123")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

// Test 30: GetOSChangeID client init failure
func TestGetOSChangeIDClientInitFailure(t *testing.T) {
	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	_, err := osm_os.GetOSChangeID(createFailingProvider(), "vol-123")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

// Test 31: GetVolumeID client init failure
func TestGetVolumeIDClientInitFailure(t *testing.T) {
	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	_, err := osm_os.GetVolumeID(createFailingProvider(), "vm", "disk")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

// Test 32: DeleteVolume client init failure
func TestDeleteVolumeClientInitFailure(t *testing.T) {
	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	err := osm_os.DeleteVolume(createFailingProvider(), "vol-123")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

// Test 33: DeleteServer client init failure
func TestDeleteServerClientInitFailure(t *testing.T) {
	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	err := osm_os.DeleteServer(createFailingProvider(), "srv-123")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

// Test 34: DeleteFlavor client init failure
func TestDeleteFlavorClientInitFailure(t *testing.T) {
	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	err := osm_os.DeleteFlavor(createFailingProvider(), "flv-123")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

// Test 35: GetFlavorInfo client init failure
func TestGetFlavorInfoClientInitFailure(t *testing.T) {
	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	_, err := osm_os.GetFlavorInfo(createFailingProvider(), "flv-123")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

// Test 36: UpdateVolumeMetadata client init failure
func TestUpdateVolumeMetadataClientInitFailure(t *testing.T) {
	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	err := osm_os.UpdateVolumeMetadata(createFailingProvider(), "vol-123", nil)
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

// Test 37: CreateServer client init failure
func TestCreateServerClientInitFailure(t *testing.T) {
	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	_, err := osm_os.CreateServer(createFailingProvider(), osm_os.ServerArgs{})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

// Test 38: GetVolumeInfo client init failure
func TestGetVolumeInfoClientInitFailure(t *testing.T) {
	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	_, err := osm_os.GetVolumeInfo(createFailingProvider(), "vol-123")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

// Test 39: WaitForVolumeStatus success - status already matches
func TestWaitForVolumeStatusAlreadyMatches(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/volumes/vol-123", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"volume": {
				"id": "vol-123",
				"name": "test-volume",
				"status": "available",
				"size": 10
			}
		}`))
	})

	provider := createMockProvider()
	client, _ := openstack.NewBlockStorageV3(provider, gophercloud.EndpointOpts{})

	err := osm_os.WaitForVolumeStatus(client, "vol-123", "available", 3)
	if err != nil {
		t.Fatalf("WaitForVolumeStatus failed: %v", err)
	}
}

// Test 40: WaitForVolumeStatus success - status transitions after delay
func TestWaitForVolumeStatusTransitions(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	callCount := 0
	th.Mux.HandleFunc("/volumes/vol-456", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		status := "creating"
		if callCount >= 3 {
			status = "available" // Transition after 2 polls
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{
			"volume": {
				"id": "vol-456",
				"name": "test-volume",
				"status": "%s",
				"size": 10
			}
		}`, status)
	})

	provider := createMockProvider()
	client, _ := openstack.NewBlockStorageV3(provider, gophercloud.EndpointOpts{})

	err := osm_os.WaitForVolumeStatus(client, "vol-456", "available", 5)
	if err != nil {
		t.Fatalf("WaitForVolumeStatus failed: %v", err)
	}
	if callCount < 3 {
		t.Errorf("expected at least 3 calls, got %d", callCount)
	}
}

// Test 41: WaitForVolumeStatus timeout
func TestWaitForVolumeStatusTimeout(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/volumes/vol-789", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Always return "creating" - never transitions
		_, _ = w.Write([]byte(`{
			"volume": {
				"id": "vol-789",
				"name": "test-volume",
				"status": "creating",
				"size": 10
			}
		}`))
	})

	provider := createMockProvider()
	client, _ := openstack.NewBlockStorageV3(provider, gophercloud.EndpointOpts{})

	err := osm_os.WaitForVolumeStatus(client, "vol-789", "available", 2)
	if err == nil {
		t.Fatal("expected timeout error but got nil")
	}
	if !strings.Contains(err.Error(), "did not reach status") {
		t.Errorf("expected timeout error, got: %v", err)
	}
}

// Test 42: WaitForVolumeStatus API error during poll
func TestWaitForVolumeStatusAPIError(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/volumes/vol-error", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "internal server error"}`))
	})

	provider := createMockProvider()
	client, _ := openstack.NewBlockStorageV3(provider, gophercloud.EndpointOpts{})

	err := osm_os.WaitForVolumeStatus(client, "vol-error", "available", 3)
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

// Test 43: WaitForServerStatus success - status already matches
func TestWaitForServerStatusAlreadyMatches(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/servers/srv-123", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"server": {
				"id": "srv-123",
				"name": "test-server",
				"status": "ACTIVE"
			}
		}`))
	})

	provider := createMockProvider()
	client, _ := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{})

	err := osm_os.WaitForServerStatus(client, "srv-123", "ACTIVE", 3)
	if err != nil {
		t.Fatalf("WaitForServerStatus failed: %v", err)
	}
}

// Test 44: WaitForServerStatus success - status transitions
func TestWaitForServerStatusTransitions(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	callCount := 0
	th.Mux.HandleFunc("/servers/srv-456", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		status := "BUILD"
		if callCount >= 3 {
			status = "ACTIVE" // Transition after 2 polls
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{
			"server": {
				"id": "srv-456",
				"name": "test-server",
				"status": "%s"
			}
		}`, status)
	})

	provider := createMockProvider()
	client, _ := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{})

	err := osm_os.WaitForServerStatus(client, "srv-456", "ACTIVE", 5)
	if err != nil {
		t.Fatalf("WaitForServerStatus failed: %v", err)
	}
	if callCount < 3 {
		t.Errorf("expected at least 3 calls, got %d", callCount)
	}
}

// Test 45: WaitForServerStatus early exit on ERROR state
func TestWaitForServerStatusErrorState(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	callCount := 0
	th.Mux.HandleFunc("/servers/srv-error", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Server immediately in ERROR state
		_, _ = w.Write([]byte(`{
			"server": {
				"id": "srv-error",
				"name": "failed-server",
				"status": "ERROR"
			}
		}`))
	})

	provider := createMockProvider()
	client, _ := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{})

	err := osm_os.WaitForServerStatus(client, "srv-error", "ACTIVE", 10)
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !strings.Contains(err.Error(), "ERROR state") {
		t.Errorf("expected ERROR state error, got: %v", err)
	}
	// Should fail on first call, not wait for timeout
	if callCount > 2 {
		t.Errorf("expected 1-2 calls (early exit), but got %d calls", callCount)
	}
}

// Test 46: WaitForServerStatus timeout
func TestWaitForServerStatusTimeout(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/servers/srv-stuck", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Always return BUILD - never transitions
		_, _ = w.Write([]byte(`{
			"server": {
				"id": "srv-stuck",
				"name": "stuck-server",
				"status": "BUILD"
			}
		}`))
	})

	provider := createMockProvider()
	client, _ := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{})

	err := osm_os.WaitForServerStatus(client, "srv-stuck", "ACTIVE", 2)
	if err == nil {
		t.Fatal("expected timeout error but got nil")
	}
	if !strings.Contains(err.Error(), "did not reach status") {
		t.Errorf("expected timeout error, got: %v", err)
	}
}

// Test 47: WaitForServerStatus API error during poll
func TestWaitForServerStatusAPIError(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/servers/srv-fail", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "internal server error"}`))
	})

	provider := createMockProvider()
	client, _ := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{})

	err := osm_os.WaitForServerStatus(client, "srv-fail", "ACTIVE", 3)
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

// Test 48: OpenstackAuth success with environment variables
func TestOpenstackAuthWithEnvVars(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/auth/tokens" && r.Method == http.MethodPost {
			w.Header().Set("X-Subject-Token", "fake-token-123")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{
				"token": {
					"catalog": [{
						"type": "identity",
						"endpoints": [{
							"url": "http://` + r.Host + `/v3",
							"interface": "public",
							"region": "RegionOne"
						}]
					}]
				}
			}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_ = os.Setenv("OS_AUTH_URL", server.URL+"/v3")
	_ = os.Setenv("OS_USERNAME", "testuser")
	_ = os.Setenv("OS_PASSWORD", "testpass")
	_ = os.Setenv("OS_PROJECT_NAME", "testproject")
	_ = os.Setenv("OS_PROJECT_ID", "testprojectid")
	_ = os.Setenv("OS_USER_DOMAIN_NAME", "Default")
	_ = os.Setenv("OS_PROJECT_DOMAIN_NAME", "Default")
	_ = os.Setenv("OS_DOMAIN_NAME", "Default")
	defer func() {
		_ = os.Unsetenv("OS_AUTH_URL")
		_ = os.Unsetenv("OS_USERNAME")
		_ = os.Unsetenv("OS_PASSWORD")
		_ = os.Unsetenv("OS_PROJECT_NAME")
		_ = os.Unsetenv("OS_PROJECT_ID")
		_ = os.Unsetenv("OS_USER_DOMAIN_NAME")
		_ = os.Unsetenv("OS_PROJECT_DOMAIN_NAME")
		_ = os.Unsetenv("OS_DOMAIN_NAME")
	}()

	provider, err := osm_os.OpenstackAuth(context.Background(), osm_os.DstCloud{})
	if err != nil {
		t.Fatalf("OpenstackAuth failed: %v", err)
	}
	if provider == nil || provider.TokenID == "" {
		t.Fatal("expected provider with valid token")
	}
}

// Test 49: OpenstackAuth success with explicit credentials
func TestOpenstackAuthWithExplicitCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/auth/tokens" && r.Method == http.MethodPost {
			w.Header().Set("X-Subject-Token", "fake-token-456")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{
				"token": {
					"catalog": [{
						"type": "identity",
						"endpoints": [{
							"url": "http://` + r.Host + `/v3",
							"interface": "public",
							"region": "RegionOne"
						}]
					}]
				}
			}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_ = os.Unsetenv("OS_AUTH_URL")

	dstCloud := osm_os.DstCloud{
		Auth: osm_os.Auth{
			AuthURL:        server.URL + "/v3",
			Username:       "explicituser",
			Password:       "explicitpass",
			ProjectName:    "explicitproject",
			UserDomainName: "Default",
		},
		RegionName: "RegionOne",
	}

	provider, err := osm_os.OpenstackAuth(context.Background(), dstCloud)
	if err != nil {
		t.Fatalf("OpenstackAuth failed: %v", err)
	}
	if provider == nil {
		t.Fatal("expected provider but got nil")
		return
	}
	if provider.TokenID == "" {
		t.Error("expected token but got empty string")
	}
}

// Test 50: OpenstackAuth failure - invalid credentials
func TestOpenstackAuthInvalidCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/auth/tokens" && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{
				"error": {
					"message": "The request you have made requires authentication.",
					"code": 401,
					"title": "Unauthorized"
				}
			}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_ = os.Unsetenv("OS_AUTH_URL")

	dstCloud := osm_os.DstCloud{
		Auth: osm_os.Auth{
			AuthURL:     server.URL + "/v3",
			Username:    "baduser",
			Password:    "badpass",
			ProjectName: "badproject",
		},
	}

	_, err := osm_os.OpenstackAuth(context.Background(), dstCloud)
	if err == nil {
		t.Fatal("expected authentication error but got nil")
	}
}

// Test 51: OpenstackAuth failure - unreachable endpoint
func TestOpenstackAuthUnreachableEndpoint(t *testing.T) {
	_ = os.Unsetenv("OS_AUTH_URL")

	dstCloud := osm_os.DstCloud{
		Auth: osm_os.Auth{
			AuthURL:     "http://invalid.endpoint.local:9999/v3",
			Username:    "testuser",
			Password:    "testpass",
			ProjectName: "testproject",
		},
	}

	_, err := osm_os.OpenstackAuth(context.Background(), dstCloud)
	if err == nil {
		t.Fatal("expected connection error but got nil")
	}
}

// Test 52: CreateVolume success - basic (non-UEFI)
func TestCreateVolumeSuccessBasic(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	// Mock POST /volumes (create)
	th.Mux.HandleFunc("/volumes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("Expected POST but got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{
			"volume": {
				"id": "vol-new-123",
				"name": "test-volume",
				"status": "creating",
				"size": 50
			}
		}`))
	})

	// Mock GET /volumes/{id} for WaitForVolumeStatus
	th.Mux.HandleFunc("/volumes/vol-new-123", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"volume": {
					"id": "vol-new-123",
					"name": "test-volume",
					"status": "available",
					"size": 50
				}
			}`))
		case http.MethodPost:
			// Handle SetBootable action
			w.WriteHeader(http.StatusOK)
		}
	})

	// Mock POST /volumes/{id}/action for SetBootable
	th.Mux.HandleFunc("/volumes/vol-new-123/action", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("Expected POST but got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	opts := osm_os.VolOpts{
		Name:             "test-volume",
		Size:             50,
		VolumeType:       "default",
		AvailabilityZone: "nova",
		Metadata:         map[string]string{"key": "value"},
	}

	volume, err := osm_os.CreateVolume(createMockProvider(), opts, false)
	if err != nil {
		t.Fatalf("CreateVolume failed: %v", err)
	}
	if volume.ID != "vol-new-123" {
		t.Errorf("expected volume ID 'vol-new-123', got '%s'", volume.ID)
	}
}
// Test 53: CreateVolume failure - API error
func TestCreateVolumeFailure(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	// Mock POST /volumes returns error
	th.Mux.HandleFunc("/volumes", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{
			"badRequest": {
				"message": "Invalid volume size",
				"code": 400
			}
		}`))
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	opts := osm_os.VolOpts{
		Name: "invalid-volume",
		Size: -1, // Invalid size
	}

	_, err := osm_os.CreateVolume(createMockProvider(), opts, false)
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}