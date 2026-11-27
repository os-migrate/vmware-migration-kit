# proxy_vm

Checks the **proxy VM** used by the proxy disk transport.

With this transport, the snapshot disk of each VM to migrate is hot-added
(read-only, independent-nonpersistent) to a small Linux VM running in vSphere,
and read from there over NBD by the conversion host. There is no VDDK, no NFC
and no SSH to ESXi.

**Nothing is ever executed inside the proxy VM, and it needs no credentials.**
The proxy permanently runs one nbdkit service:

```
nbdkit --readonly --port 10809 file dir=/dev/disk/by-id
```

The `file` plugin resolves the NBD *export name* to a file of that directory
**when a client connects**, so a disk hot-added long after nbdkit started is
served straight away, under the export name of its by-id link
(`wwn-0x<disk uuid>`). A migration therefore only needs:

1. the vSphere API, to attach and detach the disk, and
2. a TCP connection from the conversion host to that single port.

One port serves every disk, in parallel: they are told apart by export name,
so concurrent migrations need no coordination. The limit is the number of free
SCSI slots on the proxy:

| Controller | Targets | Usable per controller |
|---|---|---|
| PVSCSI, hardware version 14 and later (vSphere 6.7+) | 64 | 63 |
| PVSCSI on older hardware, LSI Logic | 16 | 15 |

Up to 4 SCSI controllers per VM, so a modern proxy holds **~250 disks at
once**, minus its own. The pre-check reports the real figure for your proxy.
Add controllers, or proxy VMs, to go further. In practice the destination
storage saturates long before the slots do.

For now the proxy VM is prepared by hand (see below).
TODO: build a proxy OVA with the service included and let this role deploy it.

## Tasks

- `configure.yml`: turns a **blank Fedora / CentOS Stream / RHEL VM** into a
  proxy. Runs **on the proxy VM** (inventory group `proxy_vm`), with the
  vCenter calls delegated to the controller. It installs nbdkit, deploys and
  enables the export service, opens the port for the conversion hosts, and
  makes sure vSphere exposes disk UUIDs in the guest. Idempotent.
  See `playbooks/proxy_vm_setup.yml`.
- `precheck.yml` (default): runs the `proxy_precheck` module **on the
  conversion host** and fails if the proxy cannot be used. It checks:
  - the proxy VM exists, is powered on, has `disk.EnableUUID=TRUE` and a SCSI
    controller with free slots;
  - the nbdkit service answers **from the conversion host**, and serves the
    proxy's own disks by by-id name, read-only (so routing, NAT, the firewall
    and the service configuration are all validated at once);
  - each VM of `vms_list`: exists, is not the proxy, every disk is on a
    datastore the proxy ESXi host can access, no independent disk or
    physical-mode RDM, no disk UUID shared with the proxy, CBT enabled when
    `cbt_sync` is used (warning).

## Variables

| Variable | Default | Description |
|---|---|---|
| `proxy_vm_name` | `""` | Proxy VM name or inventory path (required) |
| `proxy_vm_ip` | `""` | Proxy IP used by the conversion host; empty = IP from VMware Tools |
| `proxy_vm_nbd_port` | `10809` | Port of the nbdkit service of the proxy |
| `proxy_vm_ssh_user` | `cloud-user` | SSH user, for provisioning/debugging only |
| `proxy_vm_ssh_private_key_file` | `""` | SSH private key, for provisioning/debugging only |
| `proxy_vm_ssh_public_key` | `""` | Public key to inject in the future proxy OVA (TODO) |
| `proxy_vm_vms_list` | `vms_list` | VMs to check |
| `proxy_vm_cbt_sync` | `cbt_sync` | Warn when CBT is disabled |
| `proxy_vm_vmware_insecure` | `vmware_insecure` | Skip vCenter TLS verification |
| `proxy_vm_check_network` | `true` | Run the NBD service check |

`configure.yml` only:

| Variable | Default | Description |
|---|---|---|
| `proxy_vm_allowed_sources` | `[]` | Conversion host addresses allowed to reach the NBD port, **as the proxy sees them** (required) |
| `proxy_vm_bind_address` | `proxy_vm_ip` or the default IPv4 | Address nbdkit binds to |
| `proxy_vm_packages` | `nbdkit`, `nbdkit-basic-plugins` | Packages to install |
| `proxy_vm_configure_firewall` | `true` | Manage the firewalld rule |
| `proxy_vm_allow_power_cycle` | `false` | Allow restarting the VM to apply `disk.EnableUUID` |

Also uses `vcenter_hostname`, `vcenter_username`, `vcenter_password` and
`vcenter_datacenter`.

## Preparing the proxy VM

### Automatically (recommended)

Start from a blank Fedora / CentOS Stream / RHEL VM meeting the hardware
requirements below, add it to the inventory:

