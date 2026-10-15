#!/usr/bin/python

DOCUMENTATION = r"""
---
module: volume_info
short_description: Get volume information from OpenStack
extends_documentation_fragment:
    - os_migrate.vmware_migration_kit.openstack
version_added: "2.9.0"
author: "OpenStack tenant migration tools (@os-migrate)"
description:
  - "Get volume information from OpenStack by name or by UUID."
  - "Either O(name) or O(id) must be provided. When O(id) is set it takes precedence."
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
  name:
    description:
      - Name of the volume to get information for.
      - Mutually exclusive with O(id); one of O(name) or O(id) is required.
    required: false
    type: str
  id:
    description:
      - UUID of the volume to get information for.
      - Useful when the volume name is not known but its UUID is.
      - Mutually exclusive with O(name); one of O(name) or O(id) is required.
    required: false
    type: str
"""

EXAMPLES = r"""
- name: Get volume information by name
  os_migrate.vmware_migration_kit.volume_info:
    cloud: source_cloud
    name: "my-volume"
  register: volume_info

- name: Get volume information by UUID
  os_migrate.vmware_migration_kit.volume_info:
    cloud: source_cloud
    id: "11111111-2222-3333-4444-555555555555"
  register: volume_info
"""

RETURN = r"""
volumes:
    description: List of volume information
    returned: success
    type: list
    sample: [{"id": "uuid", "name": "volume-name", "status": "available"}]
"""
