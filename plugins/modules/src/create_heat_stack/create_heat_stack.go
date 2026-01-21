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
	"time"
	osm_os "vmware-migration-kit/plugins/module_utils/openstack"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/orchestration/v1/stacks"
	"gopkg.in/yaml.v3"
)

// Ansible module args
type ModuleArgs struct {
	Cloud        osm_os.DstCloud        `json:"cloud"`
	TemplatePath string                 `json:"template_path"`
	StackName    string                 `json:"stack_name"`
	Parameters   map[string]interface{} `json:"parameters"`
	Wait         bool                   `json:"wait"`
	Timeout      int                    `json:"timeout"`
}

type StackInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type Response struct {
	Msg     string    `json:"msg"`
	Changed bool      `json:"changed"`
	Failed  bool      `json:"failed"`
	Stack   StackInfo `json:"stack,omitempty"`
}

func ExitJson(responseBody Response) {
	returnResponse(responseBody)
}

func FailJson(responseBody Response) {
	responseBody.Failed = true
	returnResponse(responseBody)
}

func returnResponse(responseBody Response) {
	response, err := json.Marshal(responseBody)
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

func waitForStackStatus(ctx context.Context, client *gophercloud.ServiceClient, stackName, stackID, targetStatus string, timeout int) error {
	timeoutDuration := time.Duration(timeout) * time.Second
	startTime := time.Now()

	for {
		if time.Since(startTime) > timeoutDuration {
			return fmt.Errorf("timeout waiting for stack to reach status %s", targetStatus)
		}

		stack, err := stacks.Get(ctx, client, stackName, stackID).Extract()
		if err != nil {
			return fmt.Errorf("failed to get stack status: %w", err)
		}

		if stack.Status == targetStatus {
			return nil
		}

		// Check for failure states
		if stack.Status == "CREATE_FAILED" || stack.Status == "UPDATE_FAILED" || stack.Status == "DELETE_FAILED" {
			return fmt.Errorf("stack reached failed status: %s - %s", stack.Status, stack.StatusReason)
		}

		time.Sleep(5 * time.Second)
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
		response.Msg = "Configuration file not valid JSON: " + argsFile + " - " + err.Error()
		FailJson(response)
	}

	// Validate inputs
	if moduleArgs.StackName == "" {
		response.Msg = "Stack name is required"
		FailJson(response)
	}

	if moduleArgs.TemplatePath == "" {
		response.Msg = "Template path is required"
		FailJson(response)
	}

	// Set default timeout if not provided
	if moduleArgs.Timeout == 0 {
		moduleArgs.Timeout = 600 // 10 minutes default
	}

	// Authenticate
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	provider, err := osm_os.OpenstackAuth(ctx, moduleArgs.Cloud)
	if err != nil {
		response.Msg = "Authentication failed: " + err.Error()
		FailJson(response)
	}

	// Create Heat client
	heatClient, err := openstack.NewOrchestrationV1(provider, gophercloud.EndpointOpts{})
	if err != nil {
		response.Msg = "Failed to create Heat client: " + err.Error()
		FailJson(response)
	}

	// Read template file
	templateContent, err := os.ReadFile(moduleArgs.TemplatePath)
	if err != nil {
		response.Msg = "Failed to read template file: " + err.Error()
		FailJson(response)
	}

	// Parse template YAML
	var templateMap map[string]interface{}
	err = yaml.Unmarshal(templateContent, &templateMap)
	if err != nil {
		response.Msg = "Failed to parse template YAML: " + err.Error()
		FailJson(response)
	}

	// Create template options
	templateOpts := &stacks.Template{
		TE: stacks.TE{
			Bin:    templateContent,
			Parsed: templateMap,
		},
	}

	// Create stack
	createOpts := stacks.CreateOpts{
		Name:         moduleArgs.StackName,
		TemplateOpts: templateOpts,
		Parameters:   moduleArgs.Parameters,
		Timeout:      moduleArgs.Timeout / 60, // Convert seconds to minutes
	}

	createResult := stacks.Create(ctx, heatClient, createOpts)
	if createResult.Err != nil {
		response.Msg = "Failed to create Heat stack: " + createResult.Err.Error()
		FailJson(response)
	}

	createdStack, err := createResult.Extract()
	if err != nil {
		response.Msg = "Failed to extract created stack: " + err.Error()
		FailJson(response)
	}

	// If wait is true, wait for stack to reach CREATE_COMPLETE
	if moduleArgs.Wait {
		err = waitForStackStatus(ctx, heatClient, moduleArgs.StackName, createdStack.ID, "CREATE_COMPLETE", moduleArgs.Timeout)
		if err != nil {
			response.Msg = "Stack creation failed: " + err.Error()
			response.Stack = StackInfo{
				ID:     createdStack.ID,
				Name:   moduleArgs.StackName,
				Status: "CREATE_FAILED",
			}
			FailJson(response)
		}
	}

	// Retrieve final stack details
	finalStack, err := stacks.Get(ctx, heatClient, moduleArgs.StackName, createdStack.ID).Extract()
	if err != nil {
		response.Msg = "Stack created but failed to retrieve details: " + err.Error()
		FailJson(response)
	}

	response.Changed = true
	response.Msg = "Heat stack created successfully"
	response.Stack = StackInfo{
		ID:     finalStack.ID,
		Name:   finalStack.Name,
		Status: finalStack.Status,
	}
	ExitJson(response)
}
