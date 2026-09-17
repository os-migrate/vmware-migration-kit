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
package abandon_heat_stack

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
	"vmware-migration-kit/plugins/modules/src/create_heat_stack"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/orchestration/v1/stackresources"
	"github.com/gophercloud/gophercloud/v2/openstack/orchestration/v1/stacks"
	"github.com/gophercloud/gophercloud/v2/openstack/orchestration/v1/stacktemplates"
	"gopkg.in/yaml.v3"
)

// Ansible module args
type ModuleArgs struct {
	Cloud     osm_os.DstCloud `json:"cloud"`
	StackName string          `json:"stack_name"`
	Wait      *bool           `json:"wait"`
	Timeout   int             `json:"timeout"`
	OutputDir string          `json:"output_dir"`
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
	Method   string    `json:"method,omitempty"`
	Stack    StackInfo `json:"stack,omitempty"`
	InfoPath string    `json:"info_path,omitempty"`
}

// ResourceRef is the subset of a Heat stack resource needed to rewrite
// Heat-managed ports and instances to external_id.
type ResourceRef struct {
	Name       string
	LogicalID  string
	PhysicalID string
	Type       string
}

const (
	abandonInfoFileName = "heat_stack_abandon.txt"
	stackActionAbandon  = "abandon"
	stackActionSkip     = "skip"
	methodAbandon       = "abandon"
	methodFallback      = "fallback"
	methodSkip          = "skip"
)

func WriteAbandonInfoFile(outputDir string, stack StackInfo, method string) (string, error) {
	infoPath := filepath.Join(outputDir, abandonInfoFileName)
	content := fmt.Sprintf("Stack Name: %s\nStack ID: %s\nStatus: %s\nMethod: %s\n",
		stack.Name, stack.ID, stack.Status, method)
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
		response, err := json.Marshal(responseBody)
		if err != nil {
			fmt.Println(`{"msg": "Invalid response object", "failed": true}`)
			return
		}
		fmt.Println(string(response))
	})
}

// StackAbandonDecision returns whether to abandon a stack or skip. Missing and
// DELETE_COMPLETE stacks are skip. In-progress stacks are errors. Unlike create,
// already-deleted must not recreate anything.
func StackAbandonDecision(status string) (string, error) {
	if status == "" || status == "DELETE_COMPLETE" {
		return stackActionSkip, nil
	}
	if strings.HasSuffix(status, "_IN_PROGRESS") {
		return "", fmt.Errorf("heat stack is already in progress: %s", status)
	}
	if strings.HasSuffix(status, "_FAILED") {
		return "", fmt.Errorf("heat stack exists in failed status: %s", status)
	}
	if strings.HasSuffix(status, "_COMPLETE") {
		return stackActionAbandon, nil
	}
	return "", fmt.Errorf("heat stack exists with unsupported status: %s", status)
}

func IsStackNotFound(err error) bool {
	return err != nil && gophercloud.ResponseCodeIs(err, http.StatusNotFound)
}

func IsAbandonUnsupported(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "abandon is not supported") ||
		strings.Contains(msg, "stack abandon is not supported") ||
		strings.Contains(msg, "enable_stack_abandon") {
		return true
	}
	return gophercloud.ResponseCodeIs(err, http.StatusBadRequest) ||
		gophercloud.ResponseCodeIs(err, http.StatusForbidden) ||
		gophercloud.ResponseCodeIs(err, http.StatusNotImplemented) ||
		gophercloud.ResponseCodeIs(err, http.StatusMethodNotAllowed)
}

func IsNoChangeUpdate(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no changes") ||
		strings.Contains(msg, "did not change") ||
		strings.Contains(msg, "didn't change")
}

