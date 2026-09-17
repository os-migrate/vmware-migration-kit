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
package create_heat_stack

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vmware-migration-kit/plugins/module_utils/ansible"
	osm_os "vmware-migration-kit/plugins/module_utils/openstack"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/orchestration/v1/stacks"
	"gopkg.in/yaml.v3"
)

// Ansible module args
type ModuleArgs struct {
	Cloud           osm_os.DstCloud        `json:"cloud"`
	TemplatePath    string                 `json:"template_path"`
	StackName       string                 `json:"stack_name"`
	Parameters      map[string]interface{} `json:"parameters"`
	Wait            bool                   `json:"wait"`
	Timeout         int                    `json:"timeout"`
	OutputDir       string                 `json:"output_dir"`
	DisableRollback *bool                  `json:"disable_rollback,omitempty"`
}

type StackInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type Response struct {
	Msg      string    `json:"msg"`
	Changed  bool      `json:"changed"`
	Failed   bool      `json:"failed"`
	Stack    StackInfo `json:"stack,omitempty"`
	InfoPath string    `json:"info_path,omitempty"`
}

const stackInfoFileName = "heat_stack_info.txt"

func WriteStackInfoFile(outputDir string, stack StackInfo, templatePath string) (string, error) {
	infoPath := filepath.Join(outputDir, stackInfoFileName)
	content := fmt.Sprintf("Stack Name: %s\nStack ID: %s\nStatus: %s\nTemplate: %s\n",
		stack.Name, stack.ID, stack.Status, templatePath)
	if err := os.WriteFile(infoPath, []byte(content), 0644); err != nil {
		return "", err
	}
	return infoPath, nil
}

