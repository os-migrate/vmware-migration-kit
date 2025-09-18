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
module: best_match_flavor
short_description: Returns the flavor which best matches the guest requirements
version_added: "2.9.0"
author: "OpenStack tenant migration tools (@os-migrate)"
description:
  - "Returns the flavor uuid which best matches the VMware guest requirements."
options:
  cloud:
    description:
      - A dictionary containing authentication and connection parameters for the destination OpenStack cloud.
      - This should include details like C(auth_url), C(username), C(password), C(project_name), C(user_domain_name),
        C(project_domain_name), C(region_name), etc., or a C(cloud) key to use a clouds.yaml profile.
    type: dict
    required: true
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
"""

EXAMPLES = r"""
- name: Find the best matching flavor
  os_migrate.vmware_migration_kit.best_match_flavor:
    cloud: source_cloud
    guest_info_path: /opt/os-migrate/guest_info.json
    disk_info_path: /opt/os-migrate/disk_info.json
  register: best_flavor
"""

RETURN = r"""
openstack_flavor_uuid:
    description: uuid of the openstack flavor
    returned: success
    type: str
    sample: xyz
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


def run_module():
    module_args = dict(
        cloud=dict(type="dict", required=True),
        guest_info_path=dict(type="str", required=True),
        disk_info_path=dict(type="str", required=True),
    )

    result = dict(
        changed=False,
        openstack_flavor_uuid=None,
    )

    module = AnsibleModule(
        argument_spec=module_args,
        supports_check_mode=True,
    )

    if not HAS_JSON:
        module.fail_json(msg=missing_required_lib("json"), exception=JSON_IMPORT_ERROR)

    try:
        # Get the path to the Go executable
        current_dir = os.path.dirname(os.path.abspath(__file__))
        go_executable = os.path.join(current_dir, "best_match_flavor")

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
                "guest_info_path": module.params["guest_info_path"],
                "disk_info_path": module.params["disk_info_path"],
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
            result["openstack_flavor_uuid"] = go_output.get("openstack_flavor_uuid")

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


def main():
    run_module()


if __name__ == "__main__":
    main()
