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
	"net/http"
	"os"
	"testing"

	osm_os "vmware-migration-kit/plugins/module_utils/openstack"

	"github.com/gophercloud/gophercloud/v2"
	th "github.com/gophercloud/gophercloud/v2/testhelper"
	fake "github.com/gophercloud/gophercloud/v2/testhelper/client"
)

// Test 1: CreatePort success
func TestCreatePortSuccess(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()
	// Mock API response
	th.Mux.HandleFunc("/v2.0/ports", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("Expected POST but got %s", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
        "port": {
            "id": "12345",
            "name": "test-port",
            "network_id": "net-001",
            "mac_address": "fa:16:3e:aa:bb:cc"
        }
    }`))
	})

	// env needed by NewNetworkV2
	_ = os.Setenv("OS_REGION_NAME", "RegionOne")

	provider := &gophercloud.ProviderClient{TokenID: "dummy"}
	provider.EndpointLocator = func(_ gophercloud.EndpointOpts) (string, error) {
		return fake.ServiceClient().Endpoint, nil
	}

	securityGroups := []string{"sg-01"}
	fixedIPs := []string{}

	port, err := osm_os.CreatePort(provider, "test-port", "net-001", "fa:16:3e:aa:bb:cc", "", securityGroups, fixedIPs)
	if err != nil {
		t.Fatalf("CreatePort returned error: %v", err)
	}

	if port.ID != "12345" {
		t.Errorf("expected port ID '12345', got %s", port.ID)
	}
	if port.Name != "test-port" {
		t.Errorf("expected port name 'test-port', got %s", port.Name)
	}
	if port.NetworkID != "net-001" {
		t.Errorf("expected network 'net-001', got %s", port.NetworkID)
	}
}

// Test 2: CreatePort success with fixed IP
func TestCreatePortSuccessWithFixedIP(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()
	// Mock API response
	th.Mux.HandleFunc("/v2.0/ports", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("Expected POST but got %s", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
        "port": {
            "id": "12345",
            "name": "test-port",
            "network_id": "net-001",
            "mac_address": "fa:16:3e:aa:bb:cc",
			"fixed_ips": [{"ip_address": "10.0.0.1"}]
        }
    }`))
	})

	// env needed by NewNetworkV2
	_ = os.Setenv("OS_REGION_NAME", "RegionOne")

	provider := &gophercloud.ProviderClient{TokenID: "dummy"}
	provider.EndpointLocator = func(_ gophercloud.EndpointOpts) (string, error) {
		return fake.ServiceClient().Endpoint, nil
	}

	securityGroups := []string{"sg-01"}
	fixedIPs := []string{"10.0.0.1"}

	port, err := osm_os.CreatePort(provider, "test-port", "net-001", "fa:16:3e:aa:bb:cc", "", securityGroups, fixedIPs)
	if err != nil {
		t.Fatalf("CreatePort returned error: %v", err)
	}

	if port.ID != "12345" {
		t.Errorf("expected port ID '12345', got %s", port.ID)
	}
	if port.Name != "test-port" {
		t.Errorf("expected port name 'test-port', got %s", port.Name)
	}
	if port.NetworkID != "net-001" {
		t.Errorf("expected network 'net-001', got %s", port.NetworkID)
	}
	if len(port.FixedIPs) != 1 || port.FixedIPs[0].IPAddress != "10.0.0.1" {
		t.Errorf("unexpected fixed_ips in returned port: %+v", port.FixedIPs)
	}
}

// Test 3: CreatePort client init failure
func TestCreatePortClientInitFailure(t *testing.T) {
	_ = os.Setenv("OS_REGION_NAME", "RegionOne")

	provider := &gophercloud.ProviderClient{}
	provider.EndpointLocator = func(_ gophercloud.EndpointOpts) (string, error) {
		return "", gophercloud.ErrEndpointNotFound{}
	}

	_, err := osm_os.CreatePort(provider, "p1", "net-001", "fa:16:3e:00:00:00", "", nil, nil)
	if err == nil {
		t.Fatalf("expected error but got nil")
	}
}

