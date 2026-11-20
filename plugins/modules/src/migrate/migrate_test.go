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
	"encoding/json"
	"testing"
)

func TestDatacenterPath(t *testing.T) {
	cases := map[string]string{
		"/Eco-Datacenter/vm/rhos-migrations/rhel-9.4-1": "/Eco-Datacenter",
		"/Folder/DC/vm/name":                            "/Folder/DC",
		"/ha-datacenter/vm//my-vm":                      "/ha-datacenter", // default vddkpath + "/" + name
		"Eco-Datacenter/vm/x":                           "/Eco-Datacenter",
		"my-vm":                                         "",
		"":                                              "",
	}
	for in, want := range cases {
		if got := datacenterPath(in); got != want {
			t.Errorf("datacenterPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestModuleArgsProxyFields(t *testing.T) {
	raw := `{"transport": "proxy", "proxy_vm_name": "gitlab-runner", "proxy_vm_ip": "10.46.83.55",
		"proxy_vm_guest_username": "cloud-user", "proxy_vm_guest_password": "x",
		"nbd_port_min": 10809, "nbd_port_max": 10820}`
	var a ModuleArgs
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatal(err)
	}
	if a.Transport != "proxy" || a.ProxyVmName != "gitlab-runner" || a.ProxyVmIp != "10.46.83.55" ||
		a.ProxyVmGuestUsername != "cloud-user" || a.ProxyVmGuestPassword != "x" ||
		a.NbdPortMin != 10809 || a.NbdPortMax != 10820 {
		t.Fatalf("proxy args not parsed: %+v", a)
	}
}
