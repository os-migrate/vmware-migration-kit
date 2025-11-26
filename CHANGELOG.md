# Changelog

## v1.0.0

This release the cut the following:

Migration from VMware - VCenter (7 and 8) for a Linux Operation system (Ubuntu-server, CentOS 8/9 and RHEL 8/9) with a conversion host in the OpenStack destination cloud and virt-v2v.

Release features:

- Migration from VCenter to OpenStack with virt-v2v and a conversion host
- Network mapping between VMware and OpenStack cloud
- Finding of the best match flavor or flavor creation
- Persistent macadresses
- Discover VMware hosts for futher data interpretation in the futur.

## v1.0.1

Publish Openstack Volume modules.

## v1.2.0

Improve stability.
Fix issues:

- add network_config for rhel nics
- do not failed migration if vmware tools is not installed
- multi nics & disks support

## v1.2.1

This minor release fix:

- expose vddk path to the migrate module
- extract vddk path from guest info
- do not use network_info.json for extracting mac addresses only

## v1.2.2

## v1.2.3

Move playbook migration.yml to playbooks directory so the form os_migrate.vmware_migration_kit.migration could use to call the migration.

## v1.2.4

This minor release fix a bug when after the migration the volume was not detached correctly due to hardcoded value.

## v1.2.5

## v1.2.6

Extend network_config for all Linux distributions.

## v1.2.7

This release adds:

- logrus for a better logging stream
- the golang binary is now logging to the stdout as well
- the migration log files are now uniq per migration with vm-name and random string
- the nbdkit server can use socket rather than only uri, it allows to spawn severals migration workflow for a single conversion host.

## v1.2.8

This minor release includes:

- Bug fix when a virtual machine has more than one disk and the CBT migration is enable.
- Bug fix when the CBT data sync use the socket.
- A way to handle better the CBT option in the import_workloads role: the migration can be run several time, if the correct flags is set the CBT sync will be done or skip if not provided.
- Set volume metadata to specified if the volume has been converted: The conversion can happen only once, so if true, then we should leave and skip the volume as soon as possible.
- Add new module: volume_metadata_info to get the information of the volume (converted or not)

## v1.3.1

This minor release adds:

- support of key_name for instance creation
- setup requirements for migrator host

## v1.3.2

This release includes minor fixes:

- Set default variables in order to make roles independent
- Add support for Windows srv 2K22
- Update requirements and checks for virtio-win min version

## v1.3.3

This minor release fix become for AEE in the requirements steps.

## v1.3.5

Publish v1.3.5 (skipping 1.3.4 tag, because it's already published in Ansible Galaxy).
This release includes severals fixes to improve performance:

- A dedicated Go ansible module for OpenStack instance creation, which aims to fix timeout issue with the legacy module
- Buffer scan improvement when the Migration code tries to read large chunk of bytes
- Minor nits in the Ansible code.

## v2.0.0

This release includes major refactoring for the collection (path), Golang linter and tests and Makefile for the builds.

## v2.0.2

This release includes fixes for Ansible Automation Hub and Python 2.7 linters.

## v2.0.3

This minor release includes fixes for Ansible Galaxy & automation hub + a way to boot instance with existing Cinder volume.

## v2.0.4

This minor release includes changelog update and support section in the README.md

## v2.0.5

This minor release removes unused playbooks

## v2.0.6

This release is the latest minor release before cuting the dependency to Openstack.cloud.

This release include new binaries and Gophercloud binding instead of Openstack.cloud modules.
The Golang binaries has been reorg as well with correct documentation and Ansible-tests.

## v2.0.7

Minor release which removes unused import_image module and fix AEE push

## v2.0.8

- Fix bug when Vm name contains wrong chars

## v2.0.9

- Minor fix to create_instance task for AAP compatibility

## v2.1.0

- Cut dependencies with uncertified collections

## v2.1.2

- Documentation refactoring
- Bug fix for neutron port creation

## v2.1.3

- Fix build script
