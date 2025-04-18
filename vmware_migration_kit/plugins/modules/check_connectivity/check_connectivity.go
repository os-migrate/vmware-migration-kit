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
 * Copyright 2024 Red Hat, Inc.
 *
 */

package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"time"

	"vmware-migration_kit/vmware_migration_kit/plugins/module_utils/ansible"
	"vmware-migration_kit/vmware_migration_kit/plugins/module_utils/connectivity"
)

// ModuleArgs represents the arguments for the check_connectivity module
type ModuleArgs struct {
	Server   string `json:"server"`
	Username string `json:"username"`
	Password string `json:"password"`
	Timeout  int    `json:"timeout"`
	VMName   string `json:"vmname"`
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
		response.Msg = "Configuration file not valid JSON: " + argsFile
		ansible.FailJson(response)
	}

	// Set default timeout if not provided
	timeout := 10 * time.Second
	if moduleArgs.Timeout > 0 {
		timeout = time.Duration(moduleArgs.Timeout) * time.Second
	}

	// After the timeout setup...
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Check vCenter connectivity using VMware client
	isConnected, err := connectivity.CheckVCenterConnectivity(
		ctx,
		moduleArgs.Server,
		moduleArgs.Username,
		moduleArgs.Password,
		moduleArgs.VMName,
	)
	if err != nil {
		response.Msg = "vCenter connectivity check failed: " + err.Error()
		ansible.FailJson(response)
	}

	if !isConnected {
		response.Msg = "VM '" + moduleArgs.VMName + "' is not in a normal state"
		ansible.FailJson(response)
	}

	response.Changed = false
	response.Msg = "Connectivity check for VM '" + moduleArgs.VMName + "' passed successfully"
	ansible.ExitJson(response)
}
