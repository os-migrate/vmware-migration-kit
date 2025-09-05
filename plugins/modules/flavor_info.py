#!/usr/bin/python

DOCUMENTATION = r"""
module: flavor_info
short_description: Retrieves details for a specified OpenStack flavor.
description:
  - This module connects to an OpenStack cloud using the provided C(dst_cloud) authentication details and fetches
    detailed information about a specific flavor, identified either by C(flavor_id) or C(flavor_name).
  - It is an information-gathering module and does not make any changes to the flavor or the cloud environment.
author: "OpenStack tenant migration tools (@os-migrate)"
version_added: "1.0.0"
options:
  dst_cloud:
    description:
      - A dictionary containing authentication and connection parameters for the destination OpenStack cloud.
      - This should include details like C(auth_url), C(username), C(password), C(project_name), C(user_domain_name),
        C(project_domain_name), C(region_name), etc., or a C(cloud) key to use a clouds.yaml profile.
    type: dict
    required: true
  flavor_id:
    description:
      - The UUID of the OpenStack flavor to retrieve.
    type: str
    required: false
  flavor_name:
    description:
      - The name of the OpenStack flavor to retrieve.
    type: str
    required: false
    notes:
      - Either C(flavor_id) or C(flavor_name) must be provided.
requirements:
  - openstacksdk
"""

EXAMPLES = r"""
- name: Get flavor information by flavor name
  os_migrate.vmware_migration_kit.flavor_info:
    dst_cloud: "{{ my_openstack_auth_details }}"
    flavor_name: "osm-vmware-haproxy-user1"
  register: flavor_info_result

- name: Get flavor information by flavor ID
  os_migrate.vmware_migration_kit.flavor_info:
    dst_cloud: "{{ my_openstack_auth_details }}"
    flavor_id: "a2b529d8-4505-480d-a620-35d3624c11c6"
  register: flavor_info_result

- name: Get flavor information for multiple flavors using a loop
  os_migrate.vmware_migration_kit.flavor_info:
    dst_cloud: "{{ my_openstack_auth_details }}"
    flavor_name: "{{ item }}"
  loop:
    - "osm-vmware-haproxy-user1"
    - "osm-vmware-db-user1"
  register: multiple_flavor_info_results

- name: Example from os-migrate role (adapted)
  vars:
    dst_cloud_details:
      auth_url: "http://keystone.example.com:5000/v3"
      username: "admin_user"
      password: "secret_password"
      project_name: "admin_project"
      user_domain_name: "Default"
      project_domain_name: "Default"
      region_name: "RegionOne"
    flavor_list:
      - "osm-vmware-haproxy-user1"
      - "osm-vmware-db-user1"
  tasks:
    - name: Get flavor information using loop
      os_migrate.vmware_migration_kit.flavor_info:
        dst_cloud: "{{ dst_cloud_details }}"
        flavor_name: "{{ flavor_loop_var }}"
      loop: "{{ flavor_list }}"
      loop_control:
        loop_var: flavor_loop_var
      register: flavor_info_output
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
  sample: "Flavor osm-vmware-haproxy-user1 retrieved successfully."
flavor:
  description:
    - A dictionary containing the details of the specified flavor.
    - This includes properties like C(id), C(name), C(vcpus), C(ram), C(disk), C(swift_disk), C(ephemeral), C(is_public), etc.
  returned: on success
  type: dict
  sample:
    id: "a2b529d8-4505-480d-a620-35d3624c11c6"
    name: "osm-vmware-haproxy-user1"
    ram: 2048
    vcpus: 2
    disk: 20
    swap: 0
    ephemeral: 0
    is_public: true
    extra_specs:
      hw:cpu_policy: "shared"
      hw:mem_page_size: "large"
"""
