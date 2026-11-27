#!/usr/bin/python

DOCUMENTATION = r"""
module: proxy_precheck
short_description: Checks that a proxy VM is ready for the proxy disk transport.
description:
  - With the proxy transport, the snapshot disks of the VMs to migrate are hot-added (read-only) to a Linux proxy VM
    running in vSphere, and read from the nbdkit service already running there. No VDDK, NFC or SSH is used, and
    nothing is executed inside the proxy VM, so it needs no credentials.
  - This module checks, through the vSphere API, that the proxy VM exists, is powered on, has C(disk.EnableUUID=TRUE)
    and a SCSI controller with free slots.
  - It connects to the nbdkit service of the proxy from the host running the module (the conversion host), which
    validates routing, NAT and the proxy firewall, and asks it for one of the proxy's own disks to confirm it serves
    C(dir=/dev/disk/by-id) read-only.
  - For every VM in C(vms_list), it checks that the VM exists, is not the proxy, and that each disk can be hot-added
    to the proxy (datastore accessible from the proxy ESXi host, not an independent disk or a physical-mode RDM, no
    disk UUID shared with the proxy). It warns when CBT is disabled and C(cbt_sync) is requested.
  - The module changes nothing, on either side.
  - It must run on the conversion host, the host that will read from the proxy.
author: "OpenStack tenant migration tools (@os-migrate)"
version_added: "2.4.0"
options:
  server:
    description:
      - vCenter hostname or IP address.
    type: str
    required: true
  user:
    description:
      - vCenter username.
    type: str
    required: true
  password:
    description:
      - vCenter password.
    type: str
    required: true
  vmware_insecure:
    description:
      - Skip TLS certificate verification for vCenter.
    type: bool
    default: false
  datacenter:
    description:
      - vCenter datacenter. Defaults to the only datacenter if there is one.
    type: str
  proxy_vm_name:
    description:
      - Name or inventory path of the proxy VM.
    type: str
    required: true
  proxy_vm_ip:
    description:
      - IP address of the proxy VM used by the conversion host to reach nbdkit. It must be an address of the proxy
        itself (nbdkit binds to it).
      - Defaults to the IP address reported by VMware Tools.
    type: str
  nbd_port:
    description:
      - Port of the nbdkit service of the proxy. A single port serves every disk, they are told apart by NBD export
        name, so parallel migrations need no coordination.
    type: int
    default: 10809
  vms_list:
    description:
      - Names or inventory paths of the VMs to migrate.
    type: list
    elements: str
    default: []
  cbt_sync:
    description:
      - Whether CBT incremental sync will be used. If true, VMs with CBT disabled produce a warning.
    type: bool
    default: false
  check_network:
    description:
      - Connect to the nbdkit service of the proxy to check this host can reach it.
    type: bool
    default: true
requirements:
  - The proxy VM runs C(nbdkit --readonly --port 10809 file dir=/dev/disk/by-id) as a service, with the port open
    for the conversion host, and has C(disk.EnableUUID=TRUE).
"""

EXAMPLES = r"""
- name: Check the proxy VM before migrating
  os_migrate.vmware_migration_kit.proxy_precheck:
    server: "{{ vcenter_hostname }}"
    user: "{{ vcenter_username }}"
    password: "{{ vcenter_password }}"
    vmware_insecure: true
    datacenter: "{{ vcenter_datacenter }}"
    proxy_vm_name: proxy-vm
    proxy_vm_ip: 10.0.0.55
    nbd_port: 10809
    vms_list:
      - rhel-9-vm
      - windows-2022-vm
    cbt_sync: true
  register: proxy_check
"""

RETURN = r"""
changed:
  description: Always C(false), the module only checks.
  returned: always
  type: bool
  sample: false
msg:
  description: Summary of the result, with the failed checks if any.
  returned: always
  type: str
  sample: "Proxy VM proxy-vm is ready (13 ok, 0 warning(s))"
checks:
  description: Result of every check.
  returned: always
  type: list
  elements: dict
  contains:
    name:
      description: Check name, for example C(proxy_enable_uuid), C(network) or C(vm_disk_datastore).
      type: str
    target:
      description: What was checked (proxy VM, port, VM or VM disk).
      type: str
    status:
      description: C(ok), C(warn) or C(fail). The module fails if at least one check has C(fail).
      type: str
    msg:
      description: Details and, on failure, how to fix it.
      type: str
  sample:
    - name: proxy_enable_uuid
      target: proxy-vm
      status: ok
      msg: disk.EnableUUID=TRUE
facts:
  description: Information about the proxy VM.
  returned: on success
  type: dict
  sample:
    name: proxy-vm
    address: 10.0.0.55
    host: esxi01.example.com
    power_state: poweredOn
    datastores: [datastore1]
    proxy_disk_wwns: ["wwn-0x6000c291f0c8470c061c1a10d75fcf14"]
    free_slots: 14
    port: 10809
summary:
  description: Number of checks per status.
  returned: on success
  type: dict
  sample: {"ok": 13, "warn": 0, "fail": 0}
"""
