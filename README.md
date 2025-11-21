# VMWare to Openstack tool kit

This repository is a set of tools, Ansible and Python/Golang based for being able to migrate
virtual machine from an ESXi/Vcenter environment to Openstack environment.

The code used OS-Migrate Ansible collection in order to deploy conversion host and setup
correctly the prerequistes in the Openstack destination cloud.
It also used the VMware and VMware_rest collections in order to gather informations from the source
VMware environment.

The Ansible collection provides different steps to scale your migration from VMWare to Openstack:

- A discovery phase where it analyzes the VMware source environment and provides collected data
  to help for the migration.
- A pre-migration phase where it make sure the destionation cloud is ready to perform the migration,
  by creating the conversion host for example or the required network if needed.
- A migration phase with different workflow where the user can basically scale the migration with
  a high number of virtual machines as entry point, or can migrate sensitive virtual machine by using
  a near zero down time with the change block tracking VMware option (CBT) and so perform the virtual
  machine migration in two steps. The migration can also be done without conversion host.

## Full Documentation

For detailed guides, prerequisites, and troubleshooting, please see our [full documentation site](https://os-migrate.github.io/documentation/).

## Features and supported OS

### Features

The following features are availables:

- Discovery mode
- Network mapping
- Port creation and mac addresses mapping
- Openstack flavor mapping and creation
- Migration with nbdkit server with change block tracking feature (CBT)
- Migration with virt-v2v
- Upload migrate volume via Glance
- Multi disks migration
- Multi nics
- Parallel migration on a same conversion host
- Ansible Automation Platform (AAP)
- External shared storage

### Supported OS

The VMware Migration Toolkit uses virt-v2v for conversion. For a list of
supported guest operating systems for virt-v2v, see the Red Hat Knowledgebase article:
[Converting virtual machines from other hypervisors to KVM with virt-v2v in RHEL 7, RHEL 8, RHEL 9, and RHEL 10](https://access.redhat.com/articles/1351473).

RHOSO uses Kernel-based Virtual Machine (KVM) for hypervisors. For a list of certified
guest operating systems for KVM, see the Red Hat Knowledgebase article:
[Certified Guest Operating Systems in Red Hat OpenStack Platform, Red Hat Virtualization, Red Hat OpenShift Virtualization and Red Hat Enterprise Linux with KVM](https://access.redhat.com/articles/certified-hypervisors).

### Migration demo from an AEE

The content of the Ansible Execution Environment can be found here:

[aae-container-file](https://github.com/os-migrate/aap/blob/main/aae-container-file)

And the live demo here:

[![Alt Migration from VMware to OpenStack](https://img.youtube.com/vi/XnEQ8WVGW64/0.jpg)](https://www.youtube.com/watch?v=XnEQ8WVGW64)

### Support

As Red Hat Ansible Certified Content, this collection is entitled to support through Ansible Automation Platform (AAP) using the **Create issue** button on the top right corner.

For any issues related to the supported components of the collection itself, please feel free to raise an [Issue](https://github.com/os-migrate/vmware-migration-kit/issues) on our GitHub repository.

If you installed this collection from Galaxy or GitHub, you can find help from the Ansible community on the [Ansible Forum](https://forum.ansible.com/).

## License

Apache License, Version 2.0

See [LICENCE](https://github.com/os-migrate/vmware-migration-kit/blob/main/LICENSE)
