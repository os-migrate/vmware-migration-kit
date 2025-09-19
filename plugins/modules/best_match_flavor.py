#!/usr/bin/python

DOCUMENTATION = r"""
module: best_match_flavor
short_description: Returns the flavor which best matches the guest requirements
description:
  - This module connects to an OpenStack cloud using the provided C(cloud) authentication details and finds
    the flavor that best matches the VMware guest requirements based on CPU, RAM, and disk specifications.
  - It is an information-gathering module and does not make any changes to the flavor or the cloud environment.
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
  guest_info_path:
    description:
      - Path of the guest info file dumped by the VMware migration kit.
    type: str
    required: true
  disk_info_path:
    description:
      - Path of the disk info file dumped by the VMware migration kit.
    type: str
    required: true
requirements:
  - openstacksdk
"""

EXAMPLES = r"""
- name: Get best matching flavor information
  os_migrate.vmware_migration_kit.best_match_flavor:
    cloud: "{{ my_openstack_auth_details }}"
    guest_info_path: /opt/os-migrate/guest_info.json
    disk_info_path: /opt/os-migrate/disk_info.json
  register: best_flavor_result

- name: Get best matching flavor for multiple VMs using a loop
  os_migrate.vmware_migration_kit.best_match_flavor:
    cloud: "{{ my_openstack_auth_details }}"
    guest_info_path: "{{ item.guest_info_path }}"
    disk_info_path: "{{ item.disk_info_path }}"
  loop:
    - guest_info_path: "/opt/os-migrate/vm1_guest_info.json"
      disk_info_path: "/opt/os-migrate/vm1_disk_info.json"
    - guest_info_path: "/opt/os-migrate/vm2_guest_info.json"
      disk_info_path: "/opt/os-migrate/vm2_disk_info.json"
  register: multiple_best_flavor_results
"""

RETURN = r"""
changed:
  description: Indicates whether any change was made. For an info module, this is typically C(false).
  returned: always
  type: bool
  sample: false
msg:
  description: A message describing the outcome of the operation (e.g., success or error).
  returned: always
  type: str
  sample: "Best matching flavor found"
openstack_flavor_uuid:
  description:
    - UUID of the OpenStack flavor that best matches the VMware guest requirements.
    - The matching is based on CPU count, RAM size, and total disk capacity.
  returned: on success
  type: str
  sample: "a1b2c3d4-e5f6-7890-1234-567890abcdef"
"""
