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
package volume_metadata_info

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	osm_os "vmware-migration-kit/vmware_migration_kit/plugins/module_utils/openstack"
)

// Ansible
type ModuleArgs struct {
	DstCloud osm_os.DstCloud `json:"dst_cloud"`
	VolumeID string
}
type Response struct {
	Msg       string `json:"msg"`
	Changed   bool   `json:"changed"`
	Failed    bool   `json:"failed"`
	Converted bool   `json:"converted"`
}

func ExitJson(responseBody Response) {
	returnResponse(responseBody)
}

func FailJson(responseBody Response) {
	responseBody.Failed = true
	returnResponse(responseBody)
}

func FailWithMessage(msg string) {
	response := Response{Msg: msg}
	FailJson(response)
}

func returnResponse(responseBody Response) {
	var response []byte
	var err error
	response, err = json.Marshal(responseBody)
	if err != nil {
		response, _ = json.Marshal(Response{Msg: "Invalid response object"})
	}
	fmt.Println(string(response))
	if responseBody.Failed {
		os.Exit(1)
	} else {
		os.Exit(0)
	}
}

func main() {
	var response Response
	if len(os.Args) != 2 {
		response.Msg = "No argument file provided"
		FailJson(response)
	}

	argsFile := os.Args[1]
	text, err := ioutil.ReadFile(argsFile)
	if err != nil {
		response.Msg = "Could not read configuration file: " + argsFile
		FailJson(response)
	}

	var moduleArgs ModuleArgs
	err = json.Unmarshal(text, &moduleArgs)
	if err != nil {
		response.Msg = "Configuration file not valid JSON: " + argsFile
		FailJson(response)
	}

	// Get the volume metadata
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	provider, err := osm_os.OpenstackAuth(ctx, moduleArgs.DstCloud)
	if err != nil {
		response.Msg = "Failed to authenticate Openstack client: " + err.Error()
		FailJson(response)
	}
	converted, err := osm_os.IsVolumeConverted(provider, moduleArgs.VolumeID)
	if err != nil {
		response.Msg = "Failed to get volume metadata: " + err.Error()
		FailJson(response)
	}
	response.Changed = true
	response.Msg = "Volume metadata info"
	response.Converted = converted
	ExitJson(response)
}