func exitJson(responseBody Response) {
	ansible.ReturnResponseWithDeps(ansible.Response{
		Msg:     responseBody.Msg,
		Changed: responseBody.Changed,
		Failed:  responseBody.Failed,
	}, os.Exit, func(s string) {
		// Marshall and print custom response instead
		response, err := json.Marshal(responseBody)
		if err != nil {
			fmt.Println(`{"msg": "Invalid response object", "failed": true}`)
			return
		}
		fmt.Println(string(response))
	})
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

const (
	stackActionCreate = "create"
	stackActionSkip   = "skip"
)

// StackCreateDecision returns whether to create a new stack or reuse an existing
// complete one. Failed or in-progress stacks are errors so wrap is not duplicated.
func StackCreateDecision(status string) (string, error) {
	if status == "" {
		return stackActionCreate, nil
	}
	if strings.HasSuffix(status, "_FAILED") {
		return "", fmt.Errorf("heat stack exists in failed status: %s", status)
	}
	if strings.HasSuffix(status, "_IN_PROGRESS") {
		return "", fmt.Errorf("heat stack is already in progress: %s", status)
	}
	if status == "DELETE_COMPLETE" {
		return stackActionCreate, nil
	}
	if strings.HasSuffix(status, "_COMPLETE") {
		return stackActionSkip, nil
	}
	return "", fmt.Errorf("heat stack exists with unsupported status: %s", status)
}

func FindExistingStack(ctx context.Context, client *gophercloud.ServiceClient, stackName string) (*stacks.RetrievedStack, error) {
	stack, err := stacks.Find(ctx, client, stackName).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return stack, nil
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

	// Validate inputs
	if moduleArgs.StackName == "" {
		ansible.FailJson(ansible.Response{Msg: "Stack name is required"})
	}

	if moduleArgs.TemplatePath == "" {
		ansible.FailJson(ansible.Response{Msg: "Template path is required"})
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
		ansible.FailJson(ansible.Response{Msg: "Authentication failed: " + err.Error()})
	}

	// Create Heat client
	heatClient, err := openstack.NewOrchestrationV1(provider, gophercloud.EndpointOpts{})
	if err != nil {
		ansible.FailJson(ansible.Response{Msg: "Failed to create Heat client: " + err.Error()})
	}

	existing, err := FindExistingStack(ctx, heatClient, moduleArgs.StackName)
	if err != nil {
		ansible.FailJson(ansible.Response{Msg: "Failed to look up existing Heat stack: " + err.Error()})
	}
	if existing != nil {
		action, err := StackCreateDecision(existing.Status)
		if err != nil {
			ansible.FailJson(ansible.Response{Msg: err.Error()})
		}
		if action == stackActionSkip {
			response := Response{
				Changed: false,
				Msg:     "Heat stack already exists",
				Stack: StackInfo{
					ID:     existing.ID,
					Name:   existing.Name,
					Status: existing.Status,
				},
			}
			if moduleArgs.OutputDir != "" {
				infoPath, err := WriteStackInfoFile(moduleArgs.OutputDir, response.Stack, moduleArgs.TemplatePath)
				if err != nil {
					ansible.FailJson(ansible.Response{Msg: "Failed to write stack info file: " + err.Error()})
				}
				response.InfoPath = infoPath
			}
			exitJson(response)
		}
	}

	// Read template file
	templateContent, err := os.ReadFile(moduleArgs.TemplatePath)
	if err != nil {
		ansible.FailJson(ansible.Response{Msg: "Failed to read template file: " + err.Error()})
	}

	// Parse template as map[string]interface{} for Gophercloud
	var templateMap map[string]interface{}
	err = yaml.Unmarshal(templateContent, &templateMap)
	if err != nil {
		ansible.FailJson(ansible.Response{Msg: "Template YAML parsing failed: " + err.Error()})
	}

	// Create template using both Bin and Parsed
	template := &stacks.Template{
		TE: stacks.TE{
			Bin:    templateContent,
			Parsed: templateMap,
		},
	}

	// Create stack
	createOpts := stacks.CreateOpts{
		Name:         moduleArgs.StackName,
		TemplateOpts: template,
		Parameters:   moduleArgs.Parameters,
		Timeout:      moduleArgs.Timeout / 60, // Convert seconds to minutes
	}
	if moduleArgs.DisableRollback != nil {
		createOpts.DisableRollback = moduleArgs.DisableRollback
	}

	createResult := stacks.Create(ctx, heatClient, createOpts)
	if createResult.Err != nil {
		ansible.FailJson(ansible.Response{Msg: "Failed to create Heat stack: " + createResult.Err.Error()})
	}

	createdStack, err := createResult.Extract()
	if err != nil {
		ansible.FailJson(ansible.Response{Msg: "Failed to extract created stack: " + err.Error()})
	}

	// If wait is true, wait for stack to reach CREATE_COMPLETE
	if moduleArgs.Wait {
		err = waitForStackStatus(ctx, heatClient, moduleArgs.StackName, createdStack.ID, "CREATE_COMPLETE", moduleArgs.Timeout)
		if err != nil {
			// For wait failures, we need to include stack info, so use custom response
			response := Response{
				Msg:     "Stack creation failed: " + err.Error(),
				Failed:  true,
				Changed: true,
				Stack: StackInfo{
					ID:     createdStack.ID,
					Name:   moduleArgs.StackName,
					Status: "CREATE_FAILED",
				},
			}
			exitJson(response)
		}
	}

	// Retrieve final stack details
	finalStack, err := stacks.Get(ctx, heatClient, moduleArgs.StackName, createdStack.ID).Extract()
	if err != nil {
		ansible.FailJson(ansible.Response{Msg: "Stack created but failed to retrieve details: " + err.Error()})
	}

	response := Response{
		Changed: true,
		Msg:     "Heat stack created successfully",
		Stack: StackInfo{
			ID:     finalStack.ID,
			Name:   finalStack.Name,
			Status: finalStack.Status,
		},
	}

	if moduleArgs.OutputDir != "" {
		infoPath, err := WriteStackInfoFile(moduleArgs.OutputDir, StackInfo{
			Name:   finalStack.Name,
			ID:     finalStack.ID,
			Status: finalStack.Status,
		}, moduleArgs.TemplatePath)
		if err != nil {
			ansible.FailJson(ansible.Response{Msg: "Failed to write stack info file: " + err.Error()})
		}
		response.InfoPath = infoPath
	}

	exitJson(response)
}
