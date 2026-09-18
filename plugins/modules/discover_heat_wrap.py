#!/usr/bin/python

DOCUMENTATION = r"""
---
module: discover_heat_wrap
short_description: Discover existing migrated instances for Heat wrap
extends_documentation_fragment:
    - os_migrate.vmware_migration_kit.openstack
version_added: "2.9.0"
author: "OpenStack tenant migration tools (@os-migrate)"
description:
  - "Look up already-migrated Nova instances by exact name and collect attached Neutron ports and Cinder volumes."
  - "Output is ready for generate_heat_template with wrap_existing true."
  - "Fails if an instance already belongs to a Heat stack (OS::stack_id or metering.stack_id)."
options:
  cloud:
    description:
      - Cloud credentials for OpenStack authentication.
    required: true
    type: raw
  vm_names:
    description:
      - Exact Nova server names to wrap. Nova name filters are regex; matching is exact after listing.
    required: true
    type: list
    elements: str
"""

EXAMPLES = r"""
- name: Discover existing migrated VMs
  os_migrate.vmware_migration_kit.discover_heat_wrap:
    cloud: "{{ dst_cloud }}"
    vm_names: "{{ vms_list }}"
  register: wrap_discovered
"""

RETURN = r"""
vms_data:
    description: VM data for generate_heat_template wrap_existing
    returned: success
    type: list
    sample:
      - name: rhel-1
        instance_id: "server-uuid"
        port_ids: ["port-uuid"]
        boot_volume_id: "volume-uuid"
        flavor: "flavor-uuid"
        network: "network-uuid"
        security_groups: ["default"]
        status: ACTIVE
"""
