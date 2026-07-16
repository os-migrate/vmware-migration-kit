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
package generate_heat_template

import (
	"strings"
	"testing"
)

func TestGenerateHeatTemplateWithDataVolumes(t *testing.T) {
	vmsData := []VMData{
		{
			Name:           "vm-02-2d",
			BootVolumeID:   "boot-volume-uuid",
			Flavor:         "flavor-uuid",
			Network:        "network-uuid",
			SecurityGroups: []string{"default"},
			DataVolumeIDs:  []string{"data-volume-uuid"},
		},
	}

	template, parameters := generateHeatTemplate(vmsData, "os-migrate-test")

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

func TestGenerateHeatTemplateBootVolumeOnly(t *testing.T) {
	vmsData := []VMData{
		{
			Name:           "rhel-1",
			BootVolumeID:   "boot-volume-uuid",
			Flavor:         "flavor-uuid",
			Network:        "network-uuid",
			SecurityGroups: []string{"default"},
		},
	}

	template, parameters := generateHeatTemplate(vmsData, "os-migrate-test")

	if strings.Contains(template, "data_volume_0") {
		t.Fatalf("did not expect data volume resources for single-disk VM, got:\n%s", template)
	}

	if len(parameters) != 2 {
		t.Fatalf("expected boot and security group parameters only, got %#v", parameters)
	}
}
