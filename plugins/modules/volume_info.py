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
  - "Get volume information from OpenStack by name."
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
    required: true
    type: str
"""

EXAMPLES = r"""
- name: Get volume information
  os_migrate.vmware_migration_kit.volume_info:
    cloud: source_cloud
    name: "my-volume"
  register: volume_info
"""

RETURN = r"""
volumes:
    description: List of volume information
    returned: success
    type: list
    sample: [{"id": "uuid", "name": "volume-name", "status": "available"}]
"""
