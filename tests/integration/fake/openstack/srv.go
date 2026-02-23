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
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

/* ============================
   Keystone
============================ */

type Domain struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type User struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	DomainID         string            `json:"domain_id"`
	DefaultProjectID string            `json:"default_project_id,omitempty"`
	Enabled          bool              `json:"enabled"`
	Federated        []interface{}     `json:"federated"`
	Links            map[string]string `json:"links"`
	PasswordExpires  *string           `json:"password_expires_at"`
}

type Project struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	DomainID string `json:"domain_id"`
	Enabled  bool   `json:"enabled"`
}

type Role struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

/* ============================
   Global
============================ */

var (
	mu sync.Mutex

	domains = map[string]*Domain{
		"default": {ID: "default", Name: "default"},
	}

	users    = map[string]*User{}
	projects = map[string]*Project{}
	roles    = map[string]*Role{
		"member": {ID: "member", Name: "member"},
		"admin":  {ID: "admin", Name: "admin"},
	}
	seq = 1
)

/* ============================
   Nova
============================ */

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

var (
	flavors = []map[string]interface{}{
		{"id": "1", "name": "small", "ram": 2048, "vcpus": 1, "disk": 20},
		{"id": "2", "name": "medium", "ram": 4096, "vcpus": 2, "disk": 40},
		{"id": "3", "name": "large", "ram": 8192, "vcpus": 4, "disk": 80},
	}

	flavorExtraSpecs = map[string]map[string]string{
		"osm_flavor": {},
	}

	// servers  = map[string]interface{}{}
	keypairs = []map[string]interface{}{}
)

/*
============================

	Cinder

============================
*/
type Volume struct {
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	Size        int                      `json:"size"`
	Status      string                   `json:"status"`
	Attachments []map[string]interface{} `json:"attachments"`
	Metadata    map[string]string        `json:"metadata"`
}

var volumes = map[string]*Volume{
	"vol-1": {ID: "vol-1", Name: "volume-1", Size: 10, Status: "available", Attachments: []map[string]interface{}{}, Metadata: map[string]string{}},
}
var volumeID = 1

/*
============================

	Glance

============================
*/
var images = []map[string]interface{}{
	{"id": "img-1", "name": "cirros", "status": "active"},
}

/* ============================
   Neutron
============================ */

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

// Utils functions
func normalizeV2Path(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for strings.HasPrefix(r.URL.Path, "/v2.0/v2.0/") {
			r.URL.Path = strings.Replace(r.URL.Path, "/v2.0/v2.0/", "/v2.0/", 1)
		}
		next.ServeHTTP(w, r)
	})
}
func randomID(prefix string) string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}

	// UUID v4-ish formatting
	return fmt.Sprintf(
		"%s-%x-%x-%x-%x-%x",
		prefix,
		b[0:4],
		b[4:6],
		b[6:8],
		b[8:10],
		b[10:16],
	)
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
func parts(p string) []string {
	return strings.Split(strings.Trim(p, "/"), "/")
}

type FakeServer struct {
	URL    string
	server *httptest.Server
}

func (f *FakeServer) Close() {
	f.server.Close()
	if err := os.Remove(path.Join("/tmp", "fake_os_server.pid")); err != nil {
		log.Printf("Failed to remove PID file: %v", err)
	}
}

