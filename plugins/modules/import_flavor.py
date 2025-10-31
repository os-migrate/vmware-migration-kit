#!/usr/bin/python
# -*- coding: utf-8 -*-

ANSIBLE_METADATA = {'metadata_version': '1.1',
                    'status': ['preview'],
                    'supported_by': 'community'}

DOCUMENTATION = r"""
---
module: import_flavor
short_description: Import a flavor into OpenStack from a YAML file.
version_added: "1.0.0"
description:
    - "This module reads a flavor definition from a YAML file and imports it into an OpenStack cloud."
    - "If the flavor already exists, it returns the existing flavor ID without making changes."
author: "OpenStack tenant migration tools (@os-migrate)"
options:
  cloud:
    description:
      - A dictionary containing authentication and connection parameters for the destination OpenStack cloud.
      - This should include details like C(auth_url), C(username), C(password), C(project_name), C(user_domain_name),
        C(project_domain_name), C(region_name), etc., or a C(cloud) key to use a clouds.yaml profile.
    type: dict
    required: true
  flavors_file:
    description:
      - Path to the YAML file containing the flavor definition.
    type: str
    required: true
"""

EXAMPLES = r"""
- name: Import flavors from YAML
  os_migrate.vmware_migration_kit.import_flavor:
    cloud: "{{ dst_cloud }}"
    flavors_file: "{{ os_migrate_vmw_data_dir }}/{{ vm_name }}/flavors.yml"
  register: imported_flavors
  loop: "{{ vms }}"
  loop_control:
    loop_var: vm_name
  when: create_flavor
"""

RETURN = r"""
created_flavor:
    description: Information about the flavor created or found.
    type: dict
    returned: always
    sample: {
        "name": "osm-vmware-fdb-test-4639",
        "id": "12345678-90ab-cdef-1234-567890abcdef"
    }
changed:
    description: Indicates whether a new flavor was created.
    type: bool
    returned: always
failed:
    description: Indicates if the module failed.
    type: bool
    returned: always
msg:
    description: Error message in case of failure.
    type: str
    returned: on failure
"""
