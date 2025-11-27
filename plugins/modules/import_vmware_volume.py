#!/usr/bin/python

from __future__ import absolute_import, division, print_function

__metaclass__ = type

ANSIBLE_METADATA = {
    "metadata_version": "1.1",
    "status": ["preview"],
    "supported_by": "community",
}

DOCUMENTATION = r"""
---
module: import_vmware_volume
short_description: Import VMware volume to Openstack
version_added: "2.9.0"
author: "OpenStack tenant migration tools (@os-migrate)"
description:
  - "Import VMware volume to Openstack"
options:
    vcenter_username:
        description:
            - Username to authenticate with the VMware vCenter server.
        type: str
        required: true
    vcenter_hostname:
        description:
            - Hostname or IP address of the VMware vCenter server.
        type: str
        required: true
    esxi_hostname:
        description:
            - Hostname or IP address of the ESXi host where the virtual machine is located.
        type: str
        required: true
    vddk_libdir:
        description:
            - Directory path containing the VMware VDDK library files.
            - This is required for virt-v2v to access the VMware infrastructure.
        type: str
        required: true
    vddk_thumbprint:
        description:
            - SSL thumbprint of the VMware vCenter server for verification.
            - This ensures secure communication with the vCenter server.
        type: str
        required: true
    conversion_host_id:
        description:
            - Identifier for the conversion host where the conversion will be performed.
            - This is used to track and manage the conversion process.
        type: str
        required: true
    vm_name:
        description:
            - Name of the virtual machine to be converted.
        type: str
        required: true

notes:
    - This module requires the virt-v2v tool to be installed on the target system.
    - The VMware VDDK library must be available at the specified vddk_libdir path.
    - The module expects the VirtV2V class to be properly defined elsewhere in the code.
    - No password parameter is included; it is assumed authentication is handled outside this module or via environment variables.

requirements:
    - python >= 3.6
    - virt-v2v
    - VMware VDDK
"""

EXAMPLES = r"""
# Import vmware volume to openstack
- name: Import vmware volume to openstack
  import_vmware_volume:
    vcenter_username: "administrator@vsphere.local"
    vcenter_hostname: "vcenter.example.com"
    esxi_hostname: "esxi01.example.com"
    vddk_libdir: "/opt/vmware-vddk"
    vddk_thumbprint: "01:23:45:67:89:AB:CD:EF:01:23:45:67:89:AB:CD:EF:01:23:45:67"
    conversion_host_id: "conversion-host-01"
    vm_name: "my_vm"
"""

RETURN = r"""
changed:
    description: Whether the import was performed successfully
    type: bool
    returned: always
    sample: true
cmd:
    description: The virt-v2v command that was executed
    type: str
    returned: always
    sample: >-
        /usr/bin/virt-v2v -i vmware-vddk -ic vpx://vcenter.example.com/Datacenter/esxi01.example.com -it
        vddk --vddk-libdir=/opt/vmware-vddk --vddk-thumbprint=01:23:45... -o local -on my_vm
stdout:
    description: Standard output from the virt-v2v command
    type: str
    returned: when available
    sample: "Starting conversion of VM my_vm..."
stderr:
    description: Standard error from the virt-v2v command
    type: str
    returned: when available
    sample: "Warning: non-critical issue detected..."
rc:
    description: Return code from the virt-v2v command
    type: int
    returned: always
    sample: 0
"""

from ansible.module_utils.basic import AnsibleModule
from ansible_collections.os_migrate.vmware_migration_kit.plugins.module_utils.v2v_wrapper import (
    VirtV2V,
)


def main():
    module = AnsibleModule(
        argument_spec=dict(
            vcenter_username=dict(type="str", required=True),
            vcenter_hostname=dict(type="str", required=True),
            vcenter_datacenter=dict(type="str", required=True),
            vcenter_cluster=dict(type="str", required=True),
            esxi_hostname=dict(type="str", required=True),
            cloud=dict(type="dict", required=True),
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