// Test 4: CreatePort API error
func TestCreatePortCreateError(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()
	th.Mux.HandleFunc("/ports", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "bad request"}`))
	})

	provider := &gophercloud.ProviderClient{TokenID: "dummy"}
	provider.EndpointLocator = func(_ gophercloud.EndpointOpts) (string, error) {
		return fake.ServiceClient().Endpoint, nil
	}

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")

	_, err := osm_os.CreatePort(provider, "bad", "net-001", "fa:16:3e:bb:cc:dd", "", nil, nil)
	if err == nil {
		t.Fatalf("expected Create error but got none")
	}
}

// ============================================================================
// GetNetwork Tests
// ============================================================================

// Test 5: GetNetwork by ID success
func TestGetNetworkByIDSuccess(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/networks/net-uuid-123", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"network": {
				"id": "net-uuid-123",
				"name": "my-network",
				"status": "ACTIVE",
				"subnets": ["subnet-1", "subnet-2"]
			}
		}`))
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	provider := &gophercloud.ProviderClient{TokenID: "dummy"}
	provider.EndpointLocator = func(_ gophercloud.EndpointOpts) (string, error) {
		return fake.ServiceClient().Endpoint, nil
	}

	network, err := osm_os.GetNetwork(provider, "net-uuid-123")
	if err != nil {
		t.Fatalf("GetNetwork returned error: %v", err)
	}
	if network.ID != "net-uuid-123" {
		t.Errorf("expected ID 'net-uuid-123', got '%s'", network.ID)
	}
	if network.Name != "my-network" {
		t.Errorf("expected name 'my-network', got '%s'", network.Name)
	}
}

// Test 6: GetNetwork by name success (ID lookup fails, name lookup succeeds)
func TestGetNetworkByNameSuccess(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	// ID lookup fails
	th.Mux.HandleFunc("/v2.0/networks/my-network", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	// Name lookup succeeds
	th.Mux.HandleFunc("/v2.0/networks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"networks": [{
				"id": "net-uuid-456",
				"name": "my-network",
				"status": "ACTIVE",
				"subnets": ["subnet-1"]
			}]
		}`))
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	provider := &gophercloud.ProviderClient{TokenID: "dummy"}
	provider.EndpointLocator = func(_ gophercloud.EndpointOpts) (string, error) {
		return fake.ServiceClient().Endpoint, nil
	}

	network, err := osm_os.GetNetwork(provider, "my-network")
	if err != nil {
		t.Fatalf("GetNetwork returned error: %v", err)
	}
	if network.Name != "my-network" {
		t.Errorf("expected name 'my-network', got '%s'", network.Name)
	}
}

// Test 7: GetNetwork not found
func TestGetNetworkNotFound(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/networks/nonexistent", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	th.Mux.HandleFunc("/v2.0/networks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"networks": []}`))
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	provider := &gophercloud.ProviderClient{TokenID: "dummy"}
	provider.EndpointLocator = func(_ gophercloud.EndpointOpts) (string, error) {
		return fake.ServiceClient().Endpoint, nil
	}

	_, err := osm_os.GetNetwork(provider, "nonexistent")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

// Test 8: GetNetwork multiple found
func TestGetNetworkMultipleFound(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/networks/dup-name", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	th.Mux.HandleFunc("/v2.0/networks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"networks": [
				{"id": "net-1", "name": "dup-name", "status": "ACTIVE"},
				{"id": "net-2", "name": "dup-name", "status": "ACTIVE"}
			]
		}`))
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	provider := &gophercloud.ProviderClient{TokenID: "dummy"}
	provider.EndpointLocator = func(_ gophercloud.EndpointOpts) (string, error) {
		return fake.ServiceClient().Endpoint, nil
	}

	_, err := osm_os.GetNetwork(provider, "dup-name")
	if err == nil {
		t.Fatal("expected error for multiple networks")
	}
}

// Test 9: GetNetwork client init failure
func TestGetNetworkClientInitFailure(t *testing.T) {
	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	provider := &gophercloud.ProviderClient{}
	provider.EndpointLocator = func(_ gophercloud.EndpointOpts) (string, error) {
		return "", gophercloud.ErrEndpointNotFound{}
	}

	_, err := osm_os.GetNetwork(provider, "net-123")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

// ============================================================================
// GetSubnetIDFromNetwork Tests
// ============================================================================

// Test 10: GetSubnetIDFromNetwork success
func TestGetSubnetIDFromNetworkSuccess(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/networks/net-123", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"network": {
				"id": "net-123",
				"name": "test-net",
				"status": "ACTIVE",
				"subnets": ["subnet-aaa", "subnet-bbb"]
			}
		}`))
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	provider := &gophercloud.ProviderClient{TokenID: "dummy"}
	provider.EndpointLocator = func(_ gophercloud.EndpointOpts) (string, error) {
		return fake.ServiceClient().Endpoint, nil
	}

	subnets, err := osm_os.GetSubnetIDFromNetwork(provider, "net-123")
	if err != nil {
		t.Fatalf("GetSubnetIDFromNetwork returned error: %v", err)
	}
	if len(subnets) != 2 {
		t.Errorf("expected 2 subnets, got %d", len(subnets))
	}
	if subnets[0] != "subnet-aaa" {
		t.Errorf("expected first subnet 'subnet-aaa', got '%s'", subnets[0])
	}
}

