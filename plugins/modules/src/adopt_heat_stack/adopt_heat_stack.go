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
	"fmt"
	"os"
	"vmware-migration-kit/plugins/module_utils/logger"
	"vmware-migration-kit/plugins/module_utils/openstack"
)

type ModuleArgs struct {
	DstCloud   openstack.DstCloud `json:"cloud"`
	StackName  string             `json:"stack_name"`
	VolumeIDs  []string           `json:"volume_ids"`
	InstanceID string             `json:"instance_id"`
}

type ModuleResponse struct {
	Changed bool                     `json:"changed"`
	Failed  bool                     `json:"failed"`
	Msg     string                   `json:"msg,omitempty"`
	Stack   *openstack.HeatStackInfo `json:"stack,omitempty"`
}

func main() {
	ctx := context.TODO()

	if len(os.Args) != 2 {
		response := ModuleResponse{
			Changed: false,
			Failed:  true,
			Msg:     "No argument file provided",
		}
		outputJSON, _ := json.Marshal(response)
		fmt.Println(string(outputJSON))
		os.Exit(1)
	}

	argsFile := os.Args[1]
	text, err := os.ReadFile(argsFile)
	if err != nil {
		response := ModuleResponse{
			Changed: false,
			Failed:  true,
			Msg:     "Could not read configuration file: " + argsFile + ", error: " + err.Error(),
		}
		outputJSON, _ := json.Marshal(response)
		fmt.Println(string(outputJSON))
		os.Exit(1)
	}

	var moduleArgs ModuleArgs
	err = json.Unmarshal(text, &moduleArgs)
	if err != nil {
		response := ModuleResponse{
			Changed: false,
			Failed:  true,
			Msg:     "Configuration file not valid JSON: " + argsFile + ", error: " + err.Error(),
		}
		outputJSON, _ := json.Marshal(response)
		fmt.Println(string(outputJSON))
		os.Exit(1)
	}

	// Validate inputs
	if moduleArgs.StackName == "" {
		response := ModuleResponse{
			Changed: false,
			Failed:  true,
			Msg:     "stack_name is required",
		}
		outputJSON, _ := json.Marshal(response)
		fmt.Println(string(outputJSON))
		os.Exit(1)
	}

	if len(moduleArgs.VolumeIDs) == 0 && moduleArgs.InstanceID == "" {
		response := ModuleResponse{
			Changed: false,
			Failed:  true,
			Msg:     "At least one volume_id or instance_id must be provided",
		}
		outputJSON, _ := json.Marshal(response)
		fmt.Println(string(outputJSON))
		os.Exit(1)
	}

	// Authenticate
	provider, err := openstack.OpenstackAuth(ctx, moduleArgs.DstCloud)
	if err != nil {
		logger.Log.Errorf("Failed to authenticate: %v", err)
		response := ModuleResponse{
			Changed: false,
			Failed:  true,
			Msg:     "Authentication failed: " + err.Error(),
		}
		outputJSON, _ := json.Marshal(response)
		fmt.Println(string(outputJSON))
		os.Exit(1)
	}

	// Adopt resources into Heat stack
	stackInfo, err := openstack.AdoptResourcesIntoHeatStack(
		ctx,
		provider,
		moduleArgs.StackName,
		moduleArgs.VolumeIDs,
		moduleArgs.InstanceID,
	)
	if err != nil {
		logger.Log.Errorf("Failed to adopt stack: %v", err)
		response := ModuleResponse{
			Changed: false,
			Failed:  true,
			Msg:     "Stack adoption failed: " + err.Error(),
		}
		outputJSON, _ := json.Marshal(response)
		fmt.Println(string(outputJSON))
		os.Exit(1)
	}

	// Success
	response := ModuleResponse{
		Changed: true,
		Failed:  false,
		Msg:     fmt.Sprintf("Successfully adopted resources into Heat stack '%s'", moduleArgs.StackName),
		Stack:   stackInfo,
	}

	outputJSON, _ := json.Marshal(response)
	fmt.Println(string(outputJSON))
}
