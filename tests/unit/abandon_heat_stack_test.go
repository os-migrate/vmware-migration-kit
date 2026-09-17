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

package moduleutils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud/v2"

	abandonheatstack "vmware-migration-kit/plugins/modules/src/abandon_heat_stack"
	generateheattemplate "vmware-migration-kit/plugins/modules/src/generate_heat_template"
)

func TestStackAbandonDecision(t *testing.T) {
	action, err := abandonheatstack.StackAbandonDecision("")
	if err != nil || action != "skip" {
		t.Fatalf("empty status should skip, got action=%q err=%v", action, err)
	}

	action, err = abandonheatstack.StackAbandonDecision("DELETE_COMPLETE")
	if err != nil || action != "skip" {
		t.Fatalf("DELETE_COMPLETE should skip, got action=%q err=%v", action, err)
	}

	action, err = abandonheatstack.StackAbandonDecision("CREATE_COMPLETE")
	if err != nil || action != "abandon" {
		t.Fatalf("CREATE_COMPLETE should abandon, got action=%q err=%v", action, err)
	}

	action, err = abandonheatstack.StackAbandonDecision("UPDATE_COMPLETE")
	if err != nil || action != "abandon" {
		t.Fatalf("UPDATE_COMPLETE should abandon, got action=%q err=%v", action, err)
	}

	if _, err = abandonheatstack.StackAbandonDecision("CREATE_IN_PROGRESS"); err == nil {
		t.Fatal("CREATE_IN_PROGRESS should error")
	} else if !strings.Contains(err.Error(), "heat stack is already in progress: CREATE_IN_PROGRESS") {
		t.Fatalf("unexpected in-progress error: %v", err)
	}

	if _, err = abandonheatstack.StackAbandonDecision("CREATE_FAILED"); err == nil {
		t.Fatal("CREATE_FAILED should error")
	}

	if _, err = abandonheatstack.StackAbandonDecision("BOGUS"); err == nil {
		t.Fatal("unsupported status should error")
	}
}

func TestIsStackNotFound(t *testing.T) {
	if abandonheatstack.IsStackNotFound(nil) {
		t.Fatal("nil error is not not-found")
	}
	notFound := gophercloud.ErrUnexpectedResponseCode{Actual: http.StatusNotFound}
	if !abandonheatstack.IsStackNotFound(notFound) {
		t.Fatal("404 should be treated as stack gone")
	}
	wrapped := fmt.Errorf("failed to get stack status: %w", notFound)
	if !abandonheatstack.IsStackNotFound(wrapped) {
		t.Fatal("wrapped 404 should be detected by errors.As")
	}
	other := gophercloud.ErrUnexpectedResponseCode{Actual: http.StatusInternalServerError}
	if abandonheatstack.IsStackNotFound(other) {
		t.Fatal("500 should not be treated as stack gone")
	}
}

func TestIsAbandonUnsupported(t *testing.T) {
	if abandonheatstack.IsAbandonUnsupported(nil) {
		t.Fatal("nil error is not unsupported")
	}
	msgErr := fmt.Errorf("Stack Abandon is not supported")
	if !abandonheatstack.IsAbandonUnsupported(msgErr) {
		t.Fatal("expected message match for PSI/RHOS abandon error")
	}
	cfgErr := fmt.Errorf("enable_stack_abandon is false")
	if !abandonheatstack.IsAbandonUnsupported(cfgErr) {
		t.Fatal("expected enable_stack_abandon message match")
	}
	badRequest := gophercloud.ErrUnexpectedResponseCode{Actual: http.StatusBadRequest}
	if !abandonheatstack.IsAbandonUnsupported(badRequest) {
		t.Fatal("400 should be treated as abandon unsupported")
	}
	notFound := gophercloud.ErrUnexpectedResponseCode{Actual: http.StatusNotFound}
	if abandonheatstack.IsAbandonUnsupported(notFound) {
		t.Fatal("404 is stack gone, not unsupported abandon")
	}
}

func TestIsNoChangeUpdate(t *testing.T) {
	if abandonheatstack.IsNoChangeUpdate(nil) {
		t.Fatal("nil is not a no-change update")
	}
	if !abandonheatstack.IsNoChangeUpdate(fmt.Errorf("The template didn't change")) {
		t.Fatal("expected no-change detection")
	}
	if abandonheatstack.IsNoChangeUpdate(fmt.Errorf("stack update failed")) {
		t.Fatal("generic update error is not no-change")
	}
}

