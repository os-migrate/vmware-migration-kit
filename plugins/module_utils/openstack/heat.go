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

package openstack

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/orchestration/v1/stacks"
	"gopkg.in/yaml.v3"
)

// HeatStackInfo contains information about an adopted Heat stack
type HeatStackInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// AdoptResourcesIntoHeatStack adopts existing OpenStack resources into a Heat stack
func AdoptResourcesIntoHeatStack(ctx context.Context, provider *gophercloud.ProviderClient, stackName string, volumeIDs []string, instanceID string) (*HeatStackInfo, error) {
	// Create Heat client
	heatClient, err := openstack.NewOrchestrationV1(provider, gophercloud.EndpointOpts{})
	if err != nil {
		return nil, fmt.Errorf("failed to create Heat client: %w", err)
	}

	// Generate adoption data
	adoptionData := generateAdoptionDataForResources(stackName, volumeIDs, instanceID)
	adoptJSON, err := json.Marshal(adoptionData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal adoption data: %w", err)
	}

	// Generate Heat template
	templateYAML := generateHeatTemplate(stackName, len(volumeIDs), instanceID != "")
	var templateMap map[string]interface{}
	if err := yaml.Unmarshal([]byte(templateYAML), &templateMap); err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	template := &stacks.Template{}
	template.Bin = []byte(templateYAML)
	template.Parsed = templateMap

	// Adopt stack
	disableRollback := true
	adoptOpts := stacks.AdoptOpts{
		Name:            stackName,
		TemplateOpts:    template,
		AdoptStackData:  string(adoptJSON),
		Timeout:         15,
		DisableRollback: &disableRollback,
	}

	result := stacks.Adopt(ctx, heatClient, adoptOpts)
	if result.Err != nil {
		return nil, fmt.Errorf("failed to adopt stack: %w", result.Err)
	}

	adoptedStack, err := result.Extract()
	if err != nil {
		return nil, fmt.Errorf("failed to extract adopted stack: %w", err)
	}

	return &HeatStackInfo{
		ID:     adoptedStack.ID,
		Name:   stackName,
		Status: "ADOPT_IN_PROGRESS",
	}, nil
}

// generateAdoptionDataForResources creates adoption data structure
func generateAdoptionDataForResources(stackName string, volumeIDs []string, instanceID string) map[string]interface{} {
	resources := make(map[string]interface{})

	// Add volumes
	for i, volID := range volumeIDs {
		resourceName := fmt.Sprintf("volume_%d", i)
		resources[resourceName] = map[string]interface{}{
			"status":        "COMPLETE",
			"name":          resourceName,
			"resource_id":   volID,
			"action":        "CREATE",
			"type":          "OS::Cinder::Volume",
			"resource_data": map[string]interface{}{},
			"metadata":      map[string]interface{}{},
		}
	}

	// Add instance if provided
	if instanceID != "" {
		resources["migrated_instance"] = map[string]interface{}{
			"status":        "COMPLETE",
			"name":          "migrated_instance",
			"resource_id":   instanceID,
			"action":        "CREATE",
			"type":          "OS::Nova::Server",
			"resource_data": map[string]interface{}{},
			"metadata":      map[string]interface{}{},
		}
	}

	// Generate template for adoption data
	templateYAML := generateHeatTemplate(stackName, len(volumeIDs), instanceID != "")
	var templateMap map[string]interface{}
	yaml.Unmarshal([]byte(templateYAML), &templateMap)

	return map[string]interface{}{
		"action":    "CREATE",
		"status":    "COMPLETE",
		"name":      stackName,
		"id":        "manual-adoption",
		"resources": resources,
		"template":  templateMap,
		"environment": map[string]interface{}{
			"parameters": map[string]interface{}{},
		},
		"parameters": map[string]interface{}{},
	}
}

// generateHeatTemplate creates Heat template for migrated resources
func generateHeatTemplate(stackName string, volumeCount int, hasInstance bool) string {
	template := `heat_template_version: wallaby
description: Migrated VMware workload - %s

resources:
`
	template = fmt.Sprintf(template, stackName)

	// Add volume resources
	for i := 0; i < volumeCount; i++ {
		template += fmt.Sprintf(`  volume_%d:
    type: OS::Cinder::Volume
    properties:
      size: 1
`, i)
	}

	// Add instance resource if needed
	if hasInstance {
		template += `  migrated_instance:
    type: OS::Nova::Server
    properties:
      name: migrated-instance
      flavor: m1.small
`
	}

	// Add outputs
	template += `
outputs:
`
	for i := 0; i < volumeCount; i++ {
		template += fmt.Sprintf(`  volume_%d_id:
    description: Volume %d ID
    value: { get_resource: volume_%d }
`, i, i, i)
	}

	if hasInstance {
		template += `  instance_id:
    description: Instance ID
    value: { get_resource: migrated_instance }
`
	}

	return template
}
