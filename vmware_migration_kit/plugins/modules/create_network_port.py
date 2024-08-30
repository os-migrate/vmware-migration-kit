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
module: create_network_port

short_description: Create network ports for a VM

extends_documentation_fragment: openstack

version_added: "2.9.0"

author: "OpenStack tenant migration tools (@os-migrate)"

description:
  - "Create network ports for a VM based on the nics file dumped by the VMware migration kit."

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
  os_migrate_nics_file_path:
    description:
      Path of the nics file dumped by the VMware migration kit.
      It could the json macs_{{vm_name}}.json file or the nics_{{vm_name}}.json file.
    required: true
    type: str
  vm_name:
    description:
      Name of the VM for which the nics file was dumped.
    required: true
    type: str
  used_mapped_networks:
    description:
        Whether the nics file contains mapped networks or not.
    required: false
    type: bool
    default: true
  security_groups:
    description:
        List of security groups to be attached to the ports.
    required: false
    type: list
    default: ['default']
  network_name:
    description:
        Name of the network to which the ports should be attached.
    required: false
    type: str
'''

EXAMPLES = '''
---
- name: Create network ports for VM
  hosts: localhost
  tasks:
    - name: Create network ports
      os_migrate.vmware_migration_kit.create_network_port:
        cloud: dst
        os_migrate_nics_file_path: "/opt/os-migrate/nics_cirros.json"
        vm_name: "cirros"
        used_mapped_networks: true
        security_groups: ["default"]
      register: ports_uuid
 
    - name: Create network port
      os_migrate.vmware_migration_kit.create_network_port:
        cloud: dst
        os_migrate_nics_file_path: "/opt/os-migrate/macs_cirros.json"
        vm_name: "cirros"
        used_mapped_networks: false
        security_groups: ["default"]
        network_name: "private"
      register: ports_uuid
'''

RETURN = '''
{ "ports": [{"port-id":"uuid"}] }
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
import json
import re

def main():
    argument_spec = openstack_full_argument_spec(
        os_migrate_nics_file_path=dict(type='str', required=True),
        vm_name=dict(type='str', required=True),
        used_mapped_networks=dict(type='bool', default=True),
        security_groups=dict(type='list', default=['default']),
        network_name=dict(type='str', required=False),
    )

    result = dict(
        changed = False,
        ports = []
    )

    module = AnsibleModule(
        argument_spec=argument_spec,
        supports_check_mode=True
    )

    os_migrate_nics_file_path = module.params['os_migrate_nics_file_path']
    vm_name = module.params['vm_name']
    used_mapped_networks = module.params['used_mapped_networks']
    security_groups = module.params['security_groups']
    network_name = module.params['network_name']

    # Open Openstack connection
    sdk, conn = openstack_cloud_from_module(module)

    # Load the data file
    try:
        with open(os_migrate_nics_file_path, 'r') as f:
            vm_nics = json.load(f)
    except Exception as e:
        module.fail_json(msg=f"Failed to load network data file: {str(e)}")

    # If not mapped networks, use the network name provided
    if not used_mapped_networks:
        for nic in vm_nics:
            nic['vlan'] = network_name

    try:
        port_uuid = []
        for nic_index, item in enumerate(vm_nics):
            # Get network id
            network_object = conn.get_network(item['vlan'])
            if network_object:
                network_id = network_object['id']
            port_name = f"{vm_name}-NIC-{nic_index}-VLAN-{item['vlan']}"
            port = conn.network.create_port(
                name=port_name,
                network_id=network_id,
                mac_address=item['mac'],
                allowed_address_pairs=[{'ip_address': '0.0.0.0/0', 'mac_address': item['mac']}],
                security_groups=security_groups
            )
            port_uuid.append({'port-id': port['id']})
            result['changed'] = True
        result["ports"] = port_uuid
    except Exception as e:
        module.fail_json(msg=f"Failed to create  ports: {str(e)}")

    module.exit_json(**result)

if __name__ == '__main__':
    main()