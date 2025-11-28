#!/usr/bin/python
# Copyright 2025 Red Hat, Inc.
# All Rights Reserved.
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

from ansible.module_utils.basic import AnsibleModule
import sys

DOCUMENTATION = r'''
---
module: warn
short_description: Print a warning without failing
version_added: "2.1.3"
author: "Your Name"
description:
  - Print a warning message in Ansible output without failing the play.
options:
  msg:
    description: Message to display as a warning
    required: true
    type: str
'''

EXAMPLES = r'''
- name: Print a warning
  os_migrate.vmware_migration_kit.warn:
    msg: "Something important!"
'''

RETURN = r'''
changed:
  description: Module never changes state
  type: bool
  returned: always
message:
  description: The warning message printed
  type: str
  returned: always
'''


def print_warning(message):
    sys.stderr.write(f"[WARNING]: {message}\n")


def main():
    module_args = dict(
        msg=dict(type='str', required=True)
    )

    module = AnsibleModule(
        argument_spec=module_args,
        supports_check_mode=True
    )

    msg = module.params["msg"]

    # Print actual warning on stderr
    print_warning(msg)
    module.exit_json(changed=False, message=msg)


if __name__ == '__main__':
    main()