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

package moduleutils

import (
	"strings"
	"testing"

	generateheattemplate "vmware-migration-kit/plugins/modules/src/generate_heat_template"
)

func TestGenerateHeatTemplateWithDataVolumes(t *testing.T) {
	vmsData := []generateheattemplate.VMData{
		{
			Name:           "vm-02-2d",
			BootVolumeID:   "boot-volume-uuid",
			Flavor:         "flavor-uuid",
			Network:        "network-uuid",
			SecurityGroups: []string{"default"},
			DataVolumeIDs:  []string{"data-volume-uuid"},
		},
	}

	template, parameters := generateheattemplate.GenerateHeatTemplate(vmsData, "os-migrate-test")

	if !strings.Contains(template, "vm_02_2d_data_volume_0_id") {
		t.Fatalf("expected data volume parameter in template, got:\n%s", template)
	}

	if !strings.Contains(template, "vm_02_2d_data_volume_0:") {
		t.Fatalf("expected data volume resource in template, got:\n%s", template)
	}

	if !strings.Contains(template, "volume_id: { get_resource: vm_02_2d_data_volume_0 }") {
		t.Fatalf("expected data volume in block_device_mapping_v2, got:\n%s", template)
	}

	if parameters["vm_02_2d_boot_volume_id"] != "boot-volume-uuid" {
		t.Fatalf("expected boot volume parameter, got %#v", parameters["vm_02_2d_boot_volume_id"])
	}

	if parameters["vm_02_2d_data_volume_0_id"] != "data-volume-uuid" {
		t.Fatalf("expected data volume parameter, got %#v", parameters["vm_02_2d_data_volume_0_id"])
	}
}

func TestGenerateHeatTemplateIgnoresWrapFields(t *testing.T) {
	vmsData := []generateheattemplate.VMData{
		{
			Name:           "rhel-1",
			BootVolumeID:   "boot-volume-uuid",
			Flavor:         "flavor-uuid",
			Network:        "network-uuid",
			SecurityGroups: []string{"default"},
			InstanceID:     "server-uuid",
			PortIDs:        []string{"port-uuid"},
		},
	}

	template, _ := generateheattemplate.GenerateHeatTemplate(vmsData, "os-migrate-test")

	if !strings.Contains(template, "properties:") {
		t.Fatalf("create-mode should still emit properties, got:\n%s", template)
	}
	if strings.Contains(template, "external_id: { get_param: rhel_1_instance_id }") {
		t.Fatalf("create-mode must not wrap the Nova server, got:\n%s", template)
	}
	if !strings.Contains(template, "  rhel_1_port:\n") {
		t.Fatalf("create-mode ports stay unindexed, got:\n%s", template)
	}
}

func TestGenerateWrapHeatTemplateExternalIDsOnly(t *testing.T) {
	vmsData := []generateheattemplate.VMData{
		{
			Name:          "rhel-1",
			BootVolumeID:  "boot-volume-uuid",
			DataVolumeIDs: []string{"data-volume-uuid"},
			InstanceID:    "server-uuid",
			PortIDs:       []string{"port-uuid-a", "port-uuid-b"},
		},
	}

	template, parameters, err := generateheattemplate.GenerateWrapHeatTemplate(vmsData, "os-migrate-wrapped")
	if err != nil {
		t.Fatalf("GenerateWrapHeatTemplate failed: %v", err)
	}

	if strings.Contains(template, "properties:") {
		t.Fatalf("wrap template must not set properties, got:\n%s", template)
	}
	if strings.Contains(template, "block_device_mapping_v2") {
		t.Fatalf("wrap template must not map block devices, got:\n%s", template)
	}
	if strings.Contains(template, "security_group_id") {
		t.Fatalf("wrap template must not create ports with security groups, got:\n%s", template)
	}
	if !strings.Contains(template, "external_id: { get_param: rhel_1_instance_id }") {
		t.Fatalf("expected instance external_id, got:\n%s", template)
	}
	if !strings.Contains(template, "external_id: { get_param: rhel_1_port_0_id }") {
		t.Fatalf("expected port 0 external_id, got:\n%s", template)
	}
	if !strings.Contains(template, "external_id: { get_param: rhel_1_boot_volume_id }") {
		t.Fatalf("expected boot volume external_id, got:\n%s", template)
	}
	if parameters["rhel_1_instance_id"] != "server-uuid" {
		t.Fatalf("expected instance parameter, got %#v", parameters["rhel_1_instance_id"])
	}
	if parameters["rhel_1_port_1_id"] != "port-uuid-b" {
		t.Fatalf("expected second port parameter, got %#v", parameters["rhel_1_port_1_id"])
	}
}

func TestValidateWrapVMData(t *testing.T) {
	err := generateheattemplate.ValidateWrapVMData(nil)
	if err == nil {
		t.Fatal("expected error for empty wrap data")
	}

	err = generateheattemplate.ValidateWrapVMData([]generateheattemplate.VMData{{
		Name:         "rhel-1",
		BootVolumeID: "boot-uuid",
		PortIDs:      []string{"port-uuid"},
	}})
	if err == nil {
		t.Fatal("expected error when instance_id is missing")
	}
}

func TestGenerateHeatTemplateBootVolumeOnly(t *testing.T) {
	vmsData := []generateheattemplate.VMData{
		{
			Name:           "rhel-1",
			BootVolumeID:   "boot-volume-uuid",
			Flavor:         "flavor-uuid",
			Network:        "network-uuid",
			SecurityGroups: []string{"default"},
		},
	}

	template, parameters := generateheattemplate.GenerateHeatTemplate(vmsData, "os-migrate-test")

	if strings.Contains(template, "data_volume_0") {
		t.Fatalf("did not expect data volume resources for single-disk VM, got:\n%s", template)
	}

	if len(parameters) != 2 {
		t.Fatalf("expected boot and security group parameters only, got %#v", parameters)
	}
}
