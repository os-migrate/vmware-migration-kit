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