```yaml
proxy_vm:
  hosts:
    10.0.0.55:
      ansible_ssh_user: cloud-user
      ansible_ssh_private_key_file: /home/stack/.ssh/proxy-vm
```

and run:

```bash
ansible-playbook -i inventory.yml \
  os_migrate.vmware_migration_kit.proxy_vm_setup \
  -e @secrets.yml \
  -e proxy_vm_name=proxy-vm \
  -e '{"proxy_vm_allowed_sources": ["10.46.107.2/32"]}'
```

`proxy_vm_allowed_sources` is the conversion host address **as the proxy sees
it**: with NAT between the clouds that is the translated address, not what the
conversion host sees on itself. To find it, open a connection from the
conversion host to the proxy and look at `ss -tn` on the proxy.

If the guest has no `/dev/disk/by-id/wwn-*` links, the role sets
`disk.EnableUUID=TRUE` in vCenter. That only takes effect after a power cycle,
so the role then stops and tells you, unless `proxy_vm_allow_power_cycle=true`
lets it restart the VM itself.

Then check it from the conversion host with `playbooks/proxy_precheck.yml`.

### Hardware requirements (not configured by the role)

- Linux (RHEL/CentOS Stream 9/10), 2-4 vCPU, 2-4 GB RAM, VMXNET3 NIC.
- PVSCSI or LSI Logic SCSI controller (hot-add does not work on IDE).
  Prefer PVSCSI on hardware version 14 or later: 63 usable slots per
  controller instead of 15. Add controllers for more parallel disks.
- VM advanced setting **`disk.EnableUUID=TRUE`** (needs a power off). Without
  it the hot-added disks have no `/dev/disk/by-id/wwn-*` link and cannot be
  identified, so the migration refuses to use them.
- On an ESXi host that can access the datastores of the VMs to migrate. Add a
  DRS rule so the proxy does not drift to a host that cannot.
- Never clone the proxy from a VM to migrate (duplicate disk UUIDs).

### By hand

What `configure.yml` does, if you prefer doing it yourself:

```bash
dnf install -y nbdkit nbdkit-basic-plugins
printf 'OSM_NBD_ADDRESS=10.0.0.55\nOSM_NBD_PORT=10809\n' > /etc/sysconfig/osm-nbdkit
cp osm-nbdkit.service /etc/systemd/system/      # from this role's files/
systemctl daemon-reload
systemctl enable --now osm-nbdkit
firewall-cmd --permanent --add-rich-rule='rule family="ipv4" source address=<conv-host-ip>/32 port port=10809 protocol=tcp accept'
firewall-cmd --reload
```

> **Security.** The service exports *every* disk attached to the proxy,
> including its own system disk, read-only to anyone who can reach the port.
> The firewall restriction is therefore part of the design. Consider also
> nbdkit TLS-PSK when the traffic leaves a trusted network.

## Example

```yaml
- name: Check the proxy VM
  hosts: conversion_host
  roles:
    - role: os_migrate.vmware_migration_kit.proxy_vm
      vars:
        proxy_vm_name: proxy-vm
        proxy_vm_ip: 10.0.0.55
```

Then migrate with `transport: proxy` (see the `import_workloads` role).

With `transport: proxy` the conversion host needs **no VMware VIX DiskLib and
no nbdkit plugin**: nbdkit runs on the proxy. The `conversion_host` role skips
its libdir, custom-plugin and OpenVixDiskLib checks accordingly, so
`custom_nbdkit_plugin` or `use_openvixdisklib` left over from a VDDK setup no
longer make the run fail.

## Skipping the checks

`migration.yml` runs the pre-checks on every migration. To start copying
sooner, for example when re-running against a lab you have just validated:

```bash
-e run_prechecks=false                              # collection + proxy checks
-e os_migrate_conversion_host_content_install=false # conversion host setup (slower)
```

`proxy_vm_check_network=false` skips only the NBD reachability probe while
keeping the rest.

## VMs with several disks

Disks are migrated **one after the other**: for each disk the kit snapshots the
VM, attaches that disk to the proxy, copies it into its own Cinder volume and
detaches it. Each disk is served under its own export name
(`wwn-0x<disk uuid>`), so names never collide, and only one disk of a given VM
is attached at a time. The free SCSI slots of a proxy therefore limit how many
**VMs** migrate in parallel, not how many disks a VM may have. `multidiskfs`
(a filesystem spanning several disks) is unaffected: it works on the Cinder
volumes once every disk has been copied.

Two caveats, both pre-existing and identical with the VDDK transport: each disk
gets its own snapshot, taken at a slightly different moment, so the disks of a
multi-disk VM are not one single point in time; and two migrations of the
**same** VM must not run at the same time.
