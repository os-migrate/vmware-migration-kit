The role import_workloads runs the migration for a given virtual machines from a VMWare environment to an OpenStack environment.
It creates network port, OpenStack instance and rus the migration with nbdkit or virt-v2v. It has also a teardown set of tasks which cleans the OpenStack environment at the end.

## TLS Certificate Verification

By default, OS-Migrate verifies TLS certificates for both the VMware vCenter and the OpenStack API endpoints. If either environment uses self-signed or untrusted certificates, the migration will fail with an error such as:

```
Failed to initiate Vmware client: Post "https://vcenter.example.com/sdk": tls: failed to verify certificate: x509: certificate signed by unknown authority
```

Two variables control this behavior:

| Variable | Default | Description |
|---|---|---|
| `import_workloads_vmware_insecure` | `false` | Skip TLS certificate verification for the VMware vCenter connection |
| `import_workloads_openstack_insecure` | `false` | Skip TLS certificate verification for the OpenStack API connection |

Set one or both to `true` when the certificates are self-signed or issued by an authority not trusted by the conversion host:

```yaml
import_workloads_vmware_insecure: true
import_workloads_openstack_insecure: true
```

These variables inherit from the shorter aliases `vmware_insecure` and `openstack_insecure` if those are set at a higher scope.

> **Warning:** Disabling certificate verification removes protection against man-in-the-middle attacks. Use only in lab or trusted network environments.

## Heat

`use_heat: true` is the create path: NBDKit writes Cinder volumes, Heat creates ports and instances, volumes stay `external_id`.

To wrap VMs that were already created with `create_server` (no Heat), run the standalone playbook once for the full VM list. Do not put wrap inside the per-VM `import_workloads` loop.

```yaml
- import_playbook: os_migrate.vmware_migration_kit.wrap_heat_stack
  vars:
    vms_list: ["rhel-1", "rhel-2"]
    dst_cloud: "{{ dst_cloud }}"
    heat_stack_name: os-migrate-wrapped
```

The wrap template references existing servers, ports, and volumes with `external_id` only. Re-running against the same `heat_stack_name` skips create if that stack is already COMPLETE.

Deleting a create-mode `use_heat: true` stack with `openstack stack delete` also deletes the Heat-managed Neutron ports and Nova instances. Volumes stay because they are `external_id`. To leave that stack without killing VMs, run the standalone abandon playbook. It needs `heat_stack_name` (create-mode names are `os-migrate-<epoch>`; read it from `heat_stack_info.txt`). Do not put abandon inside the per-VM `import_workloads` loop.

```yaml
- import_playbook: os_migrate.vmware_migration_kit.abandon_heat_stack
  vars:
    heat_stack_name: os-migrate-1710000000
    dst_cloud: "{{ dst_cloud }}"
```

The module tries Heat `stacks.Abandon()` first. True Abandon requires Heat `enable_stack_abandon = True`. Test that path on DevStack/ITUP, not PSI. RHOS/PSI leaves `enable_stack_abandon` off (`Stack Abandon is not supported`); the fallback rewrites Heat-managed `OS::Nova::Server` and `OS::Neutron::Port` resources to `external_id` using the stack's live logical names (create-mode ports stay `{sanitized}_port`, not wrap's `{sanitized}_port_{i}`), then deletes the stack. Wrap stacks already use `external_id` on ports and instances, so `openstack stack delete` on a wrap stack does not kill those VMs.
