# VMware Migration Kit - Claude Code Context

## Project Overview

This is an Ansible collection (`os_migrate.vmware_migration_kit`) that provides tools for migrating virtual machines from VMware (ESXi/vCenter) environments to OpenStack clouds. The collection uses a hybrid approach combining Ansible automation with custom Golang modules for high-performance migration operations.

**Version:** 2.0.8
**License:** Apache-2.0
**Repository:** https://github.com/os-migrate/vmware-migration-kit

## Architecture

### Core Components

1. **Ansible Roles** (`roles/`):
   - `prelude`: Initial setup and validation
   - `conversion_host`: Deploys and configures OpenStack instances as conversion hosts
   - `export_metadata`: Extracts VM metadata from VMware environment
   - `convert_metadata`: Transforms VMware metadata to OpenStack format
   - `import_workloads`: Handles the actual VM migration and import

2. **Golang Modules** (`plugins/modules/src/`):
   - `migrate`: Core migration engine using nbdkit for disk transfers
   - `create_server`: OpenStack instance creation
   - `create_network_port`: Network port management
   - `flavor_info`: Flavor information retrieval
   - `best_match_flavor`: Automatic flavor selection
   - `volume_info`: Cinder volume operations
   - `volume_metadata_info`: Volume metadata management

3. **Module Utilities** (`plugins/module_utils/`):
   - `vmware/`: VMware vCenter/ESXi interaction code
   - `openstack/`: OpenStack API integration (Gophercloud)
   - `nbdkit/`: NBD server management for disk streaming
   - `logger/`: Structured logging
   - `connectivity/`: Network connectivity checks
   - `ansible/`: Ansible module interface utilities

### Migration Workflows

The collection supports three migration approaches:

1. **NBDkit (Default)**: Uses nbdkit server on a conversion host for high-performance streaming
2. **Virt-v2v**: Traditional virt-v2v-based conversion with conversion host
3. **Local NFS**: Direct migration from shared NFS storage

## Key Features

- **Change Block Tracking (CBT)**: Near-zero-downtime migrations by syncing only changed blocks
- **Parallel Migrations**: Multiple concurrent migrations on same conversion host
- **Network Mapping**: Automatic network configuration translation
- **Flavor Matching**: Intelligent OpenStack flavor selection/creation
- **Multi-disk/Multi-NIC**: Full support for complex VM configurations
- **Windows Support**: Includes Windows 10 and Server 2022 support
- **AAP Compatible**: Works with Ansible Automation Platform

## File Structure

```
vmware-migration-kit/
├── galaxy.yml                    # Collection metadata
├── Makefile                      # Build automation
├── playbooks/                    # Main playbooks
│   ├── migration.yml            # Primary migration playbook
│   ├── migration_v2v.yml        # Virt-v2v workflow
│   ├── convert_metadata.yml     # Metadata conversion
│   └── ...
├── roles/                        # Ansible roles
│   ├── prelude/                 # Setup and validation
│   ├── conversion_host/         # Conversion host deployment
│   ├── export_metadata/         # VMware metadata extraction
│   ├── convert_metadata/        # Metadata transformation
│   └── import_workloads/        # VM import and migration
├── plugins/
│   ├── modules/
│   │   └── src/                 # Golang module sources
│   │       ├── migrate/         # Core migration module
│   │       ├── create_server/   # Instance creation
│   │       └── ...
│   └── module_utils/            # Shared utilities
│       ├── vmware/              # VMware SDK integration
│       ├── openstack/           # Gophercloud integration
│       ├── nbdkit/              # NBDKit server management
│       └── logger/              # Logging framework
├── tests/                        # Test suite
├── doc/                          # Architecture diagrams (SVG)
└── scripts/                      # Build and utility scripts
```

## Build System

### Makefile Targets

- `make binaries`: Build all Golang modules in container
- `make build`: Build complete Ansible collection tarball
- `make clean-binaries`: Remove compiled binaries
- `make clean-build`: Remove collection tarball
- `make install`: Install collection with dependencies
- `make tests`: Run all tests (pytest, ansible-lint, ansible-sanity, golangci-lint)
- `make test-pytest`: Run Python tests
- `make test-ansible-sanity`: Run Ansible sanity checks
- `make test-ansible-lint`: Run Ansible linting
- `make test-golangci-lint`: Run Go linting

### Build Environment

- Uses containerized builds (Podman/Docker) with CentOS Stream 10
- Golang modules are compiled to native binaries
- SELinux-aware container security options
- Python 3.12 virtual environments for testing

## Configuration

### Required Variables

**VMware Source:**
- `vcenter_hostname`: vCenter server address
- `vcenter_username`: vCenter username
- `vcenter_password`: vCenter password
- `vcenter_datacenter`: Datacenter name
- `esxi_hostname`: ESXi host address

**OpenStack Destination:**
- `dst_cloud`: OpenStack cloud credentials (auth dict)
- `openstack_private_network`: Network UUID
- `security_groups`: Security group UUID
- `ssh_key_name`: SSH key pair name (optional)

**Migration Settings:**
- `vms_list`: Array of VM names to migrate
- `os_migrate_vmw_data_dir`: Working directory (default: `/opt/os-migrate`)
- `already_deploy_conversion_host`: Reuse existing conversion host (boolean)
- `import_workloads_cbt_sync`: Enable CBT synchronization (boolean)
- `import_workloads_cutover`: Perform final cutover (boolean)

### Network Requirements

**Conversion Host Egress:**
- 443/TCP → VMware vCenter (authentication, management)
- 902/TCP → VMware ESXi (NFC/NBD disk access)
- Various → OpenStack APIs (see OpenStack firewall docs)

