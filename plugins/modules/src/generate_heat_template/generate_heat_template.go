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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"vmware-migration-kit/plugins/module_utils/ansible"
)

// Ansible module args
type ModuleArgs struct {
	VMsData      []VMData `json:"vms_data"`
	StackName    string   `json:"stack_name"`
	OutputDir    string   `json:"output_dir"`
	WrapExisting bool     `json:"wrap_existing"`
}

type VMData struct {
	Name           string   `json:"name"`
	BootVolumeID   string   `json:"boot_volume_id"`
	Flavor         string   `json:"flavor"`
	Network        string   `json:"network"`
	SecurityGroups []string `json:"security_groups"`
	DataVolumeIDs  []string `json:"data_volume_ids,omitempty"`
	InstanceID     string   `json:"instance_id,omitempty"`
	PortIDs        []string `json:"port_ids,omitempty"`
}

type Response struct {
	Msg          string                 `json:"msg"`
	Changed      bool                   `json:"changed"`
	Failed       bool                   `json:"failed"`
	TemplatePath string                 `json:"template_path,omitempty"`
	StackName    string                 `json:"stack_name,omitempty"`
	Parameters   map[string]interface{} `json:"parameters,omitempty"`
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

func sanitizeName(name string) string {
	// Replace invalid characters for Heat resource names
	sanitized := strings.ReplaceAll(name, "-", "_")
	sanitized = strings.ReplaceAll(sanitized, ".", "_")
	return sanitized
}

func GenerateHeatTemplate(vmsData []VMData, stackName string) (string, map[string]interface{}) {
	var template strings.Builder
	parameters := make(map[string]interface{})

	// Header
	template.WriteString("heat_template_version: wallaby\n")
	template.WriteString(fmt.Sprintf("description: 'Migrated VMware workloads managed by os-migrate (Stack - %s)'\n\n", stackName))

	// Parameters section
	template.WriteString("parameters:\n")
	for _, vm := range vmsData {
		sanitized := sanitizeName(vm.Name)
		paramName := fmt.Sprintf("%s_boot_volume_id", sanitized)
		template.WriteString(fmt.Sprintf("  %s:\n", paramName))
		template.WriteString("    type: string\n")
		template.WriteString(fmt.Sprintf("    description: Boot volume ID for %s\n", vm.Name))
		parameters[paramName] = vm.BootVolumeID

		// Add data volume parameters if present
		for i, volID := range vm.DataVolumeIDs {
			dataParamName := fmt.Sprintf("%s_data_volume_%d_id", sanitized, i)
			template.WriteString(fmt.Sprintf("  %s:\n", dataParamName))
			template.WriteString("    type: string\n")
			template.WriteString(fmt.Sprintf("    description: Data volume %d ID for %s\n", i, vm.Name))
			parameters[dataParamName] = volID
		}
	}

	// Add common parameters
	template.WriteString("  security_group_id:\n")
	template.WriteString("    type: string\n")
	template.WriteString("    description: Security group ID for instances\n")
	if len(vmsData) > 0 && len(vmsData[0].SecurityGroups) > 0 {
		parameters["security_group_id"] = vmsData[0].SecurityGroups[0]
	}

	// Resources section
	template.WriteString("\nresources:\n")

	for _, vm := range vmsData {
		sanitized := sanitizeName(vm.Name)

		// External Cinder boot volume (unmanaged by Heat)
		template.WriteString(fmt.Sprintf("  %s_boot_volume:\n", sanitized))
		template.WriteString("    type: OS::Cinder::Volume\n")
		template.WriteString(fmt.Sprintf("    external_id: { get_param: %s_boot_volume_id }\n\n", sanitized))

		// External data volumes if present
		for i := range vm.DataVolumeIDs {
			template.WriteString(fmt.Sprintf("  %s_data_volume_%d:\n", sanitized, i))
			template.WriteString("    type: OS::Cinder::Volume\n")
			template.WriteString(fmt.Sprintf("    external_id: { get_param: %s_data_volume_%d_id }\n\n", sanitized, i))
		}

		// Neutron port (managed by Heat)
		template.WriteString(fmt.Sprintf("  %s_port:\n", sanitized))
		template.WriteString("    type: OS::Neutron::Port\n")
		template.WriteString("    properties:\n")
		template.WriteString(fmt.Sprintf("      network: %s\n", vm.Network))
		template.WriteString("      security_groups:\n")
		template.WriteString("        - { get_param: security_group_id }\n\n")

		// Nova instance (managed by Heat)
		template.WriteString(fmt.Sprintf("  %s_instance:\n", sanitized))
		template.WriteString("    type: OS::Nova::Server\n")
		template.WriteString("    properties:\n")
		template.WriteString(fmt.Sprintf("      name: %s\n", vm.Name))
		template.WriteString(fmt.Sprintf("      flavor: %s\n", vm.Flavor))
		template.WriteString("      block_device_mapping_v2:\n")
		template.WriteString(fmt.Sprintf("        - volume_id: { get_resource: %s_boot_volume }\n", sanitized))
		template.WriteString("          boot_index: 0\n")
		template.WriteString("          delete_on_termination: false\n")

		// Add data volumes to block device mapping
		for i := range vm.DataVolumeIDs {
			template.WriteString(fmt.Sprintf("        - volume_id: { get_resource: %s_data_volume_%d }\n", sanitized, i))
			template.WriteString(fmt.Sprintf("          boot_index: %d\n", i+1))
			template.WriteString("          delete_on_termination: false\n")
		}

		template.WriteString("      networks:\n")
		template.WriteString(fmt.Sprintf("        - port: { get_resource: %s_port }\n\n", sanitized))
	}

	// Outputs section
	template.WriteString("outputs:\n")
	for _, vm := range vmsData {
		sanitized := sanitizeName(vm.Name)
		template.WriteString(fmt.Sprintf("  %s_instance_id:\n", sanitized))
		template.WriteString(fmt.Sprintf("    description: Instance ID for %s\n", vm.Name))
		template.WriteString(fmt.Sprintf("    value: { get_resource: %s_instance }\n", sanitized))
		template.WriteString(fmt.Sprintf("  %s_port_id:\n", sanitized))
		template.WriteString(fmt.Sprintf("    description: Port ID for %s\n", vm.Name))
		template.WriteString(fmt.Sprintf("    value: { get_resource: %s_port }\n", sanitized))
	}

	return template.String(), parameters
}

func ValidateWrapVMData(vmsData []VMData) error {
	if len(vmsData) == 0 {
		return fmt.Errorf("no VMs data provided")
	}
	for _, vm := range vmsData {
		if vm.Name == "" {
			return fmt.Errorf("VM name is required for wrap")
		}
		if vm.InstanceID == "" {
			return fmt.Errorf("instance_id is required to wrap VM %s", vm.Name)
		}
		if vm.BootVolumeID == "" {
			return fmt.Errorf("boot_volume_id is required to wrap VM %s", vm.Name)
		}
		if len(vm.PortIDs) == 0 {
			return fmt.Errorf("port_ids is required to wrap VM %s", vm.Name)
		}
		for i, portID := range vm.PortIDs {
			if portID == "" {
				return fmt.Errorf("port_ids[%d] is empty for VM %s", i, vm.Name)
			}
		}
	}
	return nil
}

func writeVolumeParams(template *strings.Builder, parameters map[string]interface{}, vm VMData, sanitized string) {
	paramName := fmt.Sprintf("%s_boot_volume_id", sanitized)
	template.WriteString(fmt.Sprintf("  %s:\n", paramName))
	template.WriteString("    type: string\n")
	template.WriteString(fmt.Sprintf("    description: Boot volume ID for %s\n", vm.Name))
	parameters[paramName] = vm.BootVolumeID

	for i, volID := range vm.DataVolumeIDs {
		dataParamName := fmt.Sprintf("%s_data_volume_%d_id", sanitized, i)
		template.WriteString(fmt.Sprintf("  %s:\n", dataParamName))
		template.WriteString("    type: string\n")
		template.WriteString(fmt.Sprintf("    description: Data volume %d ID for %s\n", i, vm.Name))
		parameters[dataParamName] = volID
	}
}

func writeExternalVolumeResources(template *strings.Builder, vm VMData, sanitized string) {
	template.WriteString(fmt.Sprintf("  %s_boot_volume:\n", sanitized))
	template.WriteString("    type: OS::Cinder::Volume\n")
	template.WriteString(fmt.Sprintf("    external_id: { get_param: %s_boot_volume_id }\n\n", sanitized))

	for i := range vm.DataVolumeIDs {
		template.WriteString(fmt.Sprintf("  %s_data_volume_%d:\n", sanitized, i))
		template.WriteString("    type: OS::Cinder::Volume\n")
		template.WriteString(fmt.Sprintf("    external_id: { get_param: %s_data_volume_%d_id }\n\n", sanitized, i))
	}
}

// GenerateWrapHeatTemplate references existing Nova servers, Neutron ports, and
// Cinder volumes via external_id only. Heat must not create or rebuild them.
func GenerateWrapHeatTemplate(vmsData []VMData, stackName string) (string, map[string]interface{}, error) {
	if err := ValidateWrapVMData(vmsData); err != nil {
		return "", nil, err
	}

	var template strings.Builder
	parameters := make(map[string]interface{})

	template.WriteString("heat_template_version: wallaby\n")
	template.WriteString(fmt.Sprintf("description: 'Existing migrated workloads wrapped by os-migrate (Stack - %s)'\n\n", stackName))

	template.WriteString("parameters:\n")
	for _, vm := range vmsData {
		sanitized := sanitizeName(vm.Name)
		writeVolumeParams(&template, parameters, vm, sanitized)

		instanceParam := fmt.Sprintf("%s_instance_id", sanitized)
		template.WriteString(fmt.Sprintf("  %s:\n", instanceParam))
		template.WriteString("    type: string\n")
		template.WriteString(fmt.Sprintf("    description: Existing Nova instance ID for %s\n", vm.Name))
		parameters[instanceParam] = vm.InstanceID

		for i, portID := range vm.PortIDs {
			portParam := fmt.Sprintf("%s_port_%d_id", sanitized, i)
			template.WriteString(fmt.Sprintf("  %s:\n", portParam))
			template.WriteString("    type: string\n")
			template.WriteString(fmt.Sprintf("    description: Existing Neutron port %d ID for %s\n", i, vm.Name))
			parameters[portParam] = portID
		}
	}

	template.WriteString("\nresources:\n")
	for _, vm := range vmsData {
		sanitized := sanitizeName(vm.Name)
		writeExternalVolumeResources(&template, vm, sanitized)

		for i := range vm.PortIDs {
			template.WriteString(fmt.Sprintf("  %s_port_%d:\n", sanitized, i))
			template.WriteString("    type: OS::Neutron::Port\n")
			template.WriteString(fmt.Sprintf("    external_id: { get_param: %s_port_%d_id }\n\n", sanitized, i))
		}

		template.WriteString(fmt.Sprintf("  %s_instance:\n", sanitized))
		template.WriteString("    type: OS::Nova::Server\n")
		template.WriteString(fmt.Sprintf("    external_id: { get_param: %s_instance_id }\n\n", sanitized))
	}

	template.WriteString("outputs:\n")
	for _, vm := range vmsData {
		sanitized := sanitizeName(vm.Name)
		template.WriteString(fmt.Sprintf("  %s_instance_id:\n", sanitized))
		template.WriteString(fmt.Sprintf("    description: Instance ID for %s\n", vm.Name))
		template.WriteString(fmt.Sprintf("    value: { get_resource: %s_instance }\n", sanitized))
		for i := range vm.PortIDs {
			template.WriteString(fmt.Sprintf("  %s_port_%d_id:\n", sanitized, i))
			template.WriteString(fmt.Sprintf("    description: Port %d ID for %s\n", i, vm.Name))
			template.WriteString(fmt.Sprintf("    value: { get_resource: %s_port_%d }\n", sanitized, i))
		}
	}

	return template.String(), parameters, nil
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
	if len(moduleArgs.VMsData) == 0 {
		ansible.FailJson(ansible.Response{Msg: "No VMs data provided"})
	}

	if moduleArgs.StackName == "" {
		ansible.FailJson(ansible.Response{Msg: "Stack name is required"})
	}

	if moduleArgs.OutputDir == "" {
		ansible.FailJson(ansible.Response{Msg: "Output directory is required"})
	}

	var templateContent string
	var parameters map[string]interface{}
	if moduleArgs.WrapExisting {
		var errWrap error
		templateContent, parameters, errWrap = GenerateWrapHeatTemplate(moduleArgs.VMsData, moduleArgs.StackName)
		if errWrap != nil {
			ansible.FailJson(ansible.Response{Msg: "Failed to generate wrap Heat template: " + errWrap.Error()})
		}
	} else {
		templateContent, parameters = GenerateHeatTemplate(moduleArgs.VMsData, moduleArgs.StackName)
	}

	// Ensure output directory exists
	err = os.MkdirAll(moduleArgs.OutputDir, 0755)
	if err != nil {
		ansible.FailJson(ansible.Response{Msg: "Failed to create output directory: " + err.Error()})
	}

	// Write template to file
	templatePath := filepath.Join(moduleArgs.OutputDir, "heat_template.yaml")
	err = os.WriteFile(templatePath, []byte(templateContent), 0644)
	if err != nil {
		ansible.FailJson(ansible.Response{Msg: "Failed to write Heat template: " + err.Error()})
	}

	response := Response{
		Changed:      true,
		Msg:          fmt.Sprintf("Heat template generated successfully for %d VMs", len(moduleArgs.VMsData)),
		TemplatePath: templatePath,
		StackName:    moduleArgs.StackName,
		Parameters:   parameters,
	}
	exitJson(response)
}