func TestStackParametersForUpdate(t *testing.T) {
	params := map[string]string{
		"rhel_1_boot_volume_id": "vol-1",
		"OS::project_id":        "proj",
		"OS::stack_id":          "stack",
		"OS::stack_name":        "os-migrate-test",
	}
	out := abandonheatstack.StackParametersForUpdate(params)
	if out["rhel_1_boot_volume_id"] != "vol-1" {
		t.Fatalf("expected volume parameter, got %#v", out)
	}
	if _, ok := out["OS::stack_id"]; ok {
		t.Fatalf("OS:: parameters must be stripped, got %#v", out)
	}
	if abandonheatstack.StackParametersForUpdate(nil) != nil {
		t.Fatal("empty params should be nil")
	}
}

func TestWriteAbandonInfoFile(t *testing.T) {
	dir := t.TempDir()
	stack := abandonheatstack.StackInfo{
		Name:   "os-migrate-test",
		ID:     "abc-123",
		Status: "DELETE_COMPLETE",
	}
	infoPath, err := abandonheatstack.WriteAbandonInfoFile(dir, stack, "fallback")
	if err != nil {
		t.Fatalf("WriteAbandonInfoFile failed: %v", err)
	}
	expectedPath := filepath.Join(dir, "heat_stack_abandon.txt")
	if infoPath != expectedPath {
		t.Errorf("expected info path %q, got %q", expectedPath, infoPath)
	}
	content, err := os.ReadFile(infoPath)
	if err != nil {
		t.Fatalf("failed to read info file: %v", err)
	}
	want := strings.Join([]string{
		"Stack Name: os-migrate-test",
		"Stack ID: abc-123",
		"Status: DELETE_COMPLETE",
		"Method: fallback",
	}, "\n") + "\n"
	if string(content) != want {
		t.Errorf("unexpected content:\n%s", string(content))
	}
}

func TestParseTemplateJSONAndYAML(t *testing.T) {
	jsonTpl := []byte(`{"heat_template_version":"wallaby","resources":{"rhel_1_port":{"type":"OS::Neutron::Port"}}}`)
	parsed, err := abandonheatstack.ParseTemplate(jsonTpl)
	if err != nil {
		t.Fatalf("json parse failed: %v", err)
	}
	resources := parsed["resources"].(map[string]interface{})
	if _, ok := resources["rhel_1_port"]; !ok {
		t.Fatalf("expected rhel_1_port in json template, got %#v", parsed)
	}

	yamlTpl := []byte("heat_template_version: wallaby\nresources:\n  rhel_1_port:\n    type: OS::Neutron::Port\n")
	parsed, err = abandonheatstack.ParseTemplate(yamlTpl)
	if err != nil {
		t.Fatalf("yaml parse failed: %v", err)
	}
	resources = parsed["resources"].(map[string]interface{})
	if _, ok := resources["rhel_1_port"]; !ok {
		t.Fatalf("expected rhel_1_port in yaml template, got %#v", parsed)
	}
}

func parseGeneratedTemplate(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	parsed, err := abandonheatstack.ParseTemplate([]byte(raw))
	if err != nil {
		t.Fatalf("failed to parse generated template: %v", err)
	}
	return parsed
}

