#!/usr/bin/python

DOCUMENTATION = r"""
---
module: abandon_heat_stack
short_description: Leave a Heat stack without deleting migrated VMs
extends_documentation_fragment:
    - os_migrate.vmware_migration_kit.openstack
version_added: "2.9.0"
author: "OpenStack tenant migration tools (@os-migrate)"
description:
  - "Remove a create-mode use_heat stack without deleting Nova instances or Neutron ports."
  - "Tries Heat stacks.Abandon first. After a successful Abandon the stack is gone and no extra wait is needed."
  - "If Abandon is disabled (enable_stack_abandon off, typical on RHOS/PSI), rewrites Heat-managed OS::Nova::Server and OS::Neutron::Port resources to external_id using the stack's live logical names, then deletes the stack."
  - "Create-mode stacks are named os-migrate-<epoch>; this module cannot infer that name."
  - "Do not use this for wrap stacks. Deleting a wrap stack already leaves VMs in place."
options:
  cloud:
    description:
      - Cloud credentials for OpenStack authentication.
    required: true
    type: raw
  stack_name:
    description:
      - Name of the existing Heat stack to abandon.
      - Required. Create-mode heat_deploy.yml names stacks os-migrate-{{ ansible_date_time.epoch }}.
    required: true
    type: str
  wait:
    description:
      - Wait for fallback stack update and delete to finish. Unused after a successful Abandon.
    required: false
    type: bool
    default: true
  timeout:
    description:
      - Timeout in seconds for fallback update and delete (default 600 = 10 minutes).
    required: false
    type: int
    default: 600
  output_dir:
    description:
      - Directory where heat_stack_abandon.txt will be written.
      - When set, the file is written on the same host that runs the module, at the exact path specified.
    required: false
    type: str
"""

EXAMPLES = r"""
- name: Abandon a create-mode Heat stack without deleting VMs
  os_migrate.vmware_migration_kit.abandon_heat_stack:
    cloud: "{{ dst_cloud }}"
    stack_name: "os-migrate-1710000000"
    wait: true
    timeout: 600
    output_dir: "/opt/os-migrate"
  register: heat_stack_abandoned

- name: Display abandon result
  ansible.builtin.debug:
    msg: "Stack {{ heat_stack_abandoned.stack.name }} left via {{ heat_stack_abandoned.method }}"
"""

RETURN = r"""
method:
    description: How the stack was left (abandon, fallback, or skip)
    returned: success
    type: str
    sample: fallback
stack:
    description: Information about the Heat stack
    returned: success
    type: dict
    sample: {"id": "stack-uuid", "name": "os-migrate-1710000000", "status": "DELETE_COMPLETE"}
info_path:
  description: Path to the written heat_stack_abandon.txt file
  returned: when output_dir is provided
  type: str
  sample: "/opt/os-migrate/heat_stack_abandon.txt"
"""
