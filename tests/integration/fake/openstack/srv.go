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
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
)

type FakeServer struct {
	URL    string
	server *httptest.Server
}

var fakeFlavors = []flavors.Flavor{
	{ID: "1", Name: "small", RAM: 2048, VCPUs: 1, Disk: 20},
	{ID: "2", Name: "medium", RAM: 4096, VCPUs: 2, Disk: 40},
	{ID: "3", Name: "large", RAM: 8192, VCPUs: 4, Disk: 80},
}

// var muxServerURL string

func NewFakeServer() *FakeServer {
	fs := &FakeServer{}
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"version":{"id":"v3.0","status":"stable"}}`)); err != nil {
			log.Printf("Error writing response: %v", err)
		}
	})

	// Keystone auth + catalog
	mux.HandleFunc("/v3/auth/tokens", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("X-Subject-Token", "fake-token-123")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		resp := map[string]interface{}{
			"token": map[string]interface{}{
				"expires_at": "2099-12-31T23:59:59.000000Z",
				"project": map[string]string{
					"id":   "demo-id",
					"name": "demo",
				},
				"user": map[string]string{
					"id":   "user-id",
					"name": "demo-user",
				},
				"catalog": []map[string]interface{}{
					{
						"type": "compute",
						"name": "nova",
						"endpoints": []map[string]interface{}{
							{
								"region":    "RegionOne",
								"url":       fs.URL + "/v2.1/demo-id",
								"interface": "public",
								"id":        "fake-compute-endpoint",
							},
						},
					},
				},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("Error encoding tokens response: %v", err)
		}
	})

	// flavors list
	mux.HandleFunc("/v2.1/demo-id/flavors", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		flavorsList := []map[string]interface{}{}
		for _, f := range fakeFlavors {
			flavorsList = append(flavorsList, map[string]interface{}{
				"id":    f.ID,
				"name":  f.Name,
				"ram":   f.RAM,
				"vcpus": f.VCPUs,
				"disk":  f.Disk,
			})
		}
		if err := json.NewEncoder(w).Encode(map[string]interface{}{"flavors": flavorsList}); err != nil {
			log.Printf("Error encoding flavors list: %v", err)
		}
	})

	// flavors/detail
	mux.HandleFunc("/v2.1/demo-id/flavors/detail", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{"flavors": fakeFlavors}); err != nil {
			log.Printf("Error encoding flavors detail: %v", err)
		}
	})

	// flavor get by ID
	mux.HandleFunc("/v2.1/demo-id/flavors/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := r.URL.Path[len("/v2.1/demo-id/flavors/"):]
		for _, f := range fakeFlavors {
			if f.ID == id {
				if err := json.NewEncoder(w).Encode(map[string]interface{}{"flavor": f}); err != nil {
					log.Printf("Error encoding flavor by ID: %v", err)
				}
				return
			}
		}
		http.NotFound(w, r)
	})
	fs.server = httptest.NewServer(mux)
	fs.URL = fs.server.URL
	log.Println("Fake OpenStack server running at:", fs.URL)

	if err := os.WriteFile("/tmp/fake_os_url.txt", []byte(fs.URL), 0644); err != nil {
		log.Fatalf("Failed to write URL file: %v", err)
	}

	pid := os.Getpid()
	if err := os.WriteFile("/tmp/fake_os_server.pid", []byte(fmt.Sprintf("%d", pid)), 0644); err != nil {
		log.Fatalf("Failed to write PID file: %v", err)
	}

	return fs
}

func (f *FakeServer) Close() {
	f.server.Close()
}

func main() {
	fakeServer := NewFakeServer()
	defer fakeServer.Close()

	// Keep the server running
	select {}
}
