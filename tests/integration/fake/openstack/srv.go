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
 * Copyright 2026 Red Hat, Inc.
 *
 */
package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path"
	"strings"
	"sync"
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

	idSeq = 1
	seq   = 1
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
	}

	flavorExtraSpecs = map[string]map[string]string{
		"osm_flavor": {},
	}

	// servers  = map[string]interface{}{}
	keypairs = []map[string]interface{}{}
	imagesV2 = []map[string]interface{}{
		{"id": "img-1", "name": "cirros", "status": "active"},
	}
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
}

var volumes = map[string]*Volume{}
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

func main() {
	mux := http.NewServeMux()

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
		json.NewEncoder(w).Encode(map[string]interface{}{
			"version": map[string]interface{}{
				"id":     "v3.14",
				"status": "stable",
				"links": []map[string]string{
					{"rel": "self", "href": "http://127.0.0.1:5000/v3/"},
				},
			},
		})
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
			json.NewEncoder(w).Encode(map[string]interface{}{"domains": list})
			return
		}

		if r.Method == http.MethodPost {
			id := fmt.Sprintf("domain-%d", idSeq)
			idSeq++
			domains[id] = &Domain{ID: id, Name: id}
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{"domain": domains[id]})
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
			json.NewEncoder(w).Encode(map[string]interface{}{"domains": list})
			return
		}

		if r.Method == http.MethodPost {
			id := fmt.Sprintf("domain-%d", idSeq)
			idSeq++
			domains[id] = &Domain{ID: id, Name: id}
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{"domain": domains[id]})
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

			json.NewEncoder(w).Encode(map[string]interface{}{"projects": list})
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
			json.NewEncoder(w).Encode(map[string]interface{}{
				"project": p,
			})
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
		pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

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

			json.NewEncoder(w).Encode(map[string]interface{}{
				"roles": userRoles,
			})
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
				json.NewEncoder(w).Encode(map[string]interface{}{
					"project": p,
				})
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
			json.NewEncoder(w).Encode(map[string]interface{}{"roles": list})

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
			json.NewEncoder(w).Encode(map[string]interface{}{"role": rl})

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
			json.NewEncoder(w).Encode(map[string]interface{}{
				"role": map[string]interface{}{
					"id":   rl.ID,
					"name": rl.Name,
				},
			})
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

			json.NewEncoder(w).Encode(map[string]interface{}{
				"user": map[string]interface{}{
					"id":   rl.ID,
					"name": rl.Name,
				},
			})
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
			parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			userID := parts[len(parts)-1]
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
			json.NewEncoder(w).Encode(map[string]interface{}{
				"users": list,
			})
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
			json.NewEncoder(w).Encode(map[string]interface{}{
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
			json.NewEncoder(w).Encode(map[string]interface{}{
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
			return
		// Ansible sends PUT or PATCH instead of POST.
		case http.MethodPatch, http.MethodPut:
			// Ansible just check success status.
			var req map[string]interface{}
			json.NewDecoder(r.Body).Decode(&req)

			// TODO Update fields as needed.
			if userReq, ok := req["user"].(map[string]interface{}); ok {
				if name, ok := userReq["name"].(string); ok {
					u.Name = name
				}
			}

			json.NewEncoder(w).Encode(map[string]interface{}{
				"user": map[string]interface{}{
					"id":        u.ID,
					"name":      u.Name,
					"domain_id": u.DomainID,
					"enabled":   true,
				},
			})
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

		json.NewEncoder(w).Encode(resp)
	})

	/* ============================
	   Nova
	============================ */
	mux.HandleFunc("/v2.1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
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
	})
	mux.HandleFunc("/v2.1/flavors/detail", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"flavors": flavors,
		})
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
				json.NewEncoder(w).Encode(map[string]interface{}{"flavors": flavors})
				return
			}
			if len(p) == 4 && p[3] == "detail" {
				json.NewEncoder(w).Encode(map[string]interface{}{"flavors": flavors})
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
					json.NewEncoder(w).Encode(map[string]interface{}{
						"extra_specs": flavorExtraSpecs[flavorID],
					})
					return

				case http.MethodPost:
					var body struct {
						ExtraSpecs map[string]string `json:"extra_specs"`
					}
					json.NewDecoder(r.Body).Decode(&body)

					for k, v := range body.ExtraSpecs {
						flavorExtraSpecs[flavorID][k] = v
					}

					json.NewEncoder(w).Encode(map[string]interface{}{
						"extra_specs": flavorExtraSpecs[flavorID],
					})
					return
				}

				// json.NewEncoder(w).Encode(map[string]interface{}{
				// 	"extra_specs": flavorExtraSpecs[p[4]],
				// })
				// return
			}

		case "servers":
			// Match: /v2.1/demo/servers/{id}/os-volume_attachments
			log.Printf("%s %s", r.Method, r.URL.String())
			if strings.HasSuffix(r.URL.Path, "/os-volume_attachments") {
				// parts := strings.Split(p, "/")
				//serverID := parts[len(parts)-2]
				serverID := p[len(p)-2]

				// POST attach volume
				if r.Method == http.MethodPost {
					var req struct {
						VolumeAttachment struct {
							VolumeID string `json:"volumeId"`
						} `json:"volumeAttachment"`
					}
					json.NewDecoder(r.Body).Decode(&req)

					// Update volume state
					if v, ok := volumes[req.VolumeAttachment.VolumeID]; ok {
						v.Status = "in-use"
						v.Attachments = append(v.Attachments, map[string]interface{}{
							"server_id": serverID,
							"device":    "/dev/vdb",
						})
					}

					resp := map[string]interface{}{
						"volumeAttachment": map[string]interface{}{
							"id":       "attach-" + req.VolumeAttachment.VolumeID,
							"volumeId": req.VolumeAttachment.VolumeID,
							"serverId": serverID,
							"device":   "/dev/vdb",
						},
					}

					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(resp)
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
					json.NewEncoder(w).Encode(map[string]interface{}{
						"volumeAttachments": list,
					})
					return
				}
			}
			if strings.HasSuffix(r.URL.Path, "/detail") {
				list := []interface{}{}
				for _, s := range servers {
					list = append(list, s)
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"servers": list,
				})
				return
			}

			// Match /v2.1/{id}/servers
			log.Printf("%s %s", r.Method, r.URL.String())
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
				return
			}
		case "images":
			if strings.HasSuffix(r.URL.Path, "/detail") {
				list := []interface{}{}

				for _, img := range images {
					list = append(list, img)
				}

				json.NewEncoder(w).Encode(map[string]interface{}{
					"images": list,
				})
			} else {
				list := []interface{}{}

				for _, img := range images {
					list = append(list, map[string]interface{}{
						"id":   img["id"],
						"name": img["name"],
					})
				}

				json.NewEncoder(w).Encode(map[string]interface{}{
					"images": list,
				})
			}
			return
		case "os-keypairs":
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
			// json.NewEncoder(w).Encode(map[string]interface{}{"keypairs": keypairs})
			// return
		}
	})

	mux.HandleFunc("/v2.1/flavors/", func(w http.ResponseWriter, r *http.Request) {
		// We only care about: /v2.1/flavors/{id}/os-extra_specs
		if !strings.HasSuffix(r.URL.Path, "/os-extra_specs") {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		json.NewEncoder(w).Encode(map[string]interface{}{
			"extra_specs": map[string]string{},
		})
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
			json.NewEncoder(w).Encode(map[string]interface{}{
				"networks": networks,
			})
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

	// GET and POST /v2.0/ports
	mux.HandleFunc("/v2.0/ports", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ports": ports,
			})
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
			json.NewEncoder(w).Encode(map[string]interface{}{
				"port": port,
			})
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/v2.0/security-groups", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"security_groups": neutronSecGroups})
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
			json.NewEncoder(w).Encode(map[string]interface{}{
				"volumes": list,
			})
			return
		}
		if len(p) == 3 && p[2] == "volumes" {
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

	log.Println("Fake OpenStack API listening on :5000")
	log.Fatal(http.ListenAndServe(":5000", mux))
}
