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
package discover_heat_wrap

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"vmware-migration-kit/plugins/module_utils/ansible"
	osm_os "vmware-migration-kit/plugins/module_utils/openstack"
)

type ModuleArgs struct {
	Cloud   osm_os.DstCloud `json:"cloud"`
	VMNames []string        `json:"vm_names"`
}

type WrapVMData struct {
	Name           string   `json:"name"`
	BootVolumeID   string   `json:"boot_volume_id"`
	Flavor         string   `json:"flavor"`
	Network        string   `json:"network"`
	SecurityGroups []string `json:"security_groups"`
	DataVolumeIDs  []string `json:"data_volume_ids,omitempty"`
	InstanceID     string   `json:"instance_id"`
	PortIDs        []string `json:"port_ids"`
	Status         string   `json:"status"`
}

type Response struct {
	Msg     string       `json:"msg"`
	Changed bool         `json:"changed"`
	Failed  bool         `json:"failed"`
	VMsData []WrapVMData `json:"vms_data,omitempty"`
}

func exitJson(responseBody Response) {
	ansible.ReturnResponseWithDeps(ansible.Response{
		Msg:     responseBody.Msg,
		Changed: responseBody.Changed,
		Failed:  responseBody.Failed,
	}, os.Exit, func(s string) {
		response, err := json.Marshal(responseBody)
		if err != nil {
			fmt.Println(`{"msg": "Invalid response object", "failed": true}`)
			return
		}
		fmt.Println(string(response))
	})
}

func toWrapVMData(vms []osm_os.WrapVM) []WrapVMData {
	out := make([]WrapVMData, 0, len(vms))
	for _, vm := range vms {
		out = append(out, WrapVMData{
			Name:           vm.Name,
			BootVolumeID:   vm.BootVolumeID,
			Flavor:         vm.Flavor,
			Network:        vm.Network,
			SecurityGroups: vm.SecurityGroups,
			DataVolumeIDs:  vm.DataVolumeIDs,
			InstanceID:     vm.InstanceID,
			PortIDs:        vm.PortIDs,
			Status:         vm.Status,
		})
	}
	return out
}

func Run() {
	if len(os.Args) != 2 {
		ansible.FailJson(ansible.Response{Msg: "No argument file provided"})
	}

	argsFile := os.Args[1]
	text, err := os.ReadFile(argsFile)
	if err != nil {
		ansible.FailJson(ansible.Response{Msg: "Could not read configuration file: " + argsFile})
	}

	var moduleArgs ModuleArgs
	err = json.Unmarshal(text, &moduleArgs)
	if err != nil {
		ansible.FailJson(ansible.Response{Msg: "Configuration file not valid JSON: " + argsFile + " - " + err.Error()})
	}

	if len(moduleArgs.VMNames) == 0 {
		ansible.FailJson(ansible.Response{Msg: "vm_names is required"})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	provider, err := osm_os.OpenstackAuth(ctx, moduleArgs.Cloud)
	if err != nil {
		ansible.FailJson(ansible.Response{Msg: "Authentication failed: " + err.Error()})
	}

	vms, err := osm_os.DiscoverWrapVMs(provider, moduleArgs.VMNames)
	if err != nil {
		ansible.FailJson(ansible.Response{Msg: "Failed to discover existing instances: " + err.Error()})
	}

	exitJson(Response{
		Changed: false,
		Msg:     fmt.Sprintf("Discovered %d existing instances for Heat wrap", len(vms)),
		VMsData: toWrapVMData(vms),
	})
}