// Test 11: GetSubnetIDFromNetwork no subnets
func TestGetSubnetIDFromNetworkNoSubnets(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/networks/net-empty", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"network": {
				"id": "net-empty",
				"name": "empty-net",
				"status": "ACTIVE",
				"subnets": []
			}
		}`))
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	provider := &gophercloud.ProviderClient{TokenID: "dummy"}
	provider.EndpointLocator = func(_ gophercloud.EndpointOpts) (string, error) {
		return fake.ServiceClient().Endpoint, nil
	}

	_, err := osm_os.GetSubnetIDFromNetwork(provider, "net-empty")
	if err == nil {
		t.Fatal("expected error for no subnets")
	}
}

// Test 12: GetSubnetIDFromNetwork network not found
func TestGetSubnetIDFromNetworkNotFound(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/networks/nonexistent", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	provider := &gophercloud.ProviderClient{TokenID: "dummy"}
	provider.EndpointLocator = func(_ gophercloud.EndpointOpts) (string, error) {
		return fake.ServiceClient().Endpoint, nil
	}

	_, err := osm_os.GetSubnetIDFromNetwork(provider, "nonexistent")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

// Test 13: GetSubnetIDFromNetwork client init failure
func TestGetSubnetIDFromNetworkClientInitFailure(t *testing.T) {
	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	provider := &gophercloud.ProviderClient{}
	provider.EndpointLocator = func(_ gophercloud.EndpointOpts) (string, error) {
		return "", gophercloud.ErrEndpointNotFound{}
	}

	_, err := osm_os.GetSubnetIDFromNetwork(provider, "net-123")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

// ============================================================================
// DeletePort Tests
// ============================================================================

// Test 14: DeletePort success
func TestDeletePortSuccess(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	deleteCallCount := 0
	th.Mux.HandleFunc("/v2.0/ports/port-123", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteCallCount++
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// GET for status check - return 404 to indicate deleted
		w.WriteHeader(http.StatusNotFound)
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	provider := &gophercloud.ProviderClient{TokenID: "dummy"}
	provider.EndpointLocator = func(_ gophercloud.EndpointOpts) (string, error) {
		return fake.ServiceClient().Endpoint, nil
	}

	err := osm_os.DeletePort(provider, "port-123")
	if err != nil {
		t.Fatalf("DeletePort returned error: %v", err)
	}
	if deleteCallCount != 1 {
		t.Errorf("expected 1 delete call, got %d", deleteCallCount)
	}
}

// Test 15: DeletePort not found
func TestDeletePortNotFound(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/ports/nonexistent", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	provider := &gophercloud.ProviderClient{TokenID: "dummy"}
	provider.EndpointLocator = func(_ gophercloud.EndpointOpts) (string, error) {
		return fake.ServiceClient().Endpoint, nil
	}

	err := osm_os.DeletePort(provider, "nonexistent")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

// Test 16: DeletePort client init failure
func TestDeletePortClientInitFailure(t *testing.T) {
	_ = os.Setenv("OS_REGION_NAME", "RegionOne")
	provider := &gophercloud.ProviderClient{}
	provider.EndpointLocator = func(_ gophercloud.EndpointOpts) (string, error) {
		return "", gophercloud.ErrEndpointNotFound{}
	}

	err := osm_os.DeletePort(provider, "port-123")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}
