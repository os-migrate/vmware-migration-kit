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
	"strings"
	"sync"
)

type FakeServer struct {
	URL    string
	server *httptest.Server
}

var mu sync.Mutex

// Keystone
type Project struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	DomainID string `json:"domain_id"`
	Enabled  bool   `json:"enabled"`
	Links    struct {
		Self string `json:"self"`
	} `json:"links"`
}

type User struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	DomainID string `json:"domain_id"`
	Enabled  bool   `json:"enabled"`
}

type Role struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

var (
	projects   = map[string]*Project{}
	users      = map[string]*User{}
	roles      = map[string]*Role{}
	projectSeq = 1
	userSeq    = 1
)

// Domain
type Domain struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Links   struct {
		Self string `json:"self"`
	} `json:"links"`
}

var domains = map[string]*Domain{
	"default":    {ID: "default", Name: "default"},
	"osm_domain": {ID: "osm_domain", Name: "osm_domain"},
}

// Nova
type Server struct {
	ID       string              `json:"id"`
	Name     string              `json:"name"`
	Status   string              `json:"status"`
	Flavor   map[string]string   `json:"flavor"`
	Image    map[string]string   `json:"image"`
	Networks []map[string]string `json:"addresses"`
}

var servers = map[string]*Server{}
var serverID = 1

var flavors = []map[string]interface{}{
	{"id": "1", "name": "small", "ram": 2048, "vcpus": 1, "disk": 20},
	{"id": "2", "name": "medium", "ram": 4096, "vcpus": 2, "disk": 40},
}

var novaSecGroups = []map[string]interface{}{
	{"id": "sg-1", "name": "default"},
}

// Neutron
var networks = []map[string]interface{}{
	{"id": "net-1", "name": "private"},
}
var subnets = []map[string]interface{}{}
var ports = []map[string]interface{}{
	{
		"id":                "port-sriov",
		"network_id":        "net-1",
		"binding:vnic_type": "direct",
	},
}

var neutronSecGroups = []map[string]interface{}{
	{"id": "nsg-1", "name": "default"},
}

// Cinder
type Volume struct {
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	Size        int                      `json:"size"`
	Status      string                   `json:"status"`
	Attachments []map[string]interface{} `json:"attachments"`
}

var volumes = map[string]*Volume{}
var volumeID = 1

// Glance
var images = []map[string]interface{}{
	{"id": "img-1", "name": "cirros", "status": "active"},
}

func buildAddresses(networks []map[string]string) map[string][]map[string]interface{} {
	addresses := map[string][]map[string]interface{}{}

	for i, net := range networks {
		netName := "private"
		if netID, ok := net["uuid"]; ok {
			netName = netID
		}

		addresses[netName] = append(addresses[netName], map[string]interface{}{
			"addr":                    fmt.Sprintf("192.168.0.%d", 10+i),
			"version":                 4,
			"OS-EXT-IPS:type":         "fixed",
			"OS-EXT-IPS-MAC:mac_addr": "fa:16:3e:00:00:01",
		})
	}

	return addresses
}

func buildAddressesFromPorts(ports []map[string]interface{}) map[string][]map[string]interface{} {
	addresses := map[string][]map[string]interface{}{}

	for _, p := range ports {
		netID := p["network_id"].(string)
		fixedIPs, _ := p["fixed_ips"].([]interface{})

		for _, f := range fixedIPs {
			ip := f.(map[string]interface{})["ip_address"].(string)

			addresses[netID] = append(addresses[netID], map[string]interface{}{
				"addr":            ip,
				"version":         4,
				"OS-EXT-IPS:type": "fixed",
			})
		}
	}
	return addresses
}

