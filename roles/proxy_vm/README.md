# proxy_vm

Checks the **proxy VM** used by the proxy disk transport.

With this transport, the snapshot disk of each VM to migrate is hot-added
(read-only, independent-nonpersistent) to a small Linux VM running in vSphere,
and exported with nbdkit to the conversion host. There is no VDDK, no NFC and
no SSH to ESXi. The proxy is driven only through the vSphere API
(reconfigure + guest operations).

For now the proxy VM is prepared by hand (see "Proxy VM requirements").
TODO: build a proxy OVA with everything included and let this role deploy it.

## Tasks

- `precheck.yml` (default): runs the `proxy_precheck` module **on the
  conversion host** and fails if the proxy cannot be used. It checks:
  - the proxy VM exists, is powered on, runs VMware Tools, has
    `disk.EnableUUID=TRUE` and a SCSI controller with free slots;
  - guest operations: credentials, passwordless sudo, nbdkit + file plugin;
  - network: a temporary nbdkit on the first and last port of the range must be
    reachable from the conversion host (routing, NAT, proxy firewall);
  - each VM of `vms_list`: exists, is not the proxy, every disk is on a
    datastore the proxy ESXi host can access, no independent disk or
    physical-mode RDM, no disk UUID shared with the proxy, CBT enabled when
    `cbt_sync` is used (warning).

## Variables

| Variable | Default | Description |
|---|---|---|
| `proxy_vm_name` | `""` | Proxy VM name or inventory path (required) |
| `proxy_vm_ip` | `""` | Proxy IP used by the conversion host; empty = IP from VMware Tools |
| `proxy_vm_guest_username` | `cloud-user` | Guest user for vSphere guest operations (passwordless sudo) |
| `proxy_vm_guest_password` | `""` | Its password (required; guest operations only accept passwords) |
| `proxy_vm_ssh_user` | `cloud-user` | SSH user for provisioning/debugging (not used by the migration) |
| `proxy_vm_ssh_private_key_file` | `""` | SSH private key, like the conversion host |
| `proxy_vm_ssh_public_key` | `""` | Public key to inject in the future proxy OVA (TODO) |
| `proxy_vm_nbd_port_min` | `10809` | First NBD port |
| `proxy_vm_nbd_port_max` | `port_min + 63` | Last NBD port (one per SCSI slot, 64 max; use more proxies to scale) |
| `proxy_vm_vms_list` | `vms_list` | VMs to check |
| `proxy_vm_cbt_sync` | `cbt_sync` | Warn when CBT is disabled |
| `proxy_vm_vmware_insecure` | `vmware_insecure` | Skip vCenter TLS verification |
| `proxy_vm_check_network` | `true` | Run the end-to-end NBD network check |

Also uses `vcenter_hostname`, `vcenter_username`, `vcenter_password` and
`vcenter_datacenter`.

## Proxy VM requirements

- Linux (RHEL/CentOS Stream 9/10), 2-4 vCPU, 2-4 GB RAM, VMXNET3 NIC.
- PVSCSI or LSI Logic SCSI controller (hot-add does not work on IDE).
  Each controller gives 15 slots; add controllers for more parallel disks.
- VM advanced setting `disk.EnableUUID=TRUE` (needs a power off).
- On an ESXi host that can access the datastores of the VMs to migrate.
- Packages: `nbdkit`, `nbdkit-basic-plugins`, `open-vm-tools` (running).
- A guest user with passwordless sudo.
- Firewall: the NBD port range open **only** for the conversion host
  (use the conversion host address as seen by the proxy, after NAT):

  ```
  firewall-cmd --permanent --add-rich-rule='rule family=ipv4 source address=<conv-host-ip>/32 port port=10809-10872 protocol=tcp accept'
  firewall-cmd --reload
  ```

- Never clone the proxy from a VM to migrate (duplicate disk UUIDs).

## Example

```yaml
- name: Check the proxy VM
  hosts: conversion_host
  roles:
    - role: os_migrate.vmware_migration_kit.proxy_vm
      vars:
        proxy_vm_name: proxy-vm
        proxy_vm_ip: 10.0.0.55
        proxy_vm_guest_password: "{{ vault_proxy_vm_guest_password }}"
```
