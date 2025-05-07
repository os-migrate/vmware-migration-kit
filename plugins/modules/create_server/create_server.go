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
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"vmware-migration-kit/vmware_migration_kit/plugins/module_utils/ansible"
	"vmware-migration-kit/vmware_migration_kit/plugins/module_utils/logger"
	osm_os "vmware-migration-kit/vmware_migration_kit/plugins/module_utils/openstack"
)

/* Argument file example:
{
		"user": "Administrator@osm",
		"password": "foo",
		"server": "vcenter.osm",
		"name": "rhel-9.4.-1",
		"cbtsync": false,
		"state": "present",
		"volume": [],
		"securitygroups": "xyz",
		"boot_volume": "zyx",
		"nics": [{"port-id":"123-aqse"}],
		"flavor": "xcvb",
		"dst_cloud": {
			"auth": {
				"auth_url": "https://keystone.osm",
				"username": "admin",
				"project_name": "admin",
				"user_domain_name": "Default",
        		"project_domain_name": "Default",
				"password": "foo"
			},
			"region_name": "regionOne",
			"interface": "public",
			"identity_api_version": 3
		}
}
*/

type ModuleArgs struct {
	Cloud          osm_os.DstCloud `json:"cloud"`
	State          string          `json:"state"`
	Name           string          `json:"name"`
	Nics           []interface{}   `json:"nics"`
	BootVolume     string          `json:"boot_volume"`
	Volumes        []string        `json:"volumes"`
	SecurityGroups []string        `json:"security_groups"`
	Flavor         string          `json:"flavor"`
	KeyName        string          `json:"key_name"`
}

type ModuleResponse struct {
	Changed bool   `json:"changed"`
	Failed  bool   `json:"failed"`
	Msg     string `json:"msg,omitempty"`
	ID      string `json:"id"`
}

func success(changed bool, id string) {
	res := ModuleResponse{
		Changed: changed,
		Failed:  false,
		ID:      id,
	}
	json.NewEncoder(os.Stdout).Encode(res)
	os.Exit(0)
}

func main() {
	var response ansible.Response
	if len(os.Args) != 2 {
		response.Msg = "No argument file provided"
		ansible.FailJson(response)
	}

	argsFile := os.Args[1]
	text, err := ioutil.ReadFile(argsFile)
	if err != nil {
		response.Msg = "Could not read configuration file: " + argsFile
		ansible.FailJson(response)
	}

	var moduleArgs ModuleArgs
	err = json.Unmarshal(text, &moduleArgs)
	if err != nil {
		response.Msg = "Configuration file not valid JSON: " + string(text)
		ansible.FailJson(response)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	provider, err := osm_os.OpenstackAuth(ctx, moduleArgs.Cloud)
	if err != nil {
		logger.Log.Infof("Failed to authenticate Openstack client: %v", err)
		response.Msg = "Failed to authenticate Openstack client: " + err.Error()
		ansible.FailJson(response)
	}

	ServerAgrs := osm_os.ServerArgs{
		Name:           moduleArgs.Name,
		Flavor:         moduleArgs.Flavor,
		BootVolume:     moduleArgs.BootVolume,
		SecurityGroups: moduleArgs.SecurityGroups,
		Nics:           moduleArgs.Nics,
		Volumes:        moduleArgs.Volumes,
	}
	server, err := osm_os.CreateServer(provider, ServerAgrs)
	if err != nil {
		response.Msg = "Failed create instance: " + err.Error()
		ansible.FailJson(response)
	}

	success(true, server)
}