func TestRewriteKeepsCreateModeLogicalNames(t *testing.T) {
	vmsData := []generateheattemplate.VMData{
		{
			Name:           "rhel-1",
			BootVolumeID:   "boot-volume-uuid",
			Flavor:         "flavor-uuid",
			Network:        "network-uuid",
			SecurityGroups: []string{"default"},
		},
	}
	raw, _ := generateheattemplate.GenerateHeatTemplate(vmsData, "os-migrate-test")
	template := parseGeneratedTemplate(t, raw)

	rewritten, changed, err := abandonheatstack.RewriteHeatManagedResources(template, []abandonheatstack.ResourceRef{
		{Name: "rhel_1_port", LogicalID: "rhel_1_port", PhysicalID: "port-uuid", Type: "OS::Neutron::Port"},
		{Name: "rhel_1_instance", LogicalID: "rhel_1_instance", PhysicalID: "server-uuid", Type: "OS::Nova::Server"},
		{Name: "rhel_1_boot_volume", LogicalID: "rhel_1_boot_volume", PhysicalID: "boot-volume-uuid", Type: "OS::Cinder::Volume"},
	})
	if err != nil {
		t.Fatalf("rewrite failed: %v", err)
	}
	if !changed {
		t.Fatal("expected rewrite to change Heat-managed ports and instances")
	}

	resources := rewritten["resources"].(map[string]interface{})
	if _, ok := resources["rhel_1_port_0"]; ok {
		t.Fatalf("create-mode rewrite must not invent wrap port names, got %#v", resources)
	}
	port := resources["rhel_1_port"].(map[string]interface{})
	if port["external_id"] != "port-uuid" {
		t.Fatalf("expected port external_id, got %#v", port)
	}
	if _, ok := port["properties"]; ok {
		t.Fatalf("rewritten port must drop properties, got %#v", port)
	}
	instance := resources["rhel_1_instance"].(map[string]interface{})
	if instance["external_id"] != "server-uuid" {
		t.Fatalf("expected instance external_id, got %#v", instance)
	}
	volume := resources["rhel_1_boot_volume"].(map[string]interface{})
	if _, ok := volume["external_id"]; !ok {
		t.Fatalf("volume external_id must stay, got %#v", volume)
	}
	if _, isString := volume["external_id"].(string); isString {
		t.Fatalf("volume should keep get_param mapping, got %#v", volume["external_id"])
	}
}

func TestRewriteSkipsAlreadyExternalResources(t *testing.T) {
	vmsData := []generateheattemplate.VMData{
		{
			Name:         "rhel-1",
			BootVolumeID: "boot-volume-uuid",
			InstanceID:   "server-uuid",
			PortIDs:      []string{"port-uuid"},
		},
	}
	raw, _, err := generateheattemplate.GenerateWrapHeatTemplate(vmsData, "os-migrate-wrapped")
	if err != nil {
		t.Fatalf("wrap template failed: %v", err)
	}
	template := parseGeneratedTemplate(t, raw)
	rewritten, changed, err := abandonheatstack.RewriteHeatManagedResources(template, []abandonheatstack.ResourceRef{
		{Name: "rhel_1_port_0", LogicalID: "rhel_1_port_0", PhysicalID: "port-uuid", Type: "OS::Neutron::Port"},
		{Name: "rhel_1_instance", LogicalID: "rhel_1_instance", PhysicalID: "server-uuid", Type: "OS::Nova::Server"},
	})
	if err != nil {
		t.Fatalf("rewrite failed: %v", err)
	}
	if changed {
		t.Fatal("wrap resources already have external_id; rewrite should be a no-op")
	}
	resources := rewritten["resources"].(map[string]interface{})
	if _, ok := resources["rhel_1_port"]; ok {
		t.Fatalf("wrap rewrite must keep indexed port names, got %#v", resources)
	}
	if _, ok := resources["rhel_1_port_0"]; !ok {
		t.Fatalf("expected wrap port name rhel_1_port_0, got %#v", resources)
	}
}

func TestRewriteErrorsWithoutPhysicalID(t *testing.T) {
	template := map[string]interface{}{
		"resources": map[string]interface{}{
			"rhel_1_port": map[string]interface{}{
				"type": "OS::Neutron::Port",
				"properties": map[string]interface{}{
					"network": "net-1",
				},
			},
		},
	}
	_, _, err := abandonheatstack.RewriteHeatManagedResources(template, []abandonheatstack.ResourceRef{
		{Name: "rhel_1_port", LogicalID: "rhel_1_port", Type: "OS::Neutron::Port"},
	})
	if err == nil {
		t.Fatal("expected error when physical ID is missing")
	}
}

func TestRewriteErrorsWhenLiveResourceMissing(t *testing.T) {
	template := map[string]interface{}{
		"resources": map[string]interface{}{
			"rhel_1_instance": map[string]interface{}{
				"type":       "OS::Nova::Server",
				"properties": map[string]interface{}{"name": "rhel-1"},
			},
		},
	}
	_, _, err := abandonheatstack.RewriteHeatManagedResources(template, nil)
	if err == nil {
		t.Fatal("expected error when live resource is missing")
	}
}

func TestModuleArgsUnmarshalStackName(t *testing.T) {
	raw := `{"stack_name": "os-migrate-1710000000", "wait": true, "timeout": 600}`
	var args abandonheatstack.ModuleArgs
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if args.StackName != "os-migrate-1710000000" {
		t.Errorf("expected stack_name, got %q", args.StackName)
	}
	if !args.Wait {
		t.Fatal("expected wait true")
	}
}