**Conversion Host Ingress:**
- 22/TCP ← Ansible controller (management)

**Internal:**
- 10809/TCP: NBDKit server (localhost only)

## Common Tasks

### Running a Migration

```bash
ansible-playbook -i inventory.yml \
  os_migrate.vmware_migration_kit.migration \
  -e @secrets.yml \
  -e @myvars.yml
```

### CBT Two-Phase Migration

**Phase 1 - Initial Sync:**
```yaml
import_workloads_cbt_sync: true
import_workloads_cutover: false
```

**Phase 2 - Cutover:**
```yaml
import_workloads_cbt_sync: false
import_workloads_cutover: true
```

### Standalone Module Execution

Golang modules can run independently:
```bash
export OS_AUTH_URL=https://...
export OS_PROJECT_NAME=admin
# ... other OS_* variables

cat > args.json <<EOF
{
  "user": "root",
  "password": "...",
  "server": "vcenter.example.com",
  "vmname": "my-vm",
  "dst_cloud": { ... }
}
EOF

./plugins/modules/migrate/migrate
```

## Development Workflow

### Adding a New Golang Module

1. Create module directory: `plugins/modules/src/my_module/`
2. Implement `main.go` with Ansible module interface
3. Add to `scripts/build.sh` compilation list
4. Run `make binaries` to compile
5. Create `.py` wrapper in `plugins/modules/`
6. Add documentation and argument specs
7. Run `make tests` to validate

### Testing

```bash
# Run all tests
make tests

# Individual test suites
make test-pytest           # Python unit tests
make test-ansible-lint     # Ansible best practices
make test-ansible-sanity   # Ansible module validation
make test-golangci-lint    # Go code quality
```

## Dependencies

**Ansible Collections:**
- `community.vmware` >= 1.0.0
- `openstack.cloud` >= 2.0.0 (legacy, being phased out)
- `os_migrate.os_migrate` >= 0.0.1

**Runtime (Conversion Host):**
- NBDKit with VDDK plugin
- virtio-win >= 1.40 (for Windows support)
- CentOS Stream 10 or RHEL 9.5+ recommended

**Build:**
- Golang toolchain
- Python 3.12+
- Ansible 2.9+

## Logging and Debugging

### Log Locations

**Conversion Host:**
- `/tmp/osm-nbdkit-<vm-name>-<random-id>.log`: Migration logs

**Ansible Controller:**
- `<os_migrate_vmw_data_dir>/<vm-name>/migration.log`: Pulled on failure

### Debug Mode

Enable verbose logging:
```yaml
import_workloads_debug: true
```

## VMware ACL Requirements

Minimum vCenter permissions needed:

| Category | Privileges |
|----------|-----------|
| Datastore | Browse datastore |
| Virtual Machine | Guest operations: All |
| | Provisioning: Disk access, file access, download |
| | Service configuration: Notifications, polling, read config |
| | Snapshot: Create, remove, rename, revert |

## Supported Operating Systems

| OS Family | Version | Status |
|-----------|---------|--------|
| RHEL | 9.4, 9.3, 8.5 | ✅ Tested |
| CentOS | 9, 8 | ✅ Tested |
| Fedora | 38+ | ✅ Tested |
| Ubuntu Server | 24 | ✅ Tested |
| Windows | 10 | ✅ Tested |
| Windows Server | 2022 | ✅ Tested |

## Known Limitations

- BTRFS support requires Fedora conversion host (RHEL/CentOS kernels lack BTRFS)
- Support only for latest collection version
- External dependencies (community.vmware, etc.) not covered by support
- Cold migration only for local NFS workflow

## Important Notes for AI Assistants

1. **Golang Binaries**: All `plugins/modules/*/` binaries are compiled from `plugins/modules/src/*/` Go code
2. **Don't Edit Binaries**: Always edit `.go` source files, then run `make binaries`
3. **Ansible Wrappers**: Python files in `plugins/modules/*.py` are thin wrappers around Go binaries
4. **Container Builds**: Makefile uses containers for reproducible builds - don't assume local Go toolchain
5. **Version Sync**: Keep `galaxy.yml` version in sync with `CHANGELOG.md`
6. **CBT Metadata**: Cinder volumes store CBT changeID in metadata properties
7. **Parallel Execution**: NBDKit uses Unix sockets to allow parallel migrations on same host
8. **Support Scope**: Only support collection code, not external dependencies

## Recent Changes (v2.0.8)

- Fixed VM name sanitization for invalid characters
- Replaced openstack.cloud modules with native Gophercloud bindings
- Reorganized Golang binaries with proper documentation
- Added golangci-lint integration
- Improved Makefile build system with SELinux awareness

## Useful Commands

```bash
# Build and install locally
make install

# Run migration
ansible-playbook -i inventory.yml \
  os_migrate.vmware_migration_kit.migration \
  -e @vars.yaml

# Check migration logs on conversion host
tail -f /tmp/osm-nbdkit-*.log

# Install from Galaxy
ansible-galaxy collection install os_migrate.vmware_migration_kit

# Generate VMware thumbprint (for virt-v2v)
openssl s_client -connect ESXI_SERVER:443 </dev/null | \
  openssl x509 -in /dev/stdin -fingerprint -sha1 -noout
```

## Resources

- **Documentation**: README.md, role-specific READMEs
- **Architecture Diagrams**: `doc/*.svg` (workflow visualizations)
- **Examples**: `vars.yaml`, `localhost_inventory.yml`
- **Demo**: https://www.youtube.com/watch?v=XnEQ8WVGW64
- **Issues**: https://github.com/os-migrate/vmware-migration-kit/issues
