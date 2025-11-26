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

	port, err := osm_os.CreatePort(provider, "test-port", "net-001", "fa:16:3e:aa:bb:cc", securityGroups)
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

func TestCreatePortClientInitFailure(t *testing.T) {
	_ = os.Setenv("OS_REGION_NAME", "RegionOne")

	provider := &gophercloud.ProviderClient{}
	provider.EndpointLocator = func(_ gophercloud.EndpointOpts) (string, error) {
		return "", gophercloud.ErrEndpointNotFound{}
	}

	_, err := osm_os.CreatePort(provider, "p1", "net-001", "fa:16:3e:00:00:00", nil)
	if err == nil {
		t.Fatalf("expected error but got nil")
	}
}

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

	_, err := osm_os.CreatePort(provider, "bad", "net-001", "fa:16:3e:bb:cc:dd", nil)
	if err == nil {
		t.Fatalf("expected Create error but got none")
	}
}
