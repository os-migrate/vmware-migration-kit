#!/usr/bin/python

DOCUMENTATION = r"""
---
module: create_heat_stack
short_description: Create Heat stack from template for migrated VMware workloads
extends_documentation_fragment:
    - os_migrate.vmware_migration_kit.openstack
version_added: "2.9.0"
author: "OpenStack tenant migration tools (@os-migrate)"
description:
  - "Create a Heat orchestration stack from a generated template."
  - "Optionally wait for stack creation to complete."
options:
  cloud:
    description:
      - Cloud credentials for OpenStack authentication.
    required: true
    type: raw
  template_path:
    description:
      - Path to the Heat template YAML file.
    required: true
    type: str
  stack_name:
    description:
      - Name for the Heat stack.
    required: true
    type: str
  parameters:
    description:
      - Parameters to pass to the Heat template.
    required: false
    type: dict
    default: {}
  wait:
    description:
      - Whether to wait for stack creation to complete.
    required: false
    type: bool
    default: true
  timeout:
    description:
      - Timeout in seconds for stack creation (default 600 = 10 minutes).
    required: false
    type: int
    default: 600
"""

EXAMPLES = r"""
- name: Create Heat stack from template
  os_migrate.vmware_migration_kit.create_heat_stack:
    cloud: "{{ dst_cloud }}"
    template_path: "/opt/os-migrate/heat_template.yaml"
    stack_name: "os-migrate-20240120"
    parameters:
      rhel_1_boot_volume_id: "volume-uuid-1"
      rhel_2_boot_volume_id: "volume-uuid-2"
      security_group_id: "sg-uuid"
    wait: true
    timeout: 600
  register: heat_stack_result

- name: Display stack creation result
  ansible.builtin.debug:
    msg: "Stack {{ heat_stack_result.stack.name }} created with status {{ heat_stack_result.stack.status }}"
"""

RETURN = r"""
stack:
    description: Information about the created Heat stack
    returned: success
    type: dict
    sample: {"id": "stack-uuid", "name": "os-migrate-20240120", "status": "CREATE_COMPLETE"}
"""
