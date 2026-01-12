# VMware Migration Kit - AI Agent Context

> This document provides context for AI coding assistants working with this repository.

## Project Overview

**Name:** `os_migrate.vmware_migration_kit`  
**Version:** 2.2.0  
**License:** Apache-2.0  
**Repository:** https://github.com/os-migrate/vmware-migration-kit

This is an Ansible collection for migrating virtual machines from VMware (ESXi/vCenter) to OpenStack clouds. It uses a **hybrid architecture** combining:
- **Ansible roles & playbooks** for orchestration and workflow management
- **Go binaries** for high-performance migration operations (compiled to native executables)
- **Python wrappers** for Ansible module interface compatibility

## Architecture

### Directory Structure

```
vmware-migration-kit/
├── galaxy.yml                    # Ansible collection metadata (source of truth for version)
├── Makefile                      # Build automation (containers, tests, binaries)
├── go.mod / go.sum               # Go module dependencies
├── playbooks/                    # Main entry point playbooks
│   ├── migration.yml             # Primary NBDKit-based migration
│   ├── migration_v2v.yml         # Virt-v2v workflow
│   └── ...
├── roles/
│   ├── prelude/                  # Setup and validation
│   ├── conversion_host/          # OpenStack conversion host deployment
│   ├── export_metadata/          # VMware metadata extraction
│   ├── convert_metadata/         # Metadata transformation (VMware → OpenStack)
│   └── import_workloads/         # VM import and disk migration
├── plugins/
│   ├── modules/                  # Ansible modules (Python wrappers + Go binaries)
│   │   ├── *.py                  # Python wrapper files (DOCUMENTATION, thin interface)
│   │   ├── migrate               # Compiled Go binary (no extension)
│   │   ├── create_server         # Compiled Go binary
│   │   └── src/                  # Go source code (not shipped in collection)
│   │       ├── migrate/
│   │       │   └── migrate.go
│   │       ├── create_server/
│   │       │   └── create_server.go
│   │       └── ...
│   └── module_utils/             # Shared utilities
│       ├── vmware/               # govmomi vCenter/ESXi integration
│       ├── openstack/            # Gophercloud OpenStack integration
│       ├── nbdkit/               # NBD server management
│       ├── logger/               # Structured logging (logrus)
│       ├── ansible/              # Ansible module interface helpers
│       ├── connectivity/         # Network connectivity checks
│       └── utils.go              # Common utilities
├── tests/
│   ├── unit/                     # Go unit tests (*_test.go)
│   ├── integration/              # Ansible integration tests
│   └── sanity/                   # Ansible sanity ignore files
├── scripts/
│   └── build.sh                  # Containerized Go build script
└── aee/                          # Ansible Execution Environment config
```

### Component Relationships

```
┌─────────────────────────────────────────────────────────────────┐
│                     Ansible Controller                          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │ playbooks/   │  │ roles/       │  │ vars.yaml    │          │
│  └──────┬───────┘  └──────┬───────┘  └──────────────┘          │
│         │                 │                                     │
│         └────────┬────────┘                                     │
│                  ▼                                               │
│  ┌───────────────────────────────────────────────────────────┐ │
│  │              plugins/modules/*.py (wrappers)              │ │
│  │    (DOCUMENTATION + argument passing to Go binaries)      │ │
│  └─────────────────────────┬─────────────────────────────────┘ │
│                            ▼                                    │
│  ┌───────────────────────────────────────────────────────────┐ │
│  │           plugins/modules/<binary> (Go executables)       │ │
│  │      migrate | create_server | flavor_info | ...          │ │
│  └─────────────────────────┬─────────────────────────────────┘ │
│                            │                                    │
└────────────────────────────┼────────────────────────────────────┘
                             ▼
┌────────────────────────────────────────────────────────────────┐
│                    Conversion Host (OpenStack VM)              │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐            │
│  │   nbdkit    │  │  virt-v2v   │  │  virtio-win │            │
│  │   server    │  │ (optional)  │  │  (Windows)  │            │
│  └──────┬──────┘  └─────────────┘  └─────────────┘            │
│         │                                                      │
│    ┌────┴────┐                                                 │
│    │ NBD/VDDK│────────────────────────────────────────────────┼──► VMware ESXi/vCenter
│    └─────────┘                                                 │      (port 902/TCP)
│         │                                                      │
│    ┌────┴────┐                                                 │
│    │ Cinder  │────────────────────────────────────────────────┼──► OpenStack APIs
│    └─────────┘                                                 │
└────────────────────────────────────────────────────────────────┘
```