func NewFakeServer() *FakeServer {
	fs := &FakeServer{}
	mux := http.NewServeMux()

	/* Root */
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"version": map[string]string{"id": "v3.0"},
		})
	})

	/* ============================
	   Fake Keystone
	============================ */

	roles["member"] = &Role{ID: "member", Name: "member"}
	roles["admin"] = &Role{ID: "admin", Name: "admin"}
	projects["demo"] = &Project{ID: "demo", Name: "demo"}

	mux.HandleFunc("/v3", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"version": map[string]interface{}{
				"id":      "v3.14",
				"status":  "stable",
				"updated": "2023-01-01T00:00:00Z",
				"links": []map[string]string{
					{
						"rel":  "self",
						"href": fs.URL + "/v3/",
					},
				},
			},
		})
	})
	/* --- Keystone auth + catalogs --- */
	mux.HandleFunc("/v3/auth/tokens", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Subject-Token", "fake-token")
		w.WriteHeader(http.StatusCreated)
		// Service catalog for identity, compute, network, image, volume
		catalog := []map[string]interface{}{
			{
				"type": "identity",
				"name": "keystone",
				"endpoints": []map[string]interface{}{
					{
						"id":        "keystone-public",
						"interface": "public",
						"region":    "RegionOne",
						"region_id": "RegionOne",
						"url":       fs.URL + "/v3",
					},
				},
			},
			{
				"type": "compute",
				"name": "nova",
				"endpoints": []map[string]interface{}{
					{
						"id":        "nova-public",
						"interface": "public",
						"region":    "RegionOne",
						"region_id": "RegionOne",
						"url":       fs.URL + "/v2.1/demo",
					},
				},
			},
			{
				"type": "network",
				"name": "neutron",
				"endpoints": []map[string]interface{}{
					{
						"id":        "neutron-public",
						"interface": "public",
						"region":    "RegionOne",
						"region_id": "RegionOne",
						"url":       fs.URL + "/v2.0",
					},
				},
			},
			{
				"type": "image",
				"name": "glance",
				"endpoints": []map[string]interface{}{
					{
						"id":        "glance-public",
						"interface": "public",
						"region":    "RegionOne",
						"region_id": "RegionOne",
						"url":       fs.URL + "/v2",
					},
				},
			},
			{
				"type": "volumev3",
				"name": "cinder",
				"endpoints": []map[string]interface{}{
					{
						"id":        "cinder-public",
						"interface": "public",
						"region":    "RegionOne",
						"region_id": "RegionOne",
						"url":       fs.URL + "/v3/demo",
					},
				},
			},
		}
		// Fake token response
		json.NewEncoder(w).Encode(map[string]interface{}{
			"token": map[string]interface{}{
				"expires_at": "2099-01-01T00:00:00.000000Z",
				"issued_at":  "2026-01-01T00:00:00.000000Z",
				"project": map[string]string{
					"id":   "demo",
					"name": "demo",
				},
				"user": map[string]string{
					"id":   "demo-user",
					"name": "demo",
				},
				"catalog": catalog,
			},
		})

	})

	/* --- Projects --- */
	mux.HandleFunc("/v3/projects", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodGet {
			list := []interface{}{}
			// for _, p := range projects {
			// 	list = append(list, p)
			// }
			name := r.URL.Query().Get("name")

			for _, p := range projects {
				if name == "" || p.Name == name {
					list = append(list, p)
				}
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"projects": list,
			})
			return
		}

		if r.Method == http.MethodPost {
			var req struct {
				Project struct {
					Name string `json:"name"`
				} `json:"project"`
			}
			json.NewDecoder(r.Body).Decode(&req)

			id := fmt.Sprintf("proj-%d", projectSeq)
			projectSeq++

			p := &Project{ID: id, Name: req.Project.Name}
			projects[id] = p

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"project": p,
			})
		}
	})
	mux.HandleFunc("/v3/projects/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/users/") {
			return // role assignment handler already exists
		}

		id := strings.TrimPrefix(r.URL.Path, "/v3/projects/")
		p, ok := projects[id]
		if !ok {
			http.NotFound(w, r)
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"project": p,
		})
	})
	/* --- Users --- */
	mux.HandleFunc("/v3/users", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodGet {
			list := []interface{}{}
			// for _, u := range users {
			// 	list = append(list, u)
			// }
			name := r.URL.Query().Get("name")
			domainID := r.URL.Query().Get("domain_id")

			for _, u := range users {
				if name != "" && u.Name != name {
					continue
				}
				if domainID != "" && u.DomainID != domainID {
					continue
				}
				list = append(list, u)
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"users": list,
			})
			return
		}

		if r.Method == http.MethodPost {
			// req
			var req struct {
				User struct {
					Name             string `json:"name"`
					DomainID         string `json:"domain_id"`
					DefaultProjectID string `json:"default_project_id"`
				} `json:"user"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			fmt.Println(req)
			id := fmt.Sprintf("user-%d", userSeq)
			userSeq++

			u := &User{
				ID:       id,
				Name:     req.User.Name,
				DomainID: req.User.DomainID,
				Enabled:  true,
			}
			users[id] = u

			w.WriteHeader(http.StatusCreated)
			// resp
			json.NewEncoder(w).Encode(map[string]interface{}{
				"user": map[string]interface{}{
					"id":                 u.ID,
					"name":               u.Name,
					"domain_id":          u.DomainID,
					"default_project_id": req.User.DefaultProjectID,
					"enabled":            true,
				},
			})
		}
	})
	/* /v3/users/{user_id} PATCH */
	mux.HandleFunc("/v3/users/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"user": map[string]interface{}{
				"id": strings.TrimPrefix(r.URL.Path, "/v3/users/"),
			},
		})
	})

	/* --- Roles --- */
	mux.HandleFunc("/v3/roles", func(w http.ResponseWriter, r *http.Request) {
		list := []interface{}{}
		for _, r := range roles {
			list = append(list, r)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"roles": list,
		})
	})

	// mux.HandleFunc("/v3/projects/", func(w http.ResponseWriter, r *http.Request) {
	// 	if !strings.Contains(r.URL.Path, "/users/") {
	// 		return
	// 	}
	// 	if r.Method != http.MethodPut {
	// 		return
	// 	}

	// 	// We don't store assignments — just acknowledge
	// 	w.WriteHeader(http.StatusNoContent)
	// })

	/* domains */
	mux.HandleFunc("/v3/domains", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		list := []interface{}{}

		for _, d := range domains {
			if name == "" || d.Name == name {
				list = append(list, d)
			}
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"domains": list,
		})
	})

	mux.HandleFunc("/v3/domains/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v3/domains/")
		d, ok := domains[id]
		if !ok {
			http.NotFound(w, r)
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"domain": d,
		})
	})
	/* ============================
	   Fake Nova
	============================ */
	mux.HandleFunc("/v2.1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"version": map[string]interface{}{
				"id":          "v2.1",
				"status":      "CURRENT",
				"min_version": "2.1",
				"version":     "2.90",
				"updated":     "2023-01-01T00:00:00Z",
				"links": []map[string]string{
					{
						"rel":  "self",
						"href": fs.URL + "/v2.1/",
					},
				},
			},
		})
	})
	mux.HandleFunc("/v2.1/demo/servers", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodGet {
			list := []interface{}{}
			for _, s := range servers {
				list = append(list, map[string]interface{}{
					"id":        s.ID,
					"name":      s.Name,
					"status":    s.Status,
					"flavor":    s.Flavor,
					"image":     s.Image,
					"addresses": buildAddresses(s.Networks),
				})
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"servers": list})
			return
		}

		// if r.Method == http.MethodGet {
		// 	list := []interface{}{}
		// 	for _, s := range servers {
		// 		list = append(list, s)
		// 	}
		// 	json.NewEncoder(w).Encode(map[string]interface{}{"servers": list})
		// 	return
		// }

		if r.Method == http.MethodPost {
			var req struct {
				Server struct {
					Name      string              `json:"name"`
					FlavorRef string              `json:"flavorRef"`
					ImageRef  string              `json:"imageRef"`
					Networks  []map[string]string `json:"networks"`
				} `json:"server"`
			}
			json.NewDecoder(r.Body).Decode(&req)

			id := fmt.Sprintf("%d", serverID)
			serverID++

			s := &Server{
				ID:       id,
				Name:     req.Server.Name,
				Status:   "ACTIVE",
				Flavor:   map[string]string{"id": req.Server.FlavorRef},
				Image:    map[string]string{"id": req.Server.ImageRef},
				Networks: req.Server.Networks,
			}
			servers[id] = s

			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(map[string]interface{}{"server": s})
		}
	})

	mux.HandleFunc("/v2.1/demo/servers/detail", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		list := []interface{}{}
		for _, s := range servers {
			list = append(list, s)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"servers": list,
		})
	})

	var keypairs = []map[string]interface{}{}

	mux.HandleFunc("/v2.1/demo/os-keypairs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		defer mu.Unlock()

		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"keypairs": keypairs,
			})
			return
		}

		if r.Method == http.MethodPost {
			var req struct {
				Keypair map[string]interface{} `json:"keypair"`
			}
			json.NewDecoder(r.Body).Decode(&req)

			kp := req.Keypair
			kp["fingerprint"] = "fake:fingerprint"
			kp["created_at"] = "2026-01-01T00:00:00Z"

			keypairs = append(keypairs, map[string]interface{}{
				"keypair": kp,
			})

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"keypair": kp,
			})
		}
	})
	mux.HandleFunc("/v2.1/demo/flavors", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"flavors": flavors,
		})
	})

	mux.HandleFunc("/v2.1/demo/flavors/detail", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"flavors": flavors,
		})
	})

	/*	mux.HandleFunc("/v2.1/demo/flavors", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"flavors": flavors})
	})*/

	mux.HandleFunc("/v2.1/demo/os-security-groups", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"security_groups": novaSecGroups})
	})

	/* ============================
	   Fake Neutron
	============================ */
	/* Neutron network GET and POST */
	mux.HandleFunc("/v2.0/networks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		defer mu.Unlock()

		// -----------------
		// GET /v2.0/networks
		// -----------------
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"networks": networks,
			})
			return
		}

		// -----------------
		// POST /v2.0/networks
		// -----------------
		if r.Method == http.MethodPost {
			var req struct {
				Network map[string]interface{} `json:"network"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			network := req.Network
			network["id"] = fmt.Sprintf("net-%d", len(networks)+1)

			// Neutron defaults
			if _, ok := network["admin_state_up"]; !ok {
				network["admin_state_up"] = true
			}
			if _, ok := network["status"]; !ok {
				network["status"] = "ACTIVE"
			}
			if _, ok := network["mtu"]; !ok {
				network["mtu"] = 1500
			}
			if _, ok := network["revision_number"]; !ok {
				network["revision_number"] = 1
			}
			networks = append(networks, network)

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"network": network,
			})
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})
	// GET /v2.0/networks/{id}
	mux.HandleFunc("/v2.0/networks/", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Path[len("/v2.0/networks/"):]
		mu.Lock()
		defer mu.Unlock()

		for _, n := range networks {
			if n["id"] == id {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"network": n,
				})
				return
			}
		}

		http.NotFound(w, r)
	})

	/* Subnet GET and POST */
	mux.HandleFunc("/v2.0/subnets", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		defer mu.Unlock()

		// GET /v2.0/subnets
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"subnets": subnets,
			})
			return
		}

		// POST /v2.0/subnets
		if r.Method == http.MethodPost {
			var req struct {
				Subnet map[string]interface{} `json:"subnet"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			subnet := req.Subnet
			subnet["id"] = fmt.Sprintf("subnet-%d", len(subnets)+1)
			subnet["ip_version"] = 4
			subnet["status"] = "ACTIVE"

			subnets = append(subnets, subnet)

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"subnet": subnet,
			})
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})

	/* Neutron port GET and POST */
	mux.HandleFunc("/v2.0/ports", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		defer mu.Unlock()

		// -----------------
		// GET /v2.0/ports
		// -----------------
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ports": ports,
			})
			return
		}

		// -----------------
		// POST /v2.0/ports
		// -----------------
		if r.Method == http.MethodPost {
			var req struct {
				Port map[string]interface{} `json:"port"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			port := req.Port
			port["id"] = fmt.Sprintf("port-%d", len(ports)+1)
			port["status"] = "ACTIVE"
			if _, ok := port["binding:vif_type"]; !ok {
				port["binding:vif_type"] = "hw_veb"
			}

			if vnic, ok := port["binding:vnic_type"]; ok && vnic == "direct" {
				port["binding:vif_details"] = map[string]interface{}{
					"pci_slot":         "0000:af:06.0",
					"physical_network": "physnet2",
				}
			}
			ports = append(ports, port)

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"port": port,
			})
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})
	// mux.HandleFunc("/v2.0/ports", func(w http.ResponseWriter, r *http.Request) {
	// 	json.NewEncoder(w).Encode(map[string]interface{}{"ports": ports})
	// })

	mux.HandleFunc("/v2.0/security-groups", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"security_groups": neutronSecGroups})
	})

	/* ============================
	   Fake Cinder
	============================ */
	mux.HandleFunc("/v3/demo/volumes", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		if r.Method == http.MethodGet {
			list := []interface{}{}
			for _, v := range volumes {
				list = append(list, v)
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"volumes": list})
			return
		}

		if r.Method == http.MethodPost {
			var req struct {
				Volume struct {
					Name string `json:"name"`
					Size int    `json:"size"`
				} `json:"volume"`
			}
			json.NewDecoder(r.Body).Decode(&req)

			id := fmt.Sprintf("%d", volumeID)
			volumeID++

			v := &Volume{
				ID:          id,
				Name:        req.Volume.Name,
				Size:        req.Volume.Size,
				Status:      "available",
				Attachments: []map[string]interface{}{},
			}
			volumes[id] = v

			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(map[string]interface{}{"volume": v})
		}
	})

	mux.HandleFunc("/v3/demo/volumes/detail", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		list := []interface{}{}
		for _, v := range volumes {
			list = append(list, v)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"volumes": list,
		})
	})

	/* ============================
	   Fake Glance
	============================ */
	mux.HandleFunc("/v2/images", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]interface{}{"images": images})
			return
		}

		if r.Method == http.MethodPost {
			var img map[string]interface{}
			json.NewDecoder(r.Body).Decode(&img)
			img["id"] = fmt.Sprintf("img-%d", len(images)+1)
			img["status"] = "active"
			images = append(images, img)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(img)
		}
	})

	// Start Http Fake server
	fs.server = httptest.NewServer(mux)
	fs.URL = fs.server.URL

	log.Println("Fake OpenStack running at:", fs.URL)
	os.WriteFile("/tmp/fake_os_url.txt", []byte(fs.URL), 0644)

	return fs
}

func (f *FakeServer) Close() {
	f.server.Close()
}

func main() {
	fs := NewFakeServer()
	defer fs.Close()
	select {}
}
