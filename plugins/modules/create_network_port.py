#!/usr/bin/python

# Copyright (c) 2025 Red Hat, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

from __future__ import absolute_import, division, print_function

__metaclass__ = type

ANSIBLE_METADATA = {
    "metadata_version": "1.1",
    "status": ["preview"],
    "supported_by": "community",
}

DOCUMENTATION = r"""
---
module: create_network_port
short_description: Create network ports for a VM
version_added: "2.9.0"
author: "OpenStack tenant migration tools (@os-migrate)"
description:
  - "Create network ports for a VM based on the nics file dumped by the VMware migration kit."
options:
  cloud:
    description:
      - A dictionary containing authentication and connection parameters for the destination OpenStack cloud.
      - This should include details like C(auth_url), C(username), C(password), C(project_name), C(user_domain_name),
        C(project_domain_name), C(region_name), etc., or a C(cloud) key to use a clouds.yaml profile.
    type: dict
    required: true
  os_migrate_nics_file_path:
    description:
      Path of the nics file dumped by the VMware migration kit.
      It could the json macs_{{vm_name}}.json file or the nics_{{vm_name}}.json file.
    required: true
    type: str
  vm_name:
    description:
      Name of the VM for which the nics file was dumped.
    required: true
    type: str
  used_mapped_networks:
    description:
        Whether the nics file contains mapped networks or not.
    required: false
    type: bool
    default: true
  security_groups:
    description:
        List of security groups to be attached to the ports.
    required: false
    type: list
    default: ['default']
    elements: str
  network_name:
    description:
        Name of the network to which the ports should be attached.
    required: false
    type: str
"""

EXAMPLES = r"""
---
- name: Create network ports for VM
  hosts: localhost
  tasks:
    - name: Create network ports
      os_migrate.vmware_migration_kit.create_network_port:
        cloud: dst
        os_migrate_nics_file_path: "/opt/os-migrate/nics_cirros.json"
        vm_name: "cirros"
        used_mapped_networks: true
        security_groups: ["default"]
      register: ports_uuid

    - name: Create network port
      os_migrate.vmware_migration_kit.create_network_port:
        cloud: dst
        os_migrate_nics_file_path: "/opt/os-migrate/macs_cirros.json"
        vm_name: "cirros"
        used_mapped_networks: false
        security_groups: ["default"]
        network_name: "private"
      register: ports_uuid
"""

RETURN = r"""
ports:
    description: list of created ports
    returned: success
    type: list
    sample: [{"port-id":"uuid"}]
"""

import json
import os
import subprocess
import tempfile
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


def main():
    module_args = dict(
        cloud=dict(type="dict", required=True),
        os_migrate_nics_file_path=dict(type="str", required=True),
        vm_name=dict(type="str", required=True),
        used_mapped_networks=dict(type="bool", default=True),
        security_groups=dict(type="list", default=["default"], elements="str"),
        network_name=dict(type="str", required=False),
    )

    result = dict(changed=False, ports=[])

    module = AnsibleModule(argument_spec=module_args, supports_check_mode=True)

    if not HAS_JSON:
        module.fail_json(msg=missing_required_lib("json"), exception=JSON_IMPORT_ERROR)

    try:
        # Get the path to the Go executable
        current_dir = os.path.dirname(os.path.abspath(__file__))
        go_executable = os.path.join(current_dir, "create_network_port")

        # Check if the Go executable exists
        if not os.path.exists(go_executable):
            module.fail_json(msg=f"Go executable not found at {go_executable}")

        # Check if the executable is actually executable
        if not os.access(go_executable, os.X_OK):
            module.fail_json(msg=f"Go executable at {go_executable} is not executable")

        # Create a temporary file for the arguments
        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".json", delete=False
        ) as args_file:
            args_data = {
                "cloud": module.params["cloud"],
                "os_migrate_nics_file_path": module.params["os_migrate_nics_file_path"],
                "vm_name": module.params["vm_name"],
                "used_mapped_networks": module.params["used_mapped_networks"],
                "security_groups": module.params["security_groups"],
                "network_name": module.params["network_name"],
            }
            json.dump(args_data, args_file)
            args_file_path = args_file.name

        try:
            # Run the Go executable
            result_process = subprocess.run(
                [go_executable, args_file_path],
                capture_output=True,
                text=True,
                check=True,
            )

            # Parse the JSON output from the Go executable
            go_output = json.loads(result_process.stdout)

            # Map the Go output to Ansible result format
            result["changed"] = go_output.get("changed", False)
            result["ports"] = go_output.get("ports", [])

            # Check if the Go executable reported a failure
            if go_output.get("failed", False):
                module.fail_json(
                    msg=go_output.get("msg", "Unknown error from Go executable")
                )

        except subprocess.CalledProcessError as e:
            module.fail_json(
                msg=f"Go executable failed with return code {e.returncode}: {e.stderr}"
            )
        except json.JSONDecodeError as e:
            module.fail_json(msg=f"Failed to parse JSON output from Go executable: {e}")
        finally:
            # Clean up the temporary file
            try:
                os.unlink(args_file_path)
            except OSError:
                pass  # Ignore cleanup errors

    except Exception as e:
        module.fail_json(
            msg=f"Unexpected error: {str(e)}", exception=traceback.format_exc()
        )

    module.exit_json(**result)


if __name__ == "__main__":
    main()