### Go Module Structure

Go modules follow a consistent pattern:

```go
// plugins/modules/src/<module_name>/<module_name>.go
package main

import (
    "vmware-migration-kit/plugins/module_utils/ansible"
    // ... other module_utils imports
)

type ModuleArgs struct {
    // Ansible module arguments (JSON unmarshaled)
    FieldName string `json:"field_name"`
}

func main() {
    var response ansible.Response
    // 1. Read args file from os.Args[1]
    // 2. Unmarshal JSON into ModuleArgs
    // 3. Validate required fields with ansible.RequireField()
    // 4. Execute business logic
    // 5. Return with ansible.ExitJson() or ansible.FailJson()
}
```

## Build System

### Key Makefile Targets

| Target | Description |
|--------|-------------|
| `make binaries` | Build all Go modules in container (CentOS Stream 9) |
| `make build` | Build complete Ansible collection tarball |
| `make clean-binaries` | Remove compiled binaries from `plugins/modules/` |
| `make install` | Build and install collection with dependencies |
| `make tests` | Run all tests (pytest, ansible-lint, sanity, golangci-lint) |
| `make test-pytest` | Python unit tests |
| `make test-ansible-lint` | Ansible linting |
| `make test-ansible-sanity` | Ansible module validation |
| `make test-golangci-lint` | Go code quality (golangci-lint v2) |

### Build Process

1. Binaries are built inside a **container** (Podman/Docker with CentOS Stream 9)
2. Uses `go build -ldflags="-s -w"` for smaller binaries
3. UPX compression applied when available
4. Output binaries placed in `plugins/modules/<module_name>` (no file extension)
5. `plugins/modules/src/` is excluded from the built collection (see `galaxy.yml` `build_ignore`)

### Building Binaries

```bash
# Full containerized build
make binaries

# Or manually with container
podman run --rm -v $(pwd):/code/ quay.io/centos/centos:stream9 /code/scripts/build.sh
```

## Development Workflow

### Adding a New Go Module

1. Create directory: `plugins/modules/src/my_module/`
2. Implement `my_module.go` following the pattern above
3. Create Python wrapper: `plugins/modules/my_module.py` with DOCUMENTATION, EXAMPLES, RETURN
4. Run `make binaries` to compile
5. Add sanity exclusions in `tests/sanity/ignore-2.*.txt` if needed (for binary files)
6. Run `make tests` to validate

### Modifying Existing Go Code

1. Edit `.go` files in `plugins/modules/src/<module>/` or `plugins/module_utils/`
2. Run `make binaries` to recompile
3. Run `make test-golangci-lint` to check code quality
4. Run Go unit tests: `go test ./tests/unit/...`

### Testing

```bash
# All tests
make tests

# Individual test suites
make test-pytest           # Python tests (tests/test_*.py)
make test-golangci-lint    # Go linting
make test-ansible-sanity   # Ansible validation
make test-ansible-lint     # Ansible best practices

# Go unit tests directly (requires local Go toolchain)
go test -v ./tests/unit/...
```

## Code Conventions

### Go Code

- **License header**: Apache 2.0 with Red Hat copyright on all `.go` files
- **Package naming**: Module main packages use `package main`; utilities use descriptive names
- **Imports**: Group stdlib, then external, then internal (`vmware-migration-kit/...`)
- **Error handling**: Return errors up the chain; log with `logger.Log.Infof()`
- **Ansible interface**: Use `ansible.RequireField()`, `ansible.DefaultIfEmpty()`, `ansible.ExitJson()`, `ansible.FailJson()`
- **JSON tags**: Use `json:"snake_case"` for struct fields

### Python Code

- **Wrappers only**: Python files in `plugins/modules/` are thin wrappers; business logic is in Go
- **DOCUMENTATION**: Full Ansible module documentation in docstring
- **Python 2.7 compatibility**: No f-strings; use `from __future__ import` for metaclass boilerplate

### Ansible

- **Variable naming**: `snake_case` with descriptive prefixes (e.g., `import_workloads_*`, `os_migrate_vmw_*`)
- **Role structure**: Follow standard `defaults/main.yml`, `tasks/main.yml`, `meta/main.yml` layout
- **Idempotency**: Check state before making changes; support repeated runs

