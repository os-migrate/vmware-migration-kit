#!/usr/bin/python


from __future__ import absolute_import, division, print_function

__metaclass__ = type

from ansible.module_utils.basic import AnsibleModule
from ansible_collections.os_migrate.vmware_migration_kit.plugins.module_utils.v2v_wrapper import (
    VirtV2V,
)


ANSIBLE_METADATA = {
    "metadata_version": "1.1",
    "status": ["preview"],
    "supported_by": "community",
}

DOCUMENTATION = """
---
module: import_vmware_volume
short_description: Import VMware volume to Openstack
extends_documentation_fragment: openstack.cloud.openstack
version_added: "2.9.0"
author: "OpenStack tenant migration tools (@os-migrate)"
description:
  - "Import VMware volume to Openstack"
# TODO: add examples and options
options:
  path:
    description:
      - Resources YAML file to where network will be serialized.
      - In case the resource file already exists, it must match the
        os-migrate version.
      - In case the resource of same type and name exists in the file,
        it will be replaced.
    required: true
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
  flavor_name:
    description:
      - Name of the falvor.
    required: true
    type: str
"""

EXAMPLES = """
"""

RETURN = """
"""


def main():
    module = AnsibleModule(
        argument_spec=dict(
            vcenter_username=dict(type="str", required=True),
            vcenter_hostname=dict(type="str", required=True),
            esxi_hostname=dict(type="str", required=True),
            vddk_libdir=dict(type="str", required=True),
            vddk_thumbprint=dict(type="str", required=True),
            conversion_host_id=dict(type="str", required=True),
            vm_name=dict(type="str", required=True),
        )
    )

    v2v = VirtV2V(module.params)
    cmd = v2v.build_command()
    result = v2v.run_command(cmd)

    if result["changed"]:
        module.exit_json(**result)
    else:
        module.fail_json(**result)


if __name__ == "__main__":
    main()
