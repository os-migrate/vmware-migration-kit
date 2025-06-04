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
module: export_flavor
short_description: Export VMware Guest/Vm definition into an OS-Migrate YAML import_flavor format
version_added: "2.9.0"
author: "OpenStack tenant migration tools (@os-migrate)"
description:
  - "Export Vmware Flavor definition into an OS-Migrate YAML format"
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

EXAMPLES = r"""
- name: Export myflavor into /opt/os-migrate/flavors.yml
  os_migrate.os_migrate.export_flavor:
    path: /opt/os-migrate/flavors.yml
    name: my_guest
    guest_info_path: /opt/os-migrate/guest_info.json
    disk_info_path: /opt/os-migrate/disk_info.json
"""

RETURN = r"""
"""

import traceback
from ansible.module_utils.basic import AnsibleModule, missing_required_lib

try:
    import json
except ImportError:
    HAS_JSON = False
    JSON_IMPORT_ERROR = traceback.format_exc()
else:
    HAS_JSON = True
    JSON_IMPORT_ERROR = None

try:
    import yaml
except ImportError:
    HAS_YAML = False
    YAML_IMPORT_ERROR = traceback.format_exc()
else:
    HAS_YAML = True
    YAML_IMPORT_ERROR = None


def get_total_disk_capacity(disk_info):
    total_capacity_kb = (
        sum(
            [
                disk_info["guest_disk_info"][disk]["capacity_in_kb"]
                for disk in disk_info["guest_disk_info"].keys()
            ]
        )
        / 1024
    )
    return total_capacity_kb


def run_module():
    module_args = dict(
        path=dict(type="str", required=True),
        guest_info_path=dict(type="str", required=True),
        disk_info_path=dict(type="str", required=True),
        flavor_name=dict(type="str", required=True),
    )

    result = dict(
        # This module doesn't change anything.
        changed=False,
    )

    module = AnsibleModule(
        argument_spec=module_args,
        # Module doesn't change anything, we can let it run as-is in
        # check mode.
        supports_check_mode=True,
    )

    if not HAS_YAML:
        module.fail_json(msg=missing_required_lib("yaml"), exception=YAML_IMPORT_ERROR)

    if not HAS_JSON:
        module.fail_json(msg=missing_required_lib("json"), exception=JSON_IMPORT_ERROR)

    flavor_name = module.params["flavor_name"]
    # Open guest_info_path file
    with open(module.params["guest_info_path"]) as guest_file:
        guest_info = json.load(guest_file)

    with open(module.params["disk_info_path"]) as disk_file:
        disk_info = json.load(disk_file)

    total_disk_capacity_gb = get_total_disk_capacity(disk_info) / 1024
    vcpu = guest_info["instance"]["hw_processor_count"]
    ram = guest_info["instance"]["hw_memtotal_mb"]

    # Dump flavor data structure filled with guest_info and disk_info
    data = {
        "os_migrate_version": "1.0.1",
        "resources": [
            {
                "_info": {"id": None, "is_disabled": False},
                "_migration_params": {},
                "params": {
                    "description": None,
                    "disk": int(total_disk_capacity_gb),
                    "ephemeral": 0,
                    "extra_specs": {},
                    "is_public": True,
                    "name": flavor_name,
                    "ram": ram,
                    "rxtx_factor": 1.0,
                    "swap": 0,
                    "vcpus": vcpu,
                },
                "type": "openstack.compute.Flavor",
            }
        ],
    }
    # Dump into file:
    with open(module.params["path"], "w") as yaml_file:
        yaml.dump(data, yaml_file, default_flow_style=False)
    module.exit_json(**result)


def main():
    run_module()


if __name__ == "__main__":
    main()
