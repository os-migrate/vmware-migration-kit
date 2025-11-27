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
        cmd.extend(self._convert_cloud_to_virtv2v_options())
        cmd.append(self.params["vm_name"])

        return cmd

    def _convert_cloud_to_virtv2v_options(self):
        cloud = self.params["cloud"]
        opts = []

        auth = cloud.get("auth", {})

        mapping = {
            "auth_url": "auth-url",
            "username": "username",
            "password": "password",
            "project_name": "project-name",
            "project_id": "project-id",
            "user_domain_name": "user-domain-name",
        }

        for k, v in auth.items():
            if k in mapping:
                opts.append("-oo")
                opts.append(f"{mapping[k]}={v}")

        if cloud.get("region_name"):
            opts.append("-oo")
            opts.append(f"region-name={cloud['region_name']}")

        return opts

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
