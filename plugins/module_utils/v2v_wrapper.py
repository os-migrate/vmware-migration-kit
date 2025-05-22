#!/usr/bin/env python

from __future__ import absolute_import, division, print_function

__metaclass__ = type

import subprocess


class VirtV2V:
    def __init__(self, params):
        self.params = params

    def build_command(self):
        return [
            "virt-v2v",
            "-ip",
            "/tmp/passwd",
            "-ic",
            "esx://{vcenter_username}@{vcenter_hostname}/Datacenter/{esxi_hostname}?no_verify=1".format(
                vcenter_username=self.params["vcenter_username"],
                vcenter_hostname=self.params["vcenter_hostname"],
                esxi_hostname=self.params["esxi_hostname"],
            ),
            "-it",
            "vddk",
            "-io",
            "vddk-libdir={}".format(self.params["vddk_libdir"]),
            "-io",
            "vddk-thumbprint={}".format(self.params["vddk_thumbprint"]),
            "-o",
            "openstack",
            "-oo",
            "server-id={}".format(self.params["conversion_host_id"]),
            self.params["vm_name"],
        ]

    def run_command(self, cmd):
        try:
            result = subprocess.run(cmd, check=True, capture_output=True, text=True)
            return dict(changed=True, stdout=result.stdout, stderr=result.stderr)
        except subprocess.CalledProcessError as e:
            return dict(
                changed=False,
                msg=f"Command failed: {e}",
                stdout=e.stdout,
                stderr=e.stderr,
            )
