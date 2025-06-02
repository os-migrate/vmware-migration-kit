#!/usr/bin/python

from __future__ import absolute_import, division, print_function

__metaclass__ = type

ANSIBLE_METADATA = {
    "metadata_version": "1.1",
    "status": ["preview"],
    "supported_by": "community",
}

DOCUMENTATION = """
---
module: best_match_flavor
short_description: Returns the flavor which best matches the guest requirements
version_added: "2.9.0"
author: "OpenStack tenant migration tools (@os-migrate)"
description:
  - "Returns the flavor uuid which best matches the VMware guest requirements."
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
      - Dictionary containing authentication information.
      - Can include auth_url, username, password, project_name, domain_name, etc.
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
  interface:
    description:
      - Endpoint URL type to fetch from the service catalog.
    choices: ['public', 'internal', 'admin']
    default: public
    type: str
    aliases: ['endpoint_type']
  validate_certs:
    description:
      - Whether or not SSL API requests should be verified.
    type: bool
    aliases: ['verify']
  ca_cert:
    description:
      - A path to a CA Cert bundle that can be used as part of verifying SSL API requests.
    type: str
    aliases: ['cacert']
  client_cert:
    description:
      - A path to a client certificate to use as part of the SSL transaction.
    type: str
    aliases: ['cert']
  client_key:
    description:
      - A path to a client key to use as part of the SSL transaction.
    type: str
    aliases: ['key']
  timeout:
    description:
      - How long should ansible wait for the requested resource.
    type: int
    default: 180
  api_timeout:
    description:
      - How long should the socket layer wait before timing out for API calls.
    type: int
  wait:
    description:
      - Should ansible wait until the requested resource is complete.
    type: bool
    default: true
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
requirements:
  - "python >= 3.6"
  - "openstacksdk >= 1.0.0"
"""

EXAMPLES = """
- name: Find the best matching flavor
  os_migrate.vmware_migration_kit.best_match_flavor:
    cloud: source_cloud
    guest_info_path: /opt/os-migrate/guest_info.json
    disk_info_path: /opt/os-migrate/disk_info.json
  register: best_flavor
"""

RETURN = """
openstack_flavor_uuid:
    description: uuid of the openstack flavor
    returned: success
    type: str
    sample: xyz
"""
import traceback
from ansible.module_utils.basic import AnsibleModule, missing_required_lib

from ansible_collections.openstack.cloud.plugins.module_utils.openstack import (
    openstack_full_argument_spec,
    openstack_cloud_from_module,
)

try:
    import json
except ImportError:
    HAS_JSON = False
    JSON_IMPORT_ERROR = traceback.format_exc()
else:
    HAS_JSON = True
    JSON_IMPORT_ERROR = None


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


def flavor_distance(flavor, guest_info, disk_capacity_mb):
    vcpu_diff = abs(flavor.vcpus - guest_info["instance"]["hw_processor_count"])
    ram_diff = abs(flavor.ram - guest_info["instance"]["hw_memtotal_mb"])
    disk_diff = abs(flavor.disk - disk_capacity_mb)
    return vcpu_diff + ram_diff + disk_diff


def run_module():
    argument_spec = openstack_full_argument_spec(
        guest_info_path=dict(type="str", required=True),
        disk_info_path=dict(type="str", required=True),
    )
    result = dict(
        changed=False,
        openstack_flavor_uuid=None,
    )

    module = AnsibleModule(
        argument_spec=argument_spec,
    )

    if not HAS_JSON:
        module.fail_json(msg=missing_required_lib("json"), exception=JSON_IMPORT_ERROR)

    # Open json files
    with open(module.params["guest_info_path"]) as guest_file:
        guest_info = json.load(guest_file)

    with open(module.params["disk_info_path"]) as disk_file:
        disk_info = json.load(disk_file)

    # Get the flavor
    sdk, conn = openstack_cloud_from_module(module)
    flavors = conn.compute.flavors()

    # Calculate the total disk capacity
    disk_capacity_mb = get_total_disk_capacity(disk_info)
    # Find the best matching flavor
    best_flavor = min(
        flavors,
        key=lambda flavor: flavor_distance(flavor, guest_info, disk_capacity_mb),
    )
    result["openstack_flavor_uuid"] = best_flavor.id

    module.exit_json(**result)


def main():
    run_module()


if __name__ == "__main__":
    main()
