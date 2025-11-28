#!/usr/bin/python
# GNU General Public License v3.0+
# (c) 2025, Your Name <you@example.com>
# SPDX-License-Identifier: GPL-3.0-or-later

# This file is part of Ansible
#
# Ansible is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.
#
# Ansible is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with Ansible.  If not, see <https://www.gnu.org/licenses/>.

DOCUMENTATION = r'''
---
module: warn
short_description: Print a warning without failing
version_added: "2.0.0"
author: "Mathieu Bultel (@matbu)"
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

from ansible.module_utils.basic import AnsibleModule
import sys


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