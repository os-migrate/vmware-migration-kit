# VMWare to Openstack tool kit

## Description

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


## Requirements

This section list the required minimum versions of Ansible and Python, and any Python or external collection dependencies.
- ansible `>=` 2.15.0
- python `>=` 3.0


## Installation

### Red Hat Customers
Red Hat customers can install this collection from the [Ansible Automation Hub](https://console.redhat.com/ansible/automation-hub/repo/published/os_migrate/vmware_migration_kit/?sort=-pulp_created).


### Community Users
Community users can install this collection from Ansible Galaxy or GitHub. For detailed installation and usage instructions, refer to the [VMware to OpenStack Guide](https://os-migrate.github.io/documentation/#os-migrate-vmware-guide_vmware).

To install from Galaxy:

```
ansible-galaxy collection install os_migrate.vmware_migration_kit
```

To install, use the Ansible Galaxy command-line tool:

```
ansible-galaxy collection install os_migrate.vmware_migration_kit
```

You can also include it in a `requirements.yml` file and install it with `ansible-galaxy collection install -r requirements.yml`, using the format:

```yaml
collections:
  - name: os_migrate.vmware_migration_kit
```

To upgrade the collection to the latest available version, run:

```
ansible-galaxy collection install os_migrate.vmware_migration_kit --upgrade
```

You can also install a specific version of the collection. Use the following syntax to install version 1.0.0:

```
ansible-galaxy collection install os_migrate.vmware_migration_kit:==1.0.0
```

To install from GitHub:

```
git clone https://github.com/os-migrate/vmware-migration-kit
```

See [using Ansible collections](https://docs.ansible.com/ansible/devel/user_guide/collections_using.html) for more details.

## Use Cases

There are different ways to run the migration from VMware to OpenStack.

- The default is by using nbdkit server with a conversion host (an Openstack instance hosted in the destination cloud).
  This way allows the user to use the CBT option and approach a zero downtime. It can also run the migration in one time cycle.
- The second one by using virt-v2v binding with a conversion host. Here you can use a conversion
  host (Openstack instance) already deployed or you can let OS-Migrate deployed a conversion host
  for you.
- A third way is available where you can skip the conversion host and perform the migration on a Linux machine, the volume
  migrated and converted will be upload a Glance image or can be use later as a Cinder volume. This way is not recommended if
  you have big disk or a huge amount of VMs to migrate: the performance is really slower than with the other ways.

All of these are configurable with Ansible boolean variables.


## Testing

The VMware Migration Toolkit uses virt-v2v for conversion. For a list of
supported guest operating systems for virt-v2v, see the Red Hat Knowledgebase article:
Converting virtual machines from other hypervisors to KVM with virt-v2v in RHEL 7, RHEL 8, RHEL 9, and RHEL 10.

RHOSO uses Kernel-based Virtual Machine (KVM) for hypervisors. For a list of certified
guest operating systems for KVM, see the Red Hat Knowledgebase article:
Certified Guest Operating Systems in Red Hat OpenStack Platform, Red Hat Virtualization, Red Hat OpenShift Virtualization and Red Hat Enterprise Linux with KVM.


## Support

This collection is maintained by the Red Hat OpenStack Migration team.

By ensuring correct connectivity, installation, user ACLs, and host setup, most migration issues can be avoided.

### Customer Support

As Red Hat Ansible Certified Content, this collection is entitled to support through the Ansible Automation Platform (AAP) using the **Create issue** button on the top right corner of the [Automation Hub](https://console.redhat.com/ansible/automation-hub/).

### Community Support

If a support case cannot be opened with Red Hat and the collection has been obtained either from Galaxy or GitHub, community help is available through:

- **GitHub Issues**: Open a bug report or feature request at [https://github.com/os-migrate/vmware-migration-kit/issues](https://github.com/os-migrate/vmware-migration-kit/issues)
- **Ansible Forum**: Get community assistance at [https://forum.ansible.com/](https://forum.ansible.com/)


## Release Notes and Roadmap

### Red Hat Customers

For Red Hat customers using this collection through Ansible Automation Hub, release information and distributions are available at:
[https://console.redhat.com/ansible/automation-hub/repo/published/os_migrate/vmware_migration_kit/distributions/](https://console.redhat.com/ansible/automation-hub/repo/published/os_migrate/vmware_migration_kit/distributions/)

### Community Users

For community users who obtained this collection from Galaxy or GitHub, changelog information is available at:
[https://github.com/os-migrate/vmware-migration-kit/blob/main/CHANGELOG.md](https://github.com/os-migrate/vmware-migration-kit/blob/main/CHANGELOG.md)

## Related Information

For detailed guides, prerequisites, and troubleshooting, please see our docs https://os-migrate.github.io/documentation/.


## License Information

Apache License, Version 2.0, https://www.apache.org/licenses/LICENSE-2.0.txt