## Key Technical Details

### Migration Workflows

1. **NBDKit (Default)**: Uses nbdkit with VDDK plugin for streaming disks to Cinder volumes
2. **Virt-v2v**: Traditional conversion with guest OS modification
3. **CBT (Change Block Tracking)**: Two-phase migration for near-zero downtime

### CBT Two-Phase Migration

```yaml
# Phase 1 - Initial sync (VM keeps running)
import_workloads_cbt_sync: true
import_workloads_cutover: false

# Phase 2 - Final cutover (VM stopped, delta sync)
import_workloads_cbt_sync: false
import_workloads_cutover: true
```

### Volume Metadata

Cinder volumes store migration state in metadata:
- `osm: "true"` - Managed by os-migrate
- `changeID: "<CBT-ID>"` - VMware CBT change identifier
- `converted: "true/false"` - V2V conversion status

### Network Requirements

| Direction | Port | Purpose |
|-----------|------|---------|
| Conversion Host → vCenter | 443/TCP | Authentication, management |
| Conversion Host → ESXi | 902/TCP | NFC/NBD disk access |
| Conversion Host → OpenStack | Various | API endpoints |
| Ansible Controller → Conversion Host | 22/TCP | SSH management |
| Internal | 10809/TCP | NBDKit server (localhost) |

## Critical Rules for AI Agents

### DO

- ✅ Edit `.go` source files in `plugins/modules/src/` or `plugins/module_utils/`
- ✅ Run `make binaries` after any Go code changes
- ✅ Keep `galaxy.yml` version in sync with `CHANGELOG.md`
- ✅ Add Apache 2.0 license headers to new Go files
- ✅ Use existing `module_utils` packages for VMware/OpenStack/logging
- ✅ Follow the established Ansible module pattern (ModuleArgs → validation → logic → Response)
- ✅ Add sanity ignore entries for new binary modules

### DON'T

- ❌ Edit binary files (`plugins/modules/migrate`, etc.) - they're compiled artifacts
- ❌ Assume local Go toolchain - use `make binaries` for reproducible builds
- ❌ Add Python business logic - put it in Go modules instead
- ❌ Use f-strings in Python (breaks Python 2.7 compatibility for sanity tests)
- ❌ Modify files in `build_ignore` paths expecting them to ship in collection

### Version Management

The authoritative version is in `galaxy.yml`. When updating:
1. Update `version` in `galaxy.yml`
2. Add entry to `CHANGELOG.md`
3. Tag release with `v<version>` (e.g., `v2.2.0`)

## Dependencies

### Go Dependencies (go.mod)

- `github.com/gophercloud/gophercloud/v2` - OpenStack SDK
- `github.com/vmware/govmomi` - VMware vSphere SDK
- `github.com/sirupsen/logrus` - Structured logging
- `libguestfs.org/libnbd` - NBD client library

### Ansible Collection Dependencies (galaxy.yml)

- `vmware.vmware` >= 2.4.0
- `vmware.vmware_rest` >= 4.9.0

### Conversion Host Runtime

- nbdkit with VDDK plugin
- virt-v2v (optional, for conversion workflow)
- virtio-win >= 1.40 (Windows migrations)
- CentOS Stream 10 or RHEL 9.5+ recommended

## Logging & Debugging

### Log Locations

- **Conversion Host**: `/tmp/osm-nbdkit-<vm-name>-<random-id>.log`
- **Ansible Controller**: `<os_migrate_vmw_data_dir>/<vm-name>/migration.log`

### Enable Debug Mode

```yaml
import_workloads_debug: true
```

## Quick Reference Commands

```bash
# Build and install locally
make install

# Run migration
ansible-playbook -i inventory.yml os_migrate.vmware_migration_kit.migration -e @vars.yaml

# Check migration logs on conversion host
tail -f /tmp/osm-nbdkit-*.log

# Install from Galaxy
ansible-galaxy collection install os_migrate.vmware_migration_kit

# Run Go tests directly
go test -v ./tests/unit/...

# Lint Go code
make test-golangci-lint
```

## Resources

- **Documentation**: https://os-migrate.github.io/documentation/
- **Issues**: https://github.com/os-migrate/vmware-migration-kit/issues
- **Demo**: https://www.youtube.com/watch?v=XnEQ8WVGW64
- **Architecture Diagrams**: `doc/*.svg`
