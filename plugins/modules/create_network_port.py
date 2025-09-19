#!/usr/bin/python

DOCUMENTATION = r"""
module: create_network_port
short_description: Create network ports for a VM
description:
  - This module connects to an OpenStack cloud using the provided C(cloud) authentication details and creates
    network ports for a VM based on the network interface information from VMware.
  - It creates ports with the appropriate MAC addresses and security groups.
author: "OpenStack tenant migration tools (@os-migrate)"
version_added: "1.0.0"
options:
  cloud:
    description:
      - A dictionary containing authentication and connection parameters for the destination OpenStack cloud.
      - This should include details like C(auth_url), C(username), C(password), C(project_name), C(user_domain_name),
        C(project_domain_name), C(region_name), etc., or a C(cloud) key to use a clouds.yaml profile.
    type: dict
    required: true
  os_migrate_nics_file_path:
    description:
      - Path of the nics file dumped by the VMware migration kit.
      - It could be the json macs_{{vm_name}}.json file or the nics_{{vm_name}}.json file.
    type: str
    required: true
  vm_name:
    description:
      - Name of the VM for which the nics file was dumped.
    type: str
    required: true
  used_mapped_networks:
    description:
      - Whether the nics file contains mapped networks or not.
    type: bool
    default: true
    required: false
  security_groups:
    description:
      - List of security groups to be attached to the ports.
    type: list
    default: ['default']
    elements: str
    required: false
  network_name:
    description:
      - Name of the network to which the ports should be attached.
    type: str
    required: false
requirements:
  - openstacksdk
"""

EXAMPLES = r"""
- name: Create network ports for VM
  os_migrate.vmware_migration_kit.create_network_port:
    cloud: "{{ my_openstack_auth_details }}"
    os_migrate_nics_file_path: "/opt/os-migrate/nics_cirros.json"
    vm_name: "cirros"
    used_mapped_networks: true
    security_groups: ["default"]

- name: Create network port with specific network
  os_migrate.vmware_migration_kit.create_network_port:
    cloud: "{{ my_openstack_auth_details }}"
    os_migrate_nics_file_path: "/opt/os-migrate/macs_cirros.json"
    vm_name: "cirros"
    used_mapped_networks: false
    security_groups: ["default"]
    network_name: "private"
"""

RETURN = r"""
changed:
  description: Indicates whether any change was made.
  returned: always
  type: bool
  sample: true
msg:
  description: A message describing the outcome of the operation.
  returned: always
  type: str
  sample: "Network ports created successfully"
ports:
  description: List of created ports with their IDs.
  returned: success
  type: list
  elements: dict
  contains:
    port-id:
      description: The UUID of the created port.
      type: str
      sample: "a1b2c3d4-e5f6-7890-1234-567890abcdef"
  sample: [{"port-id": "a1b2c3d4-e5f6-7890-1234-567890abcdef"}]
"""
