#!/usr/bin/python

DOCUMENTATION = r"""
module: adopt_heat_stack
short_description: Adopts existing OpenStack resources into a Heat stack
description:
  - This module adopts existing Cinder volumes and Nova instances into a Heat-managed stack.
  - Useful for bringing migrated VMware resources under Heat infrastructure-as-code management.
  - Requires Heat service to have stack adoption enabled (enable_stack_adopt = True).
author: "OpenStack tenant migration tools (@os-migrate)"
version_added: "2.0.0"
options:
  cloud:
    description:
      - Authentication parameters for OpenStack.
      - Can include auth_url, username, password, project_name, domain_name, etc.
    required: true
    type: dict
  stack_name:
    description:
      - Name for the Heat stack to create.
      - Should be descriptive of the migrated workload.
    required: true
    type: str
  volume_ids:
    description:
      - List of Cinder volume UUIDs to adopt into the stack.
      - Can include boot volumes and data volumes.
    required: false
    type: list
    elements: str
    default: []
  instance_id:
    description:
      - Nova instance UUID to adopt into the stack.
      - If provided, the instance will be managed by Heat.
    required: false
    type: str
requirements:
  - gophercloud v2
notes:
  - Heat service must have enable_stack_adopt = True in configuration
  - Resources must be in a stable state before adoption
  - Adoption preserves resource IDs and states
"""

EXAMPLES = r"""
# Adopt migrated volumes into Heat stack
- name: Adopt Cinder volumes into Heat
  os_migrate.vmware_migration_kit.adopt_heat_stack:
    cloud: "{{ dst_cloud }}"
    stack_name: "migrated-vm-001-stack"
    volume_ids:
      - "{{ boot_volume_uuid }}"
      - "{{ data_volume_uuid }}"

# Adopt instance with volumes
- name: Adopt complete migrated workload
  os_migrate.vmware_migration_kit.adopt_heat_stack:
    cloud: "{{ dst_cloud }}"
    stack_name: "{{ vm_name }}-stack"
    volume_ids: "{{ [boot_volume_uuid] + volumes_list }}"
    instance_id: "{{ instance_id }}"
  register: heat_stack

- name: Display adopted stack info
  ansible.builtin.debug:
    msg: "Stack {{ heat_stack.stack.name }} ({{ heat_stack.stack.id }}) adopted"
"""

RETURN = r"""
stack:
  description: Information about the adopted Heat stack.
  returned: success
  type: dict
  contains:
    id:
      description: Heat stack UUID
      type: str
      sample: "f8f47a10-d68a-4b12-a9ba-c8f13ef3fc39"
    name:
      description: Stack name
      type: str
      sample: "migrated-vm-001-stack"
    status:
      description: Stack status
      type: str
      sample: "ADOPT_IN_PROGRESS"
msg:
  description: Success or error message
  returned: always
  type: str
  sample: "Successfully adopted resources into Heat stack 'migrated-vm-001-stack'"
"""
