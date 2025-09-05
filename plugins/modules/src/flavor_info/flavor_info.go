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
	"fmt"
	"os"
	osm_os "vmware-migration-kit/plugins/module_utils/openstack"
)

// Ansible
type ModuleArgs struct {
	Cloud      osm_os.DstCloud `json:"cloud"`
	FlavorName string          `json:"flavor_name"`
	FlavorID   string          `json:"flavor_id"`
}

type Response struct {
	Msg     string      `json:"msg"`
	Changed bool        `json:"changed"`
	Failed  bool        `json:"failed"`
	Flavor  interface{} `json:"flavor"`
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
	text, err := os.ReadFile(argsFile)
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
	provider, err := osm_os.OpenstackAuth(ctx, moduleArgs.Cloud)
	if err != nil {
		response.Msg = "Failed to authenticate Openstack client: " + err.Error()
		FailJson(response)
	}
	flavor, err := osm_os.GetFlavorInfo(provider, moduleArgs.FlavorName, moduleArgs.FlavorID)
	if err != nil {
		response.Msg = "Failed to get flavor info for: " + moduleArgs.FlavorID + " or " + moduleArgs.FlavorName + " error: " + err.Error()
		FailJson(response)
	}
	response.Changed = true
	response.Msg = "Volume metadata info"
	response.Flavor = flavor
	ExitJson(response)
}
