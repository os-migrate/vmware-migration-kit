#!/usr/bin/env python

from __future__ import absolute_import, division, print_function
__metaclass__ = type

import subprocess


class VirtV2V:
    def __init__(self, params):
        self.params = params

    def build_command(self):
        cmd = [
            "virt-v2v",
            "-ip",
            "/tmp/passwd",
            "-ic",
            "vpx://{}@{}/{}/host/{}/{}?no_verify=1".format(
                self.params["vcenter_username"].replace("@", "%40"),
                self.params["vcenter_hostname"],
                self.params["vcenter_datacenter"],
                self.params["vcenter_cluster"],
                self.params["esxi_hostname"],
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
        ]
        cmd.append(self.params["vm_name"])

        return cmd

    def run_command(self, cmd):
        try:
            result = subprocess.run(cmd, check=True, capture_output=True, text=True)
            return dict(changed=True, stdout=result.stdout, stderr=result.stderr)
        except subprocess.CalledProcessError as e:
            return dict(
                changed=False,
                msg="Command failed:{}".format(e),
                stdout=e.stdout,
                stderr=e.stderr,
            )
