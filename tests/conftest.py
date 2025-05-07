import pytest
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
            f"esx://{self.params['vcenter_username']}@{self.params['vcenter_hostname']}/Datacenter/{self.params['esxi_hostname']}?no_verify=1",
            "-it",
            "vddk",
            "-io",
            f"vddk-libdir={self.params['vddk_libdir']}",
            "-io",
            f"vddk-thumbprint={self.params['vddk_thumbprint']}",
            "-o",
            "openstack",
            "-oo",
            f"server-id={self.params['conversion_host_id']}",
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


@pytest.fixture()
def virt_v2v_instance():
    params = {
        "vcenter_username": "test_user",
        "vcenter_hostname": "test_host",
        "esxi_hostname": "test_esxi",
        "vddk_libdir": "/usr/lib/vmware-vix-disklib",
        "vddk_thumbprint": "XX:XX:XX:XX",
        "conversion_host_id": "test_conversion_id",
        "vm_name": "test_vm",
    }
    return VirtV2V(params)