func StackParametersForUpdate(params map[string]string) map[string]interface{} {
	if len(params) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(params))
	for k, v := range params {
		if strings.HasPrefix(k, "OS::") {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func ParseTemplate(data []byte) (map[string]interface{}, error) {
	var template map[string]interface{}
	if err := json.Unmarshal(data, &template); err == nil && template != nil {
		return template, nil
	}
	if err := yaml.Unmarshal(data, &template); err != nil {
		return nil, fmt.Errorf("template JSON/YAML parsing failed: %w", err)
	}
	if template == nil {
		return nil, fmt.Errorf("template JSON/YAML parsing failed: empty template")
	}
	return template, nil
}

func resourceType(res map[string]interface{}) string {
	if t, ok := res["type"].(string); ok {
		return t
	}
	if t, ok := res["Type"].(string); ok {
		return t
	}
	return ""
}

func hasExternalID(res map[string]interface{}) bool {
	if _, ok := res["external_id"]; ok {
		return true
	}
	if _, ok := res["externalId"]; ok {
		return true
	}
	return false
}

func isHeatManagedServerOrPort(resType string) bool {
	return resType == "OS::Nova::Server" || resType == "OS::Neutron::Port"
}

func cloneTemplate(template map[string]interface{}) (map[string]interface{}, error) {
	raw, err := json.Marshal(template)
	if err != nil {
		return nil, err
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func indexResources(resources []ResourceRef) map[string]ResourceRef {
	byID := make(map[string]ResourceRef, len(resources)*2)
	for _, res := range resources {
		if res.LogicalID != "" {
			byID[res.LogicalID] = res
		}
		if res.Name != "" {
			if _, exists := byID[res.Name]; !exists {
				byID[res.Name] = res
			}
		}
	}
	return byID
}

// RewriteHeatManagedResources converts Heat-managed OS::Nova::Server and
// OS::Neutron::Port resources to external_id, keeping the stack's live logical
// names. Volumes and resources that already have external_id are left as-is.
func RewriteHeatManagedResources(template map[string]interface{}, resources []ResourceRef) (map[string]interface{}, bool, error) {
	if template == nil {
		return nil, false, fmt.Errorf("heat template is empty")
	}
	cloned, err := cloneTemplate(template)
	if err != nil {
		return nil, false, fmt.Errorf("failed to copy heat template: %w", err)
	}
	rawResources, ok := cloned["resources"]
	if !ok {
		return nil, false, fmt.Errorf("heat template has no resources")
	}
	resMap, ok := rawResources.(map[string]interface{})
	if !ok {
		return nil, false, fmt.Errorf("heat template resources are not a mapping")
	}

	byID := indexResources(resources)
	changed := false
	for name, raw := range resMap {
		res, ok := raw.(map[string]interface{})
		if !ok {
			return nil, false, fmt.Errorf("heat resource %s is not a mapping", name)
		}
		resType := resourceType(res)
		if !isHeatManagedServerOrPort(resType) {
			continue
		}
		if hasExternalID(res) {
			continue
		}
		live, found := byID[name]
		if !found {
			return nil, false, fmt.Errorf("heat resource %s (%s) not found in stack resources; cannot rewrite to external_id", name, resType)
		}
		if live.PhysicalID == "" {
			return nil, false, fmt.Errorf("heat resource %s (%s) has no physical ID; cannot rewrite to external_id", name, resType)
		}
		resMap[name] = map[string]interface{}{
			"type":        resType,
			"external_id": live.PhysicalID,
		}
		changed = true
	}
	cloned["resources"] = resMap
	return cloned, changed, nil
}

func resourceRefsFromHeat(resources []stackresources.Resource) []ResourceRef {
	out := make([]ResourceRef, 0, len(resources))
	for _, res := range resources {
		out = append(out, ResourceRef{
			Name:       res.Name,
			LogicalID:  res.LogicalID,
			PhysicalID: res.PhysicalID,
			Type:       res.Type,
		})
	}
	return out
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

		if stack.Status == "CREATE_FAILED" || stack.Status == "UPDATE_FAILED" || stack.Status == "DELETE_FAILED" {
			return fmt.Errorf("stack reached failed status: %s - %s", stack.Status, stack.StatusReason)
		}

		time.Sleep(5 * time.Second)
	}
}

func waitForStackDeleted(ctx context.Context, client *gophercloud.ServiceClient, stackName, stackID string, timeout int) error {
	timeoutDuration := time.Duration(timeout) * time.Second
	startTime := time.Now()

	for {
		if time.Since(startTime) > timeoutDuration {
			return fmt.Errorf("timeout waiting for stack to be deleted")
		}

		stack, err := stacks.Get(ctx, client, stackName, stackID).Extract()
		if err != nil {
			if IsStackNotFound(err) {
				return nil
			}
			return fmt.Errorf("failed to get stack status: %w", err)
		}

		if stack.Status == "DELETE_COMPLETE" {
			return nil
		}
		if stack.Status == "DELETE_FAILED" {
			return fmt.Errorf("stack reached failed status: %s - %s", stack.Status, stack.StatusReason)
		}

		time.Sleep(5 * time.Second)
	}
}

func fallbackRewriteAndDelete(ctx context.Context, client *gophercloud.ServiceClient, stack *stacks.RetrievedStack, timeout int, wait bool, abandonErr error) error {
	templateBytes, err := stacktemplates.Get(ctx, client, stack.Name, stack.ID).Extract()
	if err != nil {
		return fmt.Errorf("heat abandon failed (%v); failed to get stack template for fallback: %w", abandonErr, err)
	}
	template, err := ParseTemplate(templateBytes)
	if err != nil {
		return fmt.Errorf("heat abandon failed (%v); %w", abandonErr, err)
	}

	pages, err := stackresources.List(client, stack.Name, stack.ID, nil).AllPages(ctx)
	if err != nil {
		return fmt.Errorf("heat abandon failed (%v); failed to list stack resources for fallback: %w", abandonErr, err)
	}
	liveResources, err := stackresources.ExtractResources(pages)
	if err != nil {
		return fmt.Errorf("heat abandon failed (%v); failed to extract stack resources for fallback: %w", abandonErr, err)
	}

	rewritten, changed, err := RewriteHeatManagedResources(template, resourceRefsFromHeat(liveResources))
	if err != nil {
		return fmt.Errorf("heat abandon failed (%v); fallback rewrite failed: %w", abandonErr, err)
	}

	if changed {
		raw, err := json.Marshal(rewritten)
		if err != nil {
			return fmt.Errorf("heat abandon failed (%v); failed to marshal rewritten template: %w", abandonErr, err)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return fmt.Errorf("heat abandon failed (%v); failed to parse rewritten template: %w", abandonErr, err)
		}
		updateResult := stacks.Update(ctx, client, stack.Name, stack.ID, stacks.UpdateOpts{
			TemplateOpts: &stacks.Template{
				TE: stacks.TE{
					Bin:    raw,
					Parsed: parsed,
				},
			},
			Parameters: StackParametersForUpdate(stack.Parameters),
			Timeout:    timeout / 60,
		})
		if updateResult.Err != nil && !IsNoChangeUpdate(updateResult.Err) {
			return fmt.Errorf("heat abandon failed (%v); fallback stack update failed: %w", abandonErr, updateResult.Err)
		}
		if updateResult.Err == nil {
			if err := waitForStackStatus(ctx, client, stack.Name, stack.ID, "UPDATE_COMPLETE", timeout); err != nil {
				return fmt.Errorf("heat abandon failed (%v); fallback update wait failed: %w", abandonErr, err)
			}
		}
	}

	deleteResult := stacks.Delete(ctx, client, stack.Name, stack.ID)
	if deleteResult.Err != nil && !IsStackNotFound(deleteResult.Err) {
		return fmt.Errorf("heat abandon failed (%v); fallback stack delete failed: %w", abandonErr, deleteResult.Err)
	}
	if wait && !IsStackNotFound(deleteResult.Err) {
		if err := waitForStackDeleted(ctx, client, stack.Name, stack.ID, timeout); err != nil {
			return fmt.Errorf("heat abandon failed (%v); fallback delete wait failed: %w", abandonErr, err)
		}
	}
	return nil
}

func attachInfoFile(response Response, outputDir string) Response {
	if outputDir == "" {
		return response
	}
	infoPath, err := WriteAbandonInfoFile(outputDir, response.Stack, response.Method)
	if err != nil {
		ansible.FailJson(ansible.Response{Msg: "Failed to write abandon info file: " + err.Error()})
	}
	response.InfoPath = infoPath
	return response
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

	if moduleArgs.StackName == "" {
		ansible.FailJson(ansible.Response{Msg: "Stack name is required"})
	}

	if moduleArgs.Timeout == 0 {
		moduleArgs.Timeout = 600
	}
	wait := true
	if moduleArgs.Wait != nil {
		wait = *moduleArgs.Wait
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	provider, err := osm_os.OpenstackAuth(ctx, moduleArgs.Cloud)
	if err != nil {
		ansible.FailJson(ansible.Response{Msg: "Authentication failed: " + err.Error()})
	}

	heatClient, err := openstack.NewOrchestrationV1(provider, gophercloud.EndpointOpts{})
	if err != nil {
		ansible.FailJson(ansible.Response{Msg: "Failed to create Heat client: " + err.Error()})
	}

	existing, err := create_heat_stack.FindExistingStack(ctx, heatClient, moduleArgs.StackName)
	if err != nil {
		ansible.FailJson(ansible.Response{Msg: "Failed to look up existing Heat stack: " + err.Error()})
	}
	if existing == nil {
		response := Response{
			Changed: false,
			Msg:     "Heat stack not found; nothing to abandon",
			Method:  methodSkip,
			Stack: StackInfo{
				Name:   moduleArgs.StackName,
				Status: "NOT_FOUND",
			},
		}
		exitJson(attachInfoFile(response, moduleArgs.OutputDir))
	}

	action, err := StackAbandonDecision(existing.Status)
	if err != nil {
		ansible.FailJson(ansible.Response{Msg: err.Error()})
	}
	if action == stackActionSkip {
		response := Response{
			Changed: false,
			Msg:     "Heat stack already abandoned or deleted",
			Method:  methodSkip,
			Stack: StackInfo{
				ID:     existing.ID,
				Name:   existing.Name,
				Status: existing.Status,
			},
		}
		exitJson(attachInfoFile(response, moduleArgs.OutputDir))
	}

	abandonErr := stacks.Abandon(ctx, heatClient, existing.Name, existing.ID).Err
	if abandonErr == nil || IsStackNotFound(abandonErr) {
		status := "ABANDON_COMPLETE"
		if IsStackNotFound(abandonErr) {
			status = "NOT_FOUND"
		}
		response := Response{
			Changed: true,
			Msg:     "Heat stack abandoned; instances and ports were left intact",
			Method:  methodAbandon,
			Stack: StackInfo{
				ID:     existing.ID,
				Name:   existing.Name,
				Status: status,
			},
		}
		exitJson(attachInfoFile(response, moduleArgs.OutputDir))
	}

	if err := fallbackRewriteAndDelete(ctx, heatClient, existing, moduleArgs.Timeout, wait, abandonErr); err != nil {
		ansible.FailJson(ansible.Response{Msg: err.Error()})
	}

	msg := "Heat stack removed with external_id fallback; instances and ports were left intact"
	if IsAbandonUnsupported(abandonErr) {
		msg = "Heat Abandon is not supported (enable_stack_abandon); rewrote ports and instances to external_id then deleted the stack"
	}
	response := Response{
		Changed: true,
		Msg:     msg,
		Method:  methodFallback,
		Stack: StackInfo{
			ID:     existing.ID,
			Name:   existing.Name,
			Status: "DELETE_COMPLETE",
		},
	}
	exitJson(attachInfoFile(response, moduleArgs.OutputDir))
}