func NewFakeServer() *FakeServer {
	fs := &FakeServer{}
	mux := http.NewServeMux()
	// Write PID file
	pid := os.Getpid()
	if err := os.WriteFile("/tmp/fake_os_server.pid", []byte(fmt.Sprintf("%d", pid)), 0644); err != nil {
		log.Fatalf("Failed to write PID file: %v", err)
	}

	// Default handler.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	})

	/* ============================
	   Keystone handlers
	============================ */
	mux.HandleFunc("/v3", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.String())
		log.Printf("%s %s", r.URL.Path, r.URL.RawQuery)
		err := json.NewEncoder(w).Encode(map[string]interface{}{
			"version": map[string]interface{}{
				"id":     "v3.14",
				"status": "stable",
				"links": []map[string]string{
					{"rel": "self", "href": "http://127.0.0.1:5000/v3/"},
				},
			},
		})
		if err != nil {
			log.Printf("Error encoding JSON response: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	})
	// Token and catalog def
	mux.HandleFunc("/v3/auth/tokens", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Subject-Token", "fake-token")
		w.WriteHeader(http.StatusCreated)
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
						"url":       "http://127.0.0.1:5000/v3/",
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
						"url":       "http://127.0.0.1:5000/v2.1/demo",
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
						"url":       "http://127.0.0.1:5000/v2.0",
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
						"url":       "http://127.0.0.1:5000/v2",
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
						"url":       "http://127.0.0.1:5000/v3/demo",
					},
				},
			},
		}

		// Fake token response
		err := json.NewEncoder(w).Encode(map[string]interface{}{
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
		if err != nil {
			log.Printf("Error encoding JSON response: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	/* ---- domains ---- */
	mux.HandleFunc("/v3/domains", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		log.Printf("%s %s", r.Method, r.URL.String())
		if r.Method == http.MethodGet {
			list := []interface{}{}
			for _, d := range domains {
				list = append(list, d)
			}
			err := json.NewEncoder(w).Encode(map[string]interface{}{"domains": list})
			if err != nil {
				log.Printf("Error encoding JSON response: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}

		if r.Method == http.MethodPost {
			id := fmt.Sprintf("domain-%d", seq)
			seq++
			domains[id] = &Domain{ID: id, Name: id}
			w.WriteHeader(http.StatusCreated)
			err := json.NewEncoder(w).Encode(map[string]interface{}{"domain": domains[id]})
			if err != nil {
				log.Printf("Error encoding JSON response: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/v3/domains/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		log.Printf("%s %s", r.Method, r.URL.String())
		if r.Method == http.MethodGet {
			list := []interface{}{}
			for _, d := range domains {
				list = append(list, d)
			}
			err := json.NewEncoder(w).Encode(map[string]interface{}{"domains": list})
			if err != nil {
				log.Printf("Error encoding JSON response: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}

		if r.Method == http.MethodPost {
			id := fmt.Sprintf("domain-%d", seq)
			seq++
			domains[id] = &Domain{ID: id, Name: id}
			w.WriteHeader(http.StatusCreated)
			err := json.NewEncoder(w).Encode(map[string]interface{}{"domain": domains[id]})
			if err != nil {
				log.Printf("Error encoding JSON response: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}

	})

	/* ---- projects ---- */
	mux.HandleFunc("/v3/projects", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		log.Printf("%s %s", r.Method, r.URL.String())
		if r.Method == http.MethodGet {
			name := r.URL.Query().Get("name")
			list := []interface{}{}
			for _, p := range projects {
				if name != "" && p.Name != name {
					continue
				}
				list = append(list, map[string]interface{}{
					"id":        p.ID,
					"name":      p.Name,
					"domain_id": p.DomainID,
					"enabled":   p.Enabled,
				})
			}

			err := json.NewEncoder(w).Encode(map[string]interface{}{"projects": list})
			if err != nil {
				log.Printf("Error encoding JSON response: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}

		if r.Method == http.MethodPost {
			var req struct {
				Project struct {
					Name     string `json:"name"`
					DomainID string `json:"domain_id"`
				} `json:"project"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid JSON body", http.StatusBadRequest)
				return
			}
			if req.Project.Name == "" || req.Project.DomainID == "" {
				http.Error(w, "Missing project name or domain_id", http.StatusBadRequest)
				return
			}

			id := randomID("project")
			seq++

			p := &Project{
				ID:       id,
				Name:     req.Project.Name,
				DomainID: req.Project.DomainID,
				Enabled:  true,
			}
			projects[id] = p

			w.WriteHeader(http.StatusCreated)
			err := json.NewEncoder(w).Encode(map[string]interface{}{
				"project": p,
			})
			if err != nil {
				log.Printf("Error encoding JSON response: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})

	/* role assignments */
	// role assignments: project_id -> user_id -> roles
	var roleAssignments = map[string]map[string][]string{}

	// GET roles assigned to a user in a project
	mux.HandleFunc("/v3/projects/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.String())
		pathParts := parts(r.URL.Path)

		// Match: /v3/projects/{project_id}/users/{user_id}/roles
		if len(pathParts) == 6 && pathParts[2] == "users" && pathParts[4] == "roles" && r.Method == http.MethodGet {
			projectID := pathParts[1]
			userID := pathParts[3]

			mu.Lock()
			defer mu.Unlock()

			userRoles := []map[string]string{}
			if projMap, ok := roleAssignments[projectID]; ok {
				if roleIDs, ok := projMap[userID]; ok {
					for _, roleID := range roleIDs {
						if role, ok := roles[roleID]; ok {
							userRoles = append(userRoles, map[string]string{
								"id":   role.ID,
								"name": role.Name,
							})
						}
					}
				}
			}

			err := json.NewEncoder(w).Encode(map[string]interface{}{
				"roles": userRoles,
			})
			if err != nil {
				log.Printf("Error encoding JSON response: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}

		// Match: /v3/projects/{project_id}/users/{user_id}/roles/{role_id}
		if len(pathParts) == 7 && pathParts[2] == "users" && pathParts[4] == "roles" && r.Method == http.MethodPut {
			projectID := pathParts[1]
			userID := pathParts[3]
			roleID := pathParts[5]

			mu.Lock()
			defer mu.Unlock()

			if _, ok := roleAssignments[projectID]; !ok {
				roleAssignments[projectID] = map[string][]string{}
			}
			roleAssignments[projectID][userID] = append(roleAssignments[projectID][userID], roleID)

			w.WriteHeader(http.StatusNoContent)
			return
		}
		// Match: v3/projects/{project_id}
		if len(pathParts) == 3 {
			projectID := pathParts[2]
			mu.Lock()
			defer mu.Unlock()

			p, ok := projects[projectID]
			if !ok {
				http.NotFound(w, r)
				return
			}

			if r.Method == http.MethodGet {
				err := json.NewEncoder(w).Encode(map[string]interface{}{
					"project": p,
				})
				if err != nil {
					log.Printf("Error encoding JSON response: %v", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}
				return
			}
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})

	/* ---- roles ---- */
	mux.HandleFunc("/v3/roles", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.String())
		switch r.Method {
		case http.MethodGet:
			name := r.URL.Query().Get("name")
			list := []interface{}{}
			for _, role := range roles {
				if name != "" && role.Name != name {
					continue
				}
				list = append(list, map[string]interface{}{
					"id":   role.ID,
					"name": role.Name,
				})
			}
			err := json.NewEncoder(w).Encode(map[string]interface{}{"roles": list})
			if err != nil {
				log.Printf("Error encoding JSON response: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			return

		case http.MethodPost:
			var req struct {
				Role struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"role"`
			}

			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid JSON body", http.StatusBadRequest)
				return
			}
			fmt.Println("Creating role:", req.Role)
			fmt.Println("Creating role:", req.Role.Name)
			if req.Role.Name == "" {
				http.Error(w, "Missing role id or name", http.StatusBadRequest)
				return
			}
			if req.Role.ID == "" {
				req.Role.ID = randomID("role")
			}

			rl := &Role{
				ID:   req.Role.ID,
				Name: req.Role.Name,
			}

			roles[req.Role.ID] = rl

			w.WriteHeader(http.StatusCreated)
			err := json.NewEncoder(w).Encode(map[string]interface{}{"role": rl})
			if err != nil {
				log.Printf("Error encoding JSON response: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			return

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/v3/roles/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		log.Printf("%s %s", r.Method, r.URL.String())
		id := path.Base(r.URL.Path)

		rl, ok := roles[id]
		if !ok {
			http.NotFound(w, r)
			return
		}

		switch r.Method {

		case http.MethodGet:
			err := json.NewEncoder(w).Encode(map[string]interface{}{
				"role": map[string]interface{}{
					"id":   rl.ID,
					"name": rl.Name,
				},
			})
			if err != nil {
				log.Printf("Error encoding JSON response: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			return
		// Ansible sends PUT or PATCH instead of POST.
		case http.MethodPatch, http.MethodPut:
			// Ansible just check success status.
			var req map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid JSON body", http.StatusBadRequest)
				return
			}

			// TODO Update fields as needed.
			if rReq, ok := req["role"].(map[string]interface{}); ok {
				if name, ok := rReq["name"].(string); ok {
					rl.Name = name
				}
			}

			err := json.NewEncoder(w).Encode(map[string]interface{}{
				"user": map[string]interface{}{
					"id":   rl.ID,
					"name": rl.Name,
				},
			})
			if err != nil {
				log.Printf("Error encoding JSON response: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			return

		case http.MethodDelete:
			delete(roles, id)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})

	/* ---- users ---- */
	mux.HandleFunc("/v3/users", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.String())
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodDelete {
			part := parts(r.URL.Path)
			userID := part[len(part)-1]
			delete(users, userID)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if r.Method == http.MethodGet {
			name := r.URL.Query().Get("name")
			list := []interface{}{}
			found := false
			for _, u := range users {
				if name != "" && u.Name != name {
					continue
				}
				found = true
				list = append(list, map[string]interface{}{
					"id":                 u.ID,
					"name":               u.Name,
					"domain_id":          u.DomainID,
					"default_project_id": u.DefaultProjectID,
					"enabled":            u.Enabled,
					"federated":          []interface{}{},
					"links": map[string]string{
						"self": fmt.Sprintf("http://127.0.0.1:5000/v3/users/%s", u.ID),
					},
					"password_expires_at": nil,
				})
			}
			// Workarround filtering name_or_id in the sdk:
			// If user ID is send instead of name, the filter does not work.
			if !found {
				// Then try to find by ID
				for _, u := range users {
					if name != "" && u.ID != name {
						continue
					}
					list = append(list, map[string]interface{}{
						"id":                 u.ID,
						"name":               u.Name,
						"domain_id":          u.DomainID,
						"default_project_id": u.DefaultProjectID,
						"enabled":            u.Enabled,
						"federated":          []interface{}{},
						"links": map[string]string{
							"self": fmt.Sprintf("http://127.0.0.1:5000/v3/users/%s", u.ID),
						},
						"password_expires_at": nil,
					})
				}
			}
			err := json.NewEncoder(w).Encode(map[string]interface{}{
				"users": list,
			})
			if err != nil {
				log.Printf("Error encoding JSON response: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			return
		}

		if r.Method == http.MethodPost {
			var req struct {
				User struct {
					Name     string `json:"name"`
					DomainID string `json:"domain_id"`
				} `json:"user"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid JSON body", http.StatusBadRequest)
				return
			}

			id := randomID("user")
			seq++

			u := &User{
				ID:       id,
				Name:     req.User.Name,
				DomainID: req.User.DomainID,
				Enabled:  true,
			}
			users[id] = u

			w.WriteHeader(http.StatusCreated)
			err := json.NewEncoder(w).Encode(map[string]interface{}{
				"user": map[string]interface{}{
					"id":        u.ID,
					"name":      u.Name,
					"domain_id": u.DomainID,
					"enabled":   true,
					"links": map[string]string{
						"self": "http://127.0.0.1:5000/v3/users/" + u.ID,
					},
				},
			})
			if err != nil {
				log.Printf("Error encoding JSON response: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			return
		}
	})
	// /v3/users/<id>
	mux.HandleFunc("/v3/users/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.String())
		mu.Lock()
		defer mu.Unlock()

		id := path.Base(r.URL.Path)

		u, ok := users[id]
		if !ok {
			// Keystone returns 404 here
			http.NotFound(w, r)
			return
		}

		switch r.Method {

		case http.MethodGet:
			err := json.NewEncoder(w).Encode(map[string]interface{}{
				"user": map[string]interface{}{
					"id":        u.ID,
					"name":      u.Name,
					"domain_id": u.DomainID,
					"enabled":   true,
					"links": map[string]string{
						"self": "http://127.0.0.1:5000/v3/users/" + u.ID,
					},
					"password_expires_at": "2099-01-01T00:00:00.000000Z",
					"federated":           []interface{}{},
				},
			})
			if err != nil {
				log.Printf("Error encoding JSON response: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			return
		// Ansible sends PUT or PATCH instead of POST.
		case http.MethodPatch, http.MethodPut:
			// Ansible just check success status.
			var req map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid JSON body", http.StatusBadRequest)
				return
			}

			// TODO Update fields as needed.
			if userReq, ok := req["user"].(map[string]interface{}); ok {
				if name, ok := userReq["name"].(string); ok {
					u.Name = name
				}
			}

			err := json.NewEncoder(w).Encode(map[string]interface{}{
				"user": map[string]interface{}{
					"id":        u.ID,
					"name":      u.Name,
					"domain_id": u.DomainID,
					"enabled":   true,
				},
			})
			if err != nil {
				log.Printf("Error encoding JSON response: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			return

		case http.MethodDelete:
			delete(users, id)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})

	/* ---- role assignments ---- */
	mux.HandleFunc("/v3/role_assignments", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.String())
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		q := r.URL.Query()
		userID := q.Get("user.id")
		groupID := q.Get("group.id")
		projectID := q.Get("project.id")
		domainID := q.Get("domain.id")
		roleID := q.Get("role.id")

		log.Printf(
			"role_assignments filters: user=%s group=%s project=%s domain=%s role=%s",
			userID, groupID, projectID, domainID, roleID,
		)

		resp := map[string]interface{}{
			"role_assignments": []interface{}{},
		}

		err := json.NewEncoder(w).Encode(resp)
		if err != nil {
			log.Printf("Error encoding JSON response: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	})

	/* ============================
	   Nova
	============================ */
	mux.HandleFunc("/v2.1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(map[string]interface{}{
			"version": map[string]interface{}{
				"id":          "v2.1",
				"status":      "CURRENT",
				"min_version": "2.1",
				"version":     "2.90", // <-- set >= 2.10
				"updated":     "2023-01-01T00:00:00Z",
				"links": []map[string]string{
					{
						"rel":  "self",
						"href": "http://127.0.0.1:5000/v2.1/",
					},
				},
			},
		})
		if err != nil {
			log.Printf("Error encoding JSON response: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	})
	mux.HandleFunc("/v2.1/flavors/detail", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(map[string]interface{}{
			"flavors": flavors,
		})
		if err != nil {
			log.Printf("Error encoding JSON response: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	})
	mux.HandleFunc("/v2.1/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.String())
		p := parts(r.URL.Path)
		if len(p) < 3 {
			http.NotFound(w, r)
			return
		}
		switch p[2] {

		case "flavors":
			if len(p) == 3 {
				if r.Method == http.MethodGet {
					// Extract query parameters
					nameFilter := r.URL.Query().Get("name")

					mu.Lock()
					list := []interface{}{}
					for _, f := range flavors {
						// Filter by name if provided
						if nameFilter != "" && f["name"] != nameFilter {
							continue
						}
						list = append(list, f)
					}
					mu.Unlock()

					w.Header().Set("Content-Type", "application/json")
					err := json.NewEncoder(w).Encode(map[string]interface{}{
						"flavors": list,
					})
					if err != nil {
						log.Printf("Error encoding JSON response: %v", err)
						http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					}
					return
				}
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if len(p) == 4 && p[3] == "detail" {
				list := []interface{}{}
				for _, f := range flavors {
					list = append(list, f)
				}
				w.Header().Set("Content-Type", "application/json")
				err := json.NewEncoder(w).Encode(map[string]interface{}{
					"flavors": list,
				})
				if err != nil {
					log.Printf("Error encoding JSON response: %v", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
				return
			}
			if len(p) == 4 {
				flavorID := p[3]
				fmt.Println(flavorID)
				for _, f := range flavors {
					if f["id"] == flavorID {
						err := json.NewEncoder(w).Encode(map[string]interface{}{
							"flavor": f,
						})
						if err != nil {
							log.Printf("Error encoding JSON response: %v", err)
							http.Error(w, "Internal Server Error", http.StatusInternalServerError)
						}
						return
					}
				}
				http.NotFound(w, r)
				return
			}
			if len(p) == 5 && p[4] == "os-extra_specs" {
				flavorID := p[3]
				// Init empty extra specs if missing
				if _, ok := flavorExtraSpecs[flavorID]; !ok {
					flavorExtraSpecs[flavorID] = map[string]string{}
				}

				switch r.Method {

				case http.MethodGet:
					err := json.NewEncoder(w).Encode(map[string]interface{}{
						"extra_specs": flavorExtraSpecs[flavorID],
					})
					if err != nil {
						log.Printf("Error encoding JSON response: %v", err)
						http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					}
					return

				case http.MethodPost:
					var body struct {
						ExtraSpecs map[string]string `json:"extra_specs"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						http.Error(w, "invalid JSON body", http.StatusBadRequest)
						return
					}

					for k, v := range body.ExtraSpecs {
						flavorExtraSpecs[flavorID][k] = v
					}

					err := json.NewEncoder(w).Encode(map[string]interface{}{
						"extra_specs": flavorExtraSpecs[flavorID],
					})
					if err != nil {
						log.Printf("Error encoding JSON response: %v", err)
						http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					}
					return
				}
			}

		case "servers":
			// Match: /v2.1/demo/servers/{id}/os-volume_attachments
			log.Printf("%s %s", r.Method, r.URL.String())
			// Match: /v2.1/{tenant}/servers/{id}/os-interface
			if len(p) >= 5 && p[4] == "os-interface" {
				serverID := p[3]
				if _, ok := servers[serverID]; !ok {
					http.NotFound(w, r)
					return
				}

				switch r.Method {
				case http.MethodPost:
					var req struct {
						InterfaceAttachment struct {
							PortID string `json:"port_id"`
						} `json:"interfaceAttachment"`
					}
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						http.Error(w, "invalid JSON body", http.StatusBadRequest)
						return
					}
					if req.InterfaceAttachment.PortID == "" {
						http.Error(w, "missing port_id", http.StatusBadRequest)
						return
					}

					for _, p := range ports {
						if p["id"] == req.InterfaceAttachment.PortID {
							p["device_id"] = serverID
							p["device_owner"] = "compute:nova"
							p["status"] = "ACTIVE"
							if _, ok := p["admin_state_up"]; !ok {
								p["admin_state_up"] = true
							}
							resp := map[string]interface{}{
								"interfaceAttachment": map[string]interface{}{
									"port_id":   req.InterfaceAttachment.PortID,
									"server_id": serverID,
								},
							}
							w.WriteHeader(http.StatusOK)
							err := json.NewEncoder(w).Encode(resp)
							if err != nil {
								log.Printf("Error encoding JSON response: %v", err)
								http.Error(w, "Internal Server Error", http.StatusInternalServerError)
							}
							return
						}
					}
					http.NotFound(w, r)
					return

				case http.MethodDelete:
					if len(p) < 6 {
						http.Error(w, "missing port id", http.StatusBadRequest)
						return
					}
					portID := p[5]
					for _, p := range ports {
						if p["id"] == portID {
							p["device_id"] = nil
							p["device_owner"] = nil
							p["status"] = "DOWN"
							w.WriteHeader(http.StatusNoContent)
							return
						}
					}
					http.NotFound(w, r)
					return
				}
			}
			// DELETE /v2.1/{project}/servers/{id}/os-volume_attachments/{attachment-id}
			if r.Method == http.MethodDelete && len(p) >= 6 && p[4] == "os-volume_attachments" {
				// URL pattern: /v2.1/demo/servers/{server-id}/os-volume_attachments/{attachment-id}
				// p = ["", "v2.1", "demo", "servers", "{server-id}", "os-volume_attachments", "{attachment-id}"]
				if len(p) < 6 {
					http.Error(w, "missing attachment id", http.StatusBadRequest)
					return
				}

				attachmentID := p[5] // Extract attachment ID from URL
				serverID := p[3]     // Extract server ID from URL

				// Attachment ID format is "attach-{volume-id}"
				// Extract volume ID from attachment ID
				volumeID := strings.TrimPrefix(attachmentID, "attach-")
				if volumeID == attachmentID {
					// No "attach-" prefix found, invalid attachment ID
					http.NotFound(w, r)
					return
				}

				mu.Lock()
				defer mu.Unlock()

				// Find the volume
				vol, ok := volumes[volumeID]
				if !ok {
					http.NotFound(w, r)
					return
				}

				// Remove attachment for this server
				newAttachments := []map[string]interface{}{}
				found := false
				for _, attachment := range vol.Attachments {
					if attachment["server_id"] == serverID {
						found = true
						// Skip this attachment (don't add to newAttachments)
						log.Printf("Detaching volume %s from server %s", volumeID, serverID)
						continue
					}
					newAttachments = append(newAttachments, attachment)
				}

				if !found {
					// Attachment doesn't exist
					http.NotFound(w, r)
					return
				}

				vol.Attachments = newAttachments

				// If no more attachments, set volume back to available
				if len(vol.Attachments) == 0 {
					vol.Status = "available"
					log.Printf("Volume %s set to available (no attachments)", volumeID)
				}

				w.WriteHeader(http.StatusNoContent)
				return
			}
			if strings.HasSuffix(r.URL.Path, "/os-volume_attachments") {
				serverID := p[len(p)-2]
				// POST attach volume
				if r.Method == http.MethodPost {
					var req struct {
						VolumeAttachment struct {
							VolumeID string `json:"volumeId"`
						} `json:"volumeAttachment"`
					}
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						http.Error(w, "invalid JSON body", http.StatusBadRequest)
						return
					}

					// VALIDATION: Check if server exists
					mu.Lock()
					_, serverExists := servers[serverID]
					mu.Unlock()
					if !serverExists {
						http.Error(w, "Server not found", http.StatusNotFound)
						return
					}

					// VALIDATION: Check if volume exists
					mu.Lock()
					v, volumeExists := volumes[req.VolumeAttachment.VolumeID]
					mu.Unlock()
					if !volumeExists {
						http.Error(w, "Volume not found", http.StatusNotFound)
						return
					}

					// VALIDATION: Check if volume is available
					if v.Status != "available" {
						http.Error(w, fmt.Sprintf("Volume must be available (current: %s)", v.Status), http.StatusBadRequest)
						return
					}

					// VALIDATION: Check if volume is already attached to this server
					for _, attachment := range v.Attachments {
						if attachment["server_id"] == serverID {
							http.Error(w, "Volume already attached to this server", http.StatusConflict)
							return
						}
					}

					// Update volume state (existing code)
					mu.Lock()
					v.Status = "in-use"
					v.Attachments = append(v.Attachments, map[string]interface{}{
						"server_id": serverID,
						"device":    "/dev/vdb",
					})
					mu.Unlock()
					// Build and send response
					resp := map[string]interface{}{
						"volumeAttachment": map[string]interface{}{
							"id":       "attach-" + req.VolumeAttachment.VolumeID,
							"volumeId": req.VolumeAttachment.VolumeID,
							"serverId": serverID,
							"device":   "/dev/vdb",
						},
					}

					w.WriteHeader(http.StatusOK)
					err := json.NewEncoder(w).Encode(resp)
					if err != nil {
						log.Printf("Error encoding JSON response: %v", err)
						http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					}
					return
				}
				// GET attachments
				if r.Method == http.MethodGet {
					list := []interface{}{}
					for _, v := range volumes {
						for _, a := range v.Attachments {
							if a["server_id"] == serverID {
								list = append(list, map[string]interface{}{
									"id":       "attach-" + v.ID,
									"volumeId": v.ID,
									"serverId": serverID,
									"device":   a["device"],
								})
							}
						}
					}
					err := json.NewEncoder(w).Encode(map[string]interface{}{
						"volumeAttachments": list,
					})
					if err != nil {
						log.Printf("Error encoding JSON response: %v", err)
						http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					}
					return
				}
			}
			if strings.HasSuffix(r.URL.Path, "/detail") {
				list := []interface{}{}
				for _, s := range servers {
					list = append(list, s)
				}
				w.Header().Set("Content-Type", "application/json")
				err := json.NewEncoder(w).Encode(map[string]interface{}{
					"servers": list,
				})
				if err != nil {
					log.Printf("Error encoding JSON response: %v", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
				return
			}
			if len(p) == 4 && r.Method == http.MethodGet {
				serverID := p[3]

				mu.Lock()
				srv, ok := servers[serverID]
				mu.Unlock()

				if !ok {
					http.NotFound(w, r)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				err := json.NewEncoder(w).Encode(map[string]interface{}{
					"server": map[string]interface{}{
						"id":        srv.ID,
						"name":      srv.Name,
						"status":    srv.Status,
						"flavor":    srv.Flavor,
						"image":     srv.Image,
						"addresses": buildAddresses(srv.Networks),
					},
				})
				if err != nil {
					log.Printf("Error encoding JSON response: %v", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
				return
			}
					// DELETE /v2.1/{project}/servers/{id}
		if len(p) == 4 && r.Method == http.MethodDelete {
			serverID := p[3]

			mu.Lock()
			defer mu.Unlock()

			// Check if server exists
			_, ok := servers[serverID]
			if !ok {
				http.NotFound(w, r)
				return
			}

			log.Printf("Deleting server %s", serverID)

			// Auto-detach all volumes attached to this server
			for volID, vol := range volumes {
				newAttachments := []map[string]interface{}{}
				detachedAny := false
				for _, attachment := range vol.Attachments {
					if attachment["server_id"] == serverID {
						detachedAny = true
						log.Printf("Auto-detaching volume %s from server %s", volID, serverID)
						continue
					}
					newAttachments = append(newAttachments, attachment)
				}
				if detachedAny {
					vol.Attachments = newAttachments
					if len(vol.Attachments) == 0 {
						vol.Status = "available"
						log.Printf("Volume %s set to available (server deleted)", volID)
					}
				}
			}

			// Auto-detach all ports attached to this server
			for _, port := range ports {
				if port["device_id"] == serverID {
					log.Printf("Auto-detaching port %s from server %s", port["id"], serverID)
					port["device_id"] = nil
					port["device_owner"] = nil
					port["status"] = "DOWN"
				}
			}

			// Delete the server
			delete(servers, serverID)
			log.Printf("Server %s deleted", serverID)

			w.WriteHeader(http.StatusNoContent)
			return
		}
			// Match /v2.1/{id}/servers
			if len(p) == 3 && r.Method == http.MethodGet {
				// Extract query parameters
				nameFilter := r.URL.Query().Get("name")
				statusFilter := r.URL.Query().Get("status")

				mu.Lock()
				list := []interface{}{}
				for _, s := range servers {
					// Filter by name if provided
					if nameFilter != "" && s.Name != nameFilter {
						continue
					}

					// Filter by status if provided
					if statusFilter != "" && s.Status != statusFilter {
						continue
					}

					list = append(list, map[string]interface{}{
						"id":        s.ID,
						"name":      s.Name,
						"status":    s.Status,
						"flavor":    s.Flavor,
						"image":     s.Image,
						"addresses": buildAddresses(s.Networks),
					})
				}
				mu.Unlock()

				err := json.NewEncoder(w).Encode(map[string]interface{}{"servers": list})
				if err != nil {
					log.Printf("Error encoding JSON response: %v", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
				return
			}

			if r.Method == http.MethodPost {
				var req struct {
					Server struct {
						Name      string              `json:"name"`
						FlavorRef string              `json:"flavorRef"`
						ImageRef  string              `json:"imageRef"`
						Networks  []map[string]string `json:"networks"`
					} `json:"server"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, "invalid JSON body", http.StatusBadRequest)
					return
				}

				id := fmt.Sprintf("%d", serverID)
				serverID++

				s := &Server{
					ID:       id,
					Name:     req.Server.Name,
					Status:   "BUILD",
					Flavor:   map[string]string{"id": req.Server.FlavorRef},
					Image:    map[string]string{"id": req.Server.ImageRef},
					Networks: req.Server.Networks,
				}
				mu.Lock()
				servers[id] = s
				mu.Unlock()

				go func(srvID string) {
					time.Sleep(5 * time.Second)
					mu.Lock()
					defer mu.Unlock()
					if srv, ok := servers[srvID]; ok {
						srv.Status = "ACTIVE"
						log.Printf("Server %s transitioned to ACTIVE", srvID)
					}
				}(id)

				// Attach ports referenced in networks
				mu.Lock()
				for _, net := range req.Server.Networks {
					portID := ""
					if v, ok := net["port"]; ok {
						portID = v
					} else if v, ok := net["port_id"]; ok {
						portID = v
					}
					if portID == "" {
						continue
					}
					for _, p := range ports {
						if p["id"] == portID {
							p["device_id"] = id
							p["device_owner"] = "compute:nova"
							p["status"] = "ACTIVE"
							if _, ok := p["admin_state_up"]; !ok {
								p["admin_state_up"] = true
							}
						}
					}
				}
				mu.Unlock()

				w.WriteHeader(http.StatusAccepted)
				err := json.NewEncoder(w).Encode(map[string]interface{}{"server": s})
				if err != nil {
					log.Printf("Error encoding JSON response: %v", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
				return
			}

			if r.Method == http.MethodDelete && len(p) == 4 {
				serverID := p[3]
				if _, ok := servers[serverID]; !ok {
					http.NotFound(w, r)
					return
				}
				mu.Lock()
				delete(servers, serverID)

				// Detach all ports
				for _, p := range ports {
					if p["device_id"] == serverID {
						p["device_id"] = nil
						p["device_owner"] = nil
						p["status"] = "DOWN"
					}
				}

				// Detach all volumes (set back to available)
				for _, v := range volumes {
					newAttachments := []map[string]interface{}{}
					for _, attachment := range v.Attachments {
						if attachment["server_id"] != serverID {
							newAttachments = append(newAttachments, attachment)
						}
					}
					v.Attachments = newAttachments

					// If no more attachments, volume becomes available
					if len(v.Attachments) == 0 && v.Status == "in-use" {
						v.Status = "available"
						log.Printf("Volume %s detached and set to available", v.ID)
					}
				}
				mu.Unlock()

				w.WriteHeader(http.StatusNoContent)
				return
			}
		case "images":
			if strings.HasSuffix(r.URL.Path, "/detail") {
				list := []interface{}{}

				for _, img := range images {
					list = append(list, img)
				}

				err := json.NewEncoder(w).Encode(map[string]interface{}{
					"images": list,
				})
				if err != nil {
					log.Printf("Error encoding JSON response: %v", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			} else {
				list := []interface{}{}

				for _, img := range images {
					list = append(list, map[string]interface{}{
						"id":   img["id"],
						"name": img["name"],
					})
				}

				err := json.NewEncoder(w).Encode(map[string]interface{}{
					"images": list,
				})
				if err != nil {
					log.Printf("Error encoding JSON response: %v", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}
			return
		case "os-keypairs":
			if r.Method == http.MethodGet {
				err := json.NewEncoder(w).Encode(map[string]interface{}{
					"keypairs": keypairs,
				})
				if err != nil {
					log.Printf("Error encoding JSON response: %v", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
				return
			}

			if r.Method == http.MethodPost {
				var req struct {
					Keypair map[string]interface{} `json:"keypair"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, "invalid JSON body", http.StatusBadRequest)
					return
				}

				kp := req.Keypair
				kp["fingerprint"] = "fake:fingerprint"
				kp["created_at"] = "2026-01-01T00:00:00Z"

				keypairs = append(keypairs, map[string]interface{}{
					"keypair": kp,
				})

				w.WriteHeader(http.StatusCreated)
				err := json.NewEncoder(w).Encode(map[string]interface{}{
					"keypair": kp,
				})
				if err != nil {
					log.Printf("Error encoding JSON response: %v", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
				return
			}
		}
	})

	mux.HandleFunc("/v2.1/flavors/", func(w http.ResponseWriter, r *http.Request) {
		// We only care about: /v2.1/flavors/{id}/os-extra_specs
		if !strings.HasSuffix(r.URL.Path, "/os-extra_specs") {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		err := json.NewEncoder(w).Encode(map[string]interface{}{
			"extra_specs": map[string]string{},
		})
		if err != nil {
			log.Printf("Error encoding JSON response: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})
	/* ============================
	   Neutron
	============================ */

	// GET and POST /v2.0/networks
	mux.HandleFunc("/v2.0/networks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodGet {
			// Extract query parameters
			nameFilter := r.URL.Query().Get("name")
			statusFilter := r.URL.Query().Get("status")

			list := []interface{}{}
			for _, network := range networks {
				// Filter by name if provided
				if nameFilter != "" {
					if netName, ok := network["name"].(string); !ok || netName != nameFilter {
						continue
					}
				}

				// Filter by status if provided
				if statusFilter != "" {
					if netStatus, ok := network["status"].(string); !ok || netStatus != statusFilter {
						continue
					}
				}

				list = append(list, network)
			}

			err := json.NewEncoder(w).Encode(map[string]interface{}{"networks": list})
			if err != nil {
				log.Printf("Error encoding JSON response: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}
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
			err := json.NewEncoder(w).Encode(map[string]interface{}{
				"network": network,
			})
			if err != nil {
				log.Printf("Error encoding JSON response: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
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
				err := json.NewEncoder(w).Encode(map[string]interface{}{
					"network": n,
				})
				if err != nil {
					log.Printf("Error encoding JSON response: %v", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
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
			err := json.NewEncoder(w).Encode(map[string]interface{}{
				"subnets": subnets,
			})
			if err != nil {
				log.Printf("Error encoding JSON response: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
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

			// ID
			if _, ok := subnet["id"]; !ok {
				subnet["id"] = fmt.Sprintf("subnet-%d", len(subnets)+1)
			}

			// Status (Neutron returns ACTIVE on create)
			subnet["status"] = "ACTIVE"

			// Defaults expected by Neutron / CLI
			if _, ok := subnet["ip_version"]; !ok {
				subnet["ip_version"] = 4
			}
			if _, ok := subnet["enable_dhcp"]; !ok {
				subnet["enable_dhcp"] = true
			}
			if _, ok := subnet["dns_publish_fixed_ip"]; !ok {
				subnet["dns_publish_fixed_ip"] = false
			}

			// Arrays MUST exist (never nil)
			if _, ok := subnet["allocation_pools"]; !ok {
				subnet["allocation_pools"] = []map[string]interface{}{}
			}
			if _, ok := subnet["dns_nameservers"]; !ok {
				subnet["dns_nameservers"] = []string{}
			}
			if _, ok := subnet["host_routes"]; !ok {
				subnet["host_routes"] = []map[string]interface{}{}
			}
			if _, ok := subnet["service_types"]; !ok {
				subnet["service_types"] = []string{}
			}
			if _, ok := subnet["tags"]; !ok {
				subnet["tags"] = []string{}
			}

			// Nullable fields Neutron always returns
			if _, ok := subnet["ipv6_address_mode"]; !ok {
				subnet["ipv6_address_mode"] = nil
			}
			if _, ok := subnet["ipv6_ra_mode"]; !ok {
				subnet["ipv6_ra_mode"] = nil
			}
			if _, ok := subnet["segment_id"]; !ok {
				subnet["segment_id"] = nil
			}
			if _, ok := subnet["subnetpool_id"]; !ok {
				subnet["subnetpool_id"] = nil
			}

			// Metadata
			if _, ok := subnet["revision_number"]; !ok {
				subnet["revision_number"] = 1
			}
			if _, ok := subnet["description"]; !ok {
				subnet["description"] = ""
			}

			// Timestamps (CLI expects strings, not time.Time)
			now := time.Now().UTC().Format(time.RFC3339)
			if _, ok := subnet["created_at"]; !ok {
				subnet["created_at"] = now
			}
			if _, ok := subnet["updated_at"]; !ok {
				subnet["updated_at"] = now
			}

			// router:external default (extension)
			if _, ok := subnet["router:external"]; !ok {
				subnet["router:external"] = false
			}

			subnets = append(subnets, subnet)

			w.WriteHeader(http.StatusCreated)
			err := json.NewEncoder(w).Encode(map[string]interface{}{
				"subnet": subnet,
			})
			if err != nil {
				log.Printf("Error encoding JSON response: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})

	// GET and POST /v2.0/ports
	mux.HandleFunc("/v2.0/ports", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodGet {
			// Extract query parameters
			q := r.URL.Query()
			deviceIDFilter := q.Get("device_id")
			networkIDFilter := q.Get("network_id")
			statusFilter := q.Get("status")

			list := []interface{}{}
			for _, port := range ports {
				// Filter by device_id if provided
				if deviceIDFilter != "" {
					if portDeviceID, ok := port["device_id"].(string); ok {
						if portDeviceID != deviceIDFilter {
							continue
						}
					} else if deviceIDFilter != "" {
						// device_id is nil but filter is set
						continue
					}
				}

				// Filter by network_id if provided
				if networkIDFilter != "" {
					if portNetID, ok := port["network_id"].(string); !ok || portNetID != networkIDFilter {
						continue
					}
				}

				// Filter by status if provided
				if statusFilter != "" {
					if portStatus, ok := port["status"].(string); !ok || portStatus != statusFilter {
						continue
					}
				}

				list = append(list, port)
			}

			err := json.NewEncoder(w).Encode(map[string]interface{}{"ports": list})
			if err != nil {
				log.Printf("Error encoding JSON response: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}
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
			err := json.NewEncoder(w).Encode(map[string]interface{}{
				"port": port,
			})
			if err != nil {
				log.Printf("Error encoding JSON response: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/v2.0/security-groups", func(w http.ResponseWriter, r *http.Request) {
		err := json.NewEncoder(w).Encode(map[string]interface{}{"security_groups": neutronSecGroups})
		if err != nil {
			log.Printf("Error encoding JSON response: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	/* ============================
	   Cinder
	============================ */

	mux.HandleFunc("/v3/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.String())
		p := parts(r.URL.Path)
		// Match /v3/demo/volumes/detail first
		if strings.HasSuffix(r.URL.Path, "/detail") {
			list := []interface{}{}
			for _, v := range volumes {
				list = append(list, v)
			}

			w.Header().Set("Content-Type", "application/json")
			err := json.NewEncoder(w).Encode(map[string]interface{}{
				"volumes": list,
			})
			if err != nil {
				log.Printf("Error encoding JSON response: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}
		// GET /v3/{project}/volumes/{id}
		if len(p) == 4 && p[2] == "volumes" && r.Method == http.MethodGet {
			volumeID := p[3]

			mu.Lock()
			vol, ok := volumes[volumeID]
			mu.Unlock()

			if !ok {
				http.NotFound(w, r)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			err := json.NewEncoder(w).Encode(map[string]interface{}{"volume": vol})
			if err != nil {
				log.Printf("Error encoding JSON response: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}
		// POST /v3/{project}/volumes/{id}/action
		if len(p) == 5 && p[2] == "volumes" && p[4] == "action" && r.Method == http.MethodPost {
			volumeID := p[3]

			mu.Lock()
			vol, ok := volumes[volumeID]
			mu.Unlock()

			if !ok {
				http.NotFound(w, r)
				return
			}

			// Parse the action body
			var actionBody map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&actionBody); err != nil {
				http.Error(w, "invalid JSON body", http.StatusBadRequest)
				return
			}

			// Handle os-set_bootable action
			if bootableData, ok := actionBody["os-set_bootable"]; ok {
				if bootableMap, ok := bootableData.(map[string]interface{}); ok {
					if bootable, ok := bootableMap["bootable"].(bool); ok {
						mu.Lock()
						if vol.Metadata == nil {
							vol.Metadata = make(map[string]string)
						}
						vol.Metadata["bootable"] = fmt.Sprintf("%t", bootable)
						mu.Unlock()
						log.Printf("Set volume %s bootable=%t", volumeID, bootable)
					}
				}
				w.WriteHeader(http.StatusOK)
				return
			}

			// Handle os-set_image_metadata action (for UEFI firmware, etc.)
			if imageMetadata, ok := actionBody["os-set_image_metadata"]; ok {
				if metadataMap, ok := imageMetadata.(map[string]interface{}); ok {
					if metadata, ok := metadataMap["metadata"].(map[string]interface{}); ok {
						mu.Lock()
						if vol.Metadata == nil {
							vol.Metadata = make(map[string]string)
						}
						// Store image metadata (hw_firmware_type, hw_machine_type, etc.)
						for k, v := range metadata {
							if strVal, ok := v.(string); ok {
								vol.Metadata[k] = strVal
							}
						}
						mu.Unlock()
						log.Printf("Set volume %s image metadata: %v", volumeID, metadata)
					}
				}
				w.WriteHeader(http.StatusOK)
				return
			}

			// Unknown action
			http.Error(w, "Unsupported action", http.StatusBadRequest)
			return
		}
		// DELETE /v3/{project}/volumes/{id}
		if len(p) == 4 && p[2] == "volumes" && r.Method == http.MethodDelete {
			volumeID := p[3]

			mu.Lock()
			vol, ok := volumes[volumeID]
			mu.Unlock()

			if !ok {
				http.NotFound(w, r)
				return
			}

			// Check if volume is attached (can't delete attached volumes in real OpenStack)
			if len(vol.Attachments) > 0 {
				http.Error(w, "Volume is attached to a server", http.StatusBadRequest)
				return
			}

			// Check if volume is not in "available" or "error" state
			if vol.Status != "available" && vol.Status != "error" {
				http.Error(w, "Volume must be available or error to delete", http.StatusBadRequest)
				return
			}

			mu.Lock()
			delete(volumes, volumeID)
			mu.Unlock()

			log.Printf("Deleted volume %s", volumeID)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if len(p) == 3 && p[2] == "volumes" {
			if r.Method == http.MethodGet {
				// Extract query parameters
				q := r.URL.Query()
				nameFilter := q.Get("name")
				statusFilter := q.Get("status")

				// Parse metadata filters (e.g., ?metadata[osm]=true)
				metadataFilters := make(map[string]string)
				for key, values := range q {
					if strings.HasPrefix(key, "metadata[") && strings.HasSuffix(key, "]") {
						// Extract "osm" from "metadata[osm]"
						metaKey := key[9 : len(key)-1]
						if len(values) > 0 {
							metadataFilters[metaKey] = values[0]
						}
					}
				}

				mu.Lock()
				list := []interface{}{}
				for _, v := range volumes {
					// Filter by name if provided
					if nameFilter != "" && v.Name != nameFilter {
						continue // Skip this volume
					}

					// Filter by status if provided
					if statusFilter != "" && v.Status != statusFilter {
						continue
					}

					// Filter by metadata - ALL requested metadata keys must match
					matchesMeta := true
					for metaKey, metaValue := range metadataFilters {
						if v.Metadata[metaKey] != metaValue {
							matchesMeta = false
							break
						}
					}
					if !matchesMeta {
						continue
					}

					list = append(list, v)
				}
				mu.Unlock()

				err := json.NewEncoder(w).Encode(map[string]interface{}{"volumes": list})
				if err != nil {
					log.Printf("Error encoding JSON response: %v", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
				return
			}
			if r.Method == http.MethodPost {
				var req struct {
					Volume struct {
						Name     string            `json:"name"`
						Size     int               `json:"size"`
						Metadata map[string]string `json:"metadata"` // ← Metadata support
					} `json:"volume"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, "invalid JSON body", http.StatusBadRequest)
					return
				}

				id := fmt.Sprintf("%d", volumeID)
				volumeID++

				v := &Volume{
					ID:          id,
					Name:        req.Volume.Name,
					Size:        req.Volume.Size,
					Status:      "creating", // ← Start in creating status
					Attachments: []map[string]interface{}{},
					Metadata:    req.Volume.Metadata, // ← Store metadata
				}

				// Initialize empty metadata if nil
				if v.Metadata == nil {
					v.Metadata = map[string]string{}
				}

				mu.Lock()
				volumes[id] = v
				mu.Unlock()

				// Simulate async volume creation
				go func(volID string) {
					time.Sleep(2 * time.Second)
					mu.Lock()
					defer mu.Unlock()
					if vol, ok := volumes[volID]; ok {
						vol.Status = "available"
						log.Printf("Volume %s transitioned to available", volID)
					}
				}(id)

				w.WriteHeader(http.StatusAccepted)
				err := json.NewEncoder(w).Encode(map[string]interface{}{"volume": v})
				if err != nil {
					log.Printf("Error encoding JSON response: %v", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
				return
			}
			if r.Method == http.MethodDelete {
				http.Error(w, "DELETE volumes list not supported", http.StatusMethodNotAllowed)
				return
			}
			http.Error(w, "Method not Implemented", http.StatusNotImplemented)
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})

	/* ============================
	   Glance
	============================ */

	mux.HandleFunc("/v2/images", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			err := json.NewEncoder(w).Encode(map[string]interface{}{"images": images})
			if err != nil {
				log.Printf("Error encoding JSON response: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}

		if r.Method == http.MethodPost {
			var img map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&img); err != nil {
				http.Error(w, "invalid JSON body", http.StatusBadRequest)
				return
			}
			img["id"] = fmt.Sprintf("img-%d", len(images)+1)
			img["status"] = "active"
			images = append(images, img)
			w.WriteHeader(http.StatusCreated)
			err := json.NewEncoder(w).Encode(img)
			if err != nil {
				log.Printf("Error encoding JSON response: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}
	})

	log.Println("Fake OpenStack API listening on :5000")
	err := http.ListenAndServe(":5000", normalizeV2Path(mux))
	if err != nil {
		log.Fatalf("Server failed: %v", err)
	}
	return fs
}

func main() {
	fakeServer := NewFakeServer()
	defer fakeServer.Close()

	// Keep the server running
	select {}
}
