#!/usr/bin/python

from __future__ import (absolute_import, division, print_function)
__metaclass__ = type

ANSIBLE_METADATA = {
    'metadata_version': '1.1',
    'status': ['preview'],
    'supported_by': 'community'
}

DOCUMENTATION = '''
---
module: best_match_flavor

short_description: Returns the flavor which best matches the guest requirements

extends_documentation_fragment: openstack

version_added: "2.9.0"

author: "OpenStack tenant migration tools (@os-migrate)"

description:
  - "Returns the flavor uuid which best matches the VMware guest requirements."

options:
  cloud:
    description:
      - Cloud from clouds.yaml to use.
      - Required if 'auth' parameter is not used.
    required: false
    type: raw
  auth:
    description:
      - Required if 'cloud' param not used.
    required: false
    type: dict
  auth_type:
    description:
      - Auth type plugin for OpenStack. Can be omitted if using password authentication.
    required: false
    type: str
  region_name:
    description:
      - OpenStack region name. Can be omitted if using default region.
    required: false
    type: str
  guest_info_path:
    description:
      Path of the guest info file dumped by the VMware migration kit.
    required: true
    type: str
  disk_info_path:
    description:
      Path of the disk info file dumped by the VMware migration kit.
    required: true
    type: str
'''

EXAMPLES = '''
- name: Find the best matching flavor
  os_migrate.vmware_migration_kit.best_match_flavor:
    cloud: source_cloud
    guest_info_path: /opt/os-migrate/guest_info.json
    disk_info_path: /opt/os-migrate/disk_info.json
  register: best_flavor
'''

RETURN = '''
{ "openstack_flavor_uuid": "xyz" }
'''

from ansible.module_utils.basic import AnsibleModule
# Import openstack module utils from ansible_collections.openstack.cloud.plugins as per ansible 3+
try:
    from ansible_collections.openstack.cloud.plugins.module_utils.openstack \
        import openstack_full_argument_spec, openstack_cloud_from_module
except ImportError:
    # If this fails fall back to ansible < 3 imports
    from ansible.module_utils.openstack \
        import openstack_full_argument_spec, openstack_cloud_from_module

from ansible_collections.os_migrate.os_migrate.plugins.module_utils import filesystem
from ansible_collections.os_migrate.os_migrate.plugins.module_utils import flavor
import json

def get_total_disk_capacity(disk_info):
    total_capacity_kb = sum([disk_info['guest_disk_info'][disk]["capacity_in_kb"]
                             for disk in disk_info['guest_disk_info'].keys()]) / 1024
    return total_capacity_kb

def flavor_distance(flavor, guest_info, disk_capacity_mb):
    vcpu_diff = abs(flavor.vcpus - guest_info['instance']['hw_processor_count'])
    ram_diff = abs(flavor.ram - guest_info['instance']['hw_memtotal_mb'])
    disk_diff = abs(flavor.disk - disk_capacity_mb)
    return vcpu_diff + ram_diff + disk_diff

def run_module():
    argument_spec = openstack_full_argument_spec(
        guest_info_path=dict(type='str', required=True),
        disk_info_path=dict(type='str', required=True),
    )
    result = dict(
        changed=False,
        openstack_flavor_uuid=None,
    )

    module = AnsibleModule(
        argument_spec=argument_spec,
)
    # Open json files
    with open(module.params['guest_info_path'], 'r') as guest_file:
        guest_info = json.load(guest_file)

    with open(module.params['disk_info_path'], 'r') as disk_file:
        disk_info = json.load(disk_file)

    # Get the flavor
    sdk, conn = openstack_cloud_from_module(module)
    flavors = conn.compute.flavors()

    # Calculate the total disk capacity
    disk_capacity_mb = get_total_disk_capacity(disk_info)
    # Find the best matching flavor
    best_flavor = min(flavors,
                      key=lambda flavor: flavor_distance(flavor, guest_info, disk_capacity_mb))
    result['openstack_flavor_uuid'] = best_flavor.id
    
    module.exit_json(**result)


def main():
    run_module()


if __name__ == '__main__':
    main()
