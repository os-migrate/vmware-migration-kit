#!/usr/bin/python

DOCUMENTATION = r"""
---
module: delete_volume
short_description: Delete OpenStack volume
extends_documentation_fragment:
    - os_migrate.vmware_migration_kit.openstack
version_added: "2.9.0"
author: "OpenStack tenant migration tools (@os-migrate)"
description:
  - "Delete an OpenStack volume by name or ID."
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
      - Name or ID of the volume to delete.
    required: true
    type: str
"""

EXAMPLES = r"""
- name: Delete volume
  os_migrate.vmware_migration_kit.delete_volume:
    cloud: source_cloud
    name: "my-volume"
"""

RETURN = r"""
msg:
    description: Success message
    returned: success
    type: str
    sample: "Volume deleted successfully"
"""
