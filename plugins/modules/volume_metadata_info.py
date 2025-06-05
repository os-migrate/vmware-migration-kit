#!/usr/bin/python

DOCUMENTATION = r"""
module: volume_metadata_info
short_description: Retrieves metadata for a specified OpenStack Cinder volume.
description:
  - This module connects to an OpenStack cloud using the provided C(dst_cloud) authentication details and fetches
    the metadata associated with a specific Cinder volume, identified by C(volume_id).
  - It is an information-gathering module and does not make any changes to the volume or the cloud environment.
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
  volume_id:
    description:
      - The UUID of the OpenStack Cinder volume for which to retrieve metadata.
    type: str
    required: true
requirements:
  - openstacksdk
"""

EXAMPLES = r"""
- name: Get volume metadata info for a single volume
  os_migrate.vmware_migration_kit.volume_metadata_info:
    dst_cloud: "{{ my_openstack_auth_details }}"
    volume_id: "a1b2c3d4-e5f6-7890-1234-567890abcdef"
  register: single_volume_metadata_result

- name: Get volume metadata info for multiple volumes using a loop
  os_migrate.vmware_migration_kit.volume_metadata_info:
    dst_cloud: "{{ my_openstack_auth_details }}"
    volume_id: "{{ item }}"
  loop:
    - "a1b2c3d4-e5f6-7890-1234-567890abcdef"
    - "b2c3d4e5-f6a7-8901-2345-67890abcdef12"
  register: multiple_volume_metadata_results

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
    volume_uuid_list:
      - "uuid1-from-previous-task"
      - "uuid2-from-previous-task"
  tasks:
    - name: Get volume metadata info using loop (as per role example)
      os_migrate.vmware_migration_kit.volume_metadata_info:
        dst_cloud: "{{ dst_cloud_details }}"
        volume_id: "{{ uuid_loop_var }}"
      loop: "{{ volume_uuid_list }}"
      loop_control:
        loop_var: uuid_loop_var
      register: volume_info_metadata_output
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
  sample: "Metadata for volume a1b2c3d4-e5f6-7890-1234-567890abcdef retrieved successfully."
metadata:
  description:
    - A dictionary containing the key-value pairs of metadata associated with the specified Cinder volume.
    - If the volume has no metadata, this will be an empty dictionary.
  returned: on success
  type: dict
  sample:
    os_distro: "ubuntu"
    image_source_id: "c1d2e3f4-a5b6-7890-fedc-ba9876543210"
    custom_tag: "webserver_data_disk"
"""
