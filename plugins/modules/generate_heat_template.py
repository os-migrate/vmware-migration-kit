#!/usr/bin/python

DOCUMENTATION = r"""
---
module: generate_heat_template
short_description: Generate Heat template for migrated VMware workloads
extends_documentation_fragment:
    - os_migrate.vmware_migration_kit.openstack
version_added: "2.9.0"
author: "OpenStack tenant migration tools (@os-migrate)"
description:
  - "Generate a Heat orchestration template that references existing Cinder volumes and creates OpenStack instances."
  - "Cinder volumes are referenced as external resources (unmanaged by Heat)."
  - "Neutron ports and Nova instances are created and managed by Heat."
options:
  vms_data:
    description:
      - List of VM data dictionaries containing migration information.
      - Each VM must have name, boot_volume_id, flavor, network, and security_groups.
    required: true
    type: list
    elements: dict
  stack_name:
    description:
      - Name for the Heat stack.
    required: true
    type: str
  output_dir:
    description:
      - Directory where the Heat template will be saved.
    required: true
    type: str
"""

EXAMPLES = r"""
- name: Generate Heat template for migrated VMs
  os_migrate.vmware_migration_kit.generate_heat_template:
    vms_data:
      - name: rhel-1
        boot_volume_id: "volume-uuid-1"
        flavor: "m1.medium"
        network: "provider_network_1"
        security_groups: ["default"]
      - name: rhel-2
        boot_volume_id: "volume-uuid-2"
        flavor: "m1.medium"
        network: "provider_network_1"
        security_groups: ["default"]
    stack_name: "os-migrate-20240120"
    output_dir: "/opt/os-migrate"
  register: heat_template_result

- name: Display generated template path
  ansible.builtin.debug:
    msg: "Template generated at {{ heat_template_result.template_path }}"
"""

RETURN = r"""
template_path:
    description: Path to the generated Heat template file
    returned: success
    type: str
    sample: "/opt/os-migrate/heat_template.yaml"
stack_name:
    description: Name of the Heat stack
    returned: success
    type: str
    sample: "os-migrate-20240120"
parameters:
    description: Parameters map for the Heat template
    returned: success
    type: dict
    sample: {"rhel_1_boot_volume_id": "volume-uuid-1", "rhel_2_boot_volume_id": "volume-uuid-2"}
"""
