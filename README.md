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

## Table of Contents
- [Workflow](#workflow)
- [Features and supported OS](#features-and-supported-os)
  - [Features](#features)
  - [Supported OS](#supported-os)
  - [Nbdkit migration example](#nbdkit-migration-example)
  - [Nbdkit migration example with the Change Block Tracking](#nbdkit-migration-example-with-the-change-block-tracking)
  - [Migration demo from an AEE](#migration-demo-from-an-aee)
  - [Running migration](#running-migration)
    - [Conversion host setup](#conversion-host-setup)
    - [VMWare VDDK Setup](#vmware-vddk-setup)
    - [Inventory, Variables files and Ansible command](#inventory-variables-files-and-ansible-command)
    - [OpenStack flavors](#openstack-flavor)
    - [Using Change Block Tracking (CBT)](#using-change-block-tracking-cbt)
- [VMware ACLs requirements](#vmware-acls-requirements)
- [Network requirements](#network-requirements)
  - [Required ports](#required-ports)
- [Usage](#usage)
  - [Nbdkit (default)](#nbdkit-default)
  - [Virt-v2v](#virt-v2v)
  - [Running migration from local shared NFS](#running-migration-from-local-shared-nfs)
  - [Ansible configuration](#ansible-configuration)
  - [Running Migration outside of Ansible](#running-migration-outside-of-ansible)
  - [Enable Debugging Flags During Migration](#enable-debugging-flags-during-migration)
- [Troubleshooting](#troubleshooting)
  - [Connectivity from Conversion Host to VMware](#connectivity-from-conversion-host-to-vmware)
  - [Collection Installation](#collection-installation)
  - [vCenter User Permissions](#vcenter-user-permissions)
  - [Guest Requirements](#guest-requirements)
  - [Conversion Host requirements](#conversion-host-requirements)
  - [Debugging tips](#debugging-tips)
- [Support](#support)
- [Licence](#licence)

## Workflow

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

Currently we are supporting the following matrice:

| OS Family      | Version       | Supported & Tested | Not Tested Yet |
| -------------- | ------------- | ------------------ | -------------- |
| RHEL           | 9.4           | Yes                | -              |
| RHEL           | 9.3 and lower | Yes                | -              |
| RHEL           | 8.5           | Yes                | -              |
| RHEL           | 8.4 and lower | -                  | Yes            |
| CentOS         | 9             | Yes                | -              |
| CentOS         | 8             | Yes                | -              |
| Fedora         | 38 and upper  | Yes                | -              |
| Fedora (btrfs) | 38 and upper  | Yes                | -              |
| Ubuntu Server  | 24            | Yes                | -              |
| Windows        | 10            | Yes                | -              |
| Windows Server | 2k22          | Yes                | -              |
| Suse           | X             | -                  | Yes            |

### Nbdkit migration example

![Alt Nbdkit](doc/osm-migration-nbdkit-vmware-workflow-with-osm.drawio.svg)

### Nbdkit migration example with the Change Block Tracking

#### Step 1: The data are copied and the change ID from the VMware disk are set to the Cinder volume as metadata

> **Note:** The conversion cannot be made at this moment, and the OS instance is not created.
> This functionality can be used for large disks with a lot of data to transfer. It helps avoid a prolonged service interruption.

![Alt CBT Step 1](doc/osm-migration-nbdkit-vmware-workflow-with-osm_cbt_step1.svg)

#### Step 2: OSM compare the source (VMware disk) and the destination (Openstack Volume) change ID

> **Note:** If the change IDs are not equal, the changed blocks between the source and destination are synced.
> Then, the conversion to libvirt/KVM is triggered, and the OpenStack instance is created.
> This allows for minimal downtime for the VMs.

![Alt CBT Step 2](doc/osm-migration-nbdkit-vmware-workflow-with-osm_cbt_step2.svg)

### Migration demo from an AEE

The content of the Ansible Execution Environment can be found here:

<https://github.com/os-migrate/aap/blob/main/aae-container-file>

And the live demo here:

[![Alt Migration from VMware to OpenStack](https://img.youtube.com/vi/XnEQ8WVGW64/0.jpg)](https://www.youtube.com/watch?v=XnEQ8WVGW64)

### Running migration

#### Conversion host setup

You can use os_migrate.os_migration collection to deploy a conversion, but you can
easily create your conversion host manually.

A conversion host is basically an OpenStack instance.

The minimal requirements recommended for the conversion host settings (OpenStack flavor) for being able run from 1 to 2 migrations simultaneously are: 2 vcpus, 4Gb of ram and 16 Gb of disk.

OS-Migrate supports parallel execution against the same conversion host, the more you increase the capacity of the conversion host (ram and vcpus) the more you can run migration in parallel.

> **Note:** Consider as requirements rule allocating 1 vcpu and 2 GB of RAM per migrations

> **Note:** Important: If you want to take benefit of the current supported OS, it's highly recommended to use a _CentOS-10_ release or _RHEL-9.5_ and superior. If you want to use other Linux distribution, make sure the virtio-win package is equal or higher than 1.40 version.

> **Note:** Important: For btrfs file system support, since the RHEL and CentOS kernel don't support btrfs in the recent releases, you can use Fedora as the base OS of your conversion host. The btrfs file system is supported with Fedora conversion host.

```
curl -O -k https://cloud.centos.org/centos/10-stream/x86_64/images/CentOS-Stream-GenericCloud-10-20250217.0.x86_64.qcow2

# Create OpenStack image:
openstack image create --disk-format qcow2 --file CentOS-Stream-GenericCloud-10-20250217.0.x86_64.qcow2 CentOS-Stream-GenericCloud-10-20250217.0.x86_64.qcow2

# Create flavor, security group and network if needed
openstack server create --flavor x.medium --image 14b1a895-5003-4396-888e-1fa55cd4adf8  \
  --key-name default --network private   vmware-conv-host
openstack server add floating ip vmware-conv-host 192.168.18.205
```

#### VMware VDDK setup

##### Download and extract the VMware VDDK

1. In a browser, navigate to the [VMware VDDK version 8 download page](https://developer.vmware.com/web/dp/tool/vddk).
2. Select **version 8.0.1** and click **Download**.

> **Note:**
> We support migration from **version 8.0.0** and later

3. Save the VDDK archive file in a temporary directory.
4. Extract the VDDK archive:

   ```bash
   tar -xzf VMware-vix-disklib-<version>.x86_64.tar.gz

5. You can now specified the lib path via the variable `conversion_host_vmware_vix_disklib`. By default
the lib path point to: /usr/lib/vmware-vix-disklib

> **Note:**
> If you want to skip the conversion_host role, you can use the variable: `import_workloads_libdir` instead.

#### Inventory, Variables files and Ansible command

**inventory.yml**

```
migrator:
  hosts:
    localhost:
      ansible_connection: local
      ansible_python_interpreter: "{{ ansible_playbook_python }}"
conversion_host:
  hosts:
    192.168.18.205:
      ansible_ssh_user: cloud-user
      ansible_ssh_private_key_file: key
```

**myvars.yml:**

```
# if you run the migration from an Ansible Execution Environment (AEE)
# set this to true:
runner_from_aee: true

# osm working directory:
os_migrate_vmw_data_dir: /opt/os-migrate
copy_openstack_credentials_to_conv_host: false

# Re-use an already deployed conversion host:
already_deploy_conversion_host: true

# If no mapped network then set the openstack network:
openstack_private_network: 81cc01d2-5e47-4fad-b387-32686ec71fa4

# Security groups for the instance:
security_groups: ab7e2b1a-b9d3-4d31-9d2a-bab63f823243
use_existing_flavor: true
# key pair name, could be left blank
ssh_key_name: default
# network settings for openstack:
os_migrate_create_network_port: true
copy_metadata_to_conv_host: true
used_mapped_networks: false

vms_list:
  - rhel-9.4-1
```

**secrets.yml:**

```
# VMware parameters:
esxi_hostname: 10.0.0.7
vcenter_hostname: 10.0.0.7
vcenter_username: root
vcenter_password: root
vcenter_datacenter: Datacenter

os_cloud_environ: psi-rhos-upgrades-ci
dst_cloud:
  auth:
    auth_url: https://keystone-public-openstack.apps.ocp-4-16.standalone
    username: admin
    project_id: xyz
    project_name: admin
    user_domain_name: Default
    password: openstack
  region_name: regionOne
  interface: public
  insecure: true
  identity_api_version: 3
```

**Ansible command:**

```
ansible-playbook -i inventory.yml os_migrate.vmware_migration_kit.migration -e @secrets.yml -e @myvars.yml
```

#### OpenStack Flavor

When using **VMware** as a source in **OS-Migrate**, there are several ways to handle the **flavor** for the resulting OpenStack instance.
Since VMware does not have a native flavor mechanism like OpenStack, OS-Migrate provides three approaches to manage this part:

1. **Find the closest matching flavor**
   OS-Migrate will try to find the best matching flavor that fits the VMs specifications.
   This can be enabled using the option:
   ```yaml
   use_existing_flavor: true
   ```
   If no flavor matches the VMs specs, OS-Migrate will automatically create a new one.

2. **Create a new flavor for the VM**
   OS-Migrate can create a dedicated flavor for each VM.
   The flavor name will follow this pattern:
   ```
   osm-vmware-<vm_name>-<random_id>
   ```
   Example: `osm-vmware-myvm-9999`

3. **Provide a specific flavor UUID**
   You can also specify an existing flavor by providing its UUID:
   ```yaml
   flavor_uuid: <your_flavor_uuid>
   ```
   This option allows you to define custom properties on the flavor, which can be useful for **host aggregation** or **targeted placement** of instances on specific compute hosts.

#### Using Change Block Tracking (CBT)

The Change Block Tracking (CBT) option allows you to pre-synchronize the disk data and then synchronize only the blocks that have changed between the last execution and the current state of the disk.
OS-Migrate records the CBT ID as metadata in the Cinder volume. With this ID, OS-Migrate can copy only the changed blocks.

```
openstack volume show rhel-9 | grep properties
| properties                     | changeID='52 64 3d 2e 44 c3 62 2f-d2 4a e9 fd 82 39 54 85/138', converted='false', osm='true' |
```

In this example, we can see the `changeID` property along with two additional pieces of information:
  - `converted=false` means that the cutover has not yet been performed.
  - `osm=true` means that OS-Migrate is managing this volume.

By default, OS-Migrate migrates the entire virtual machine (disk and metadata) in one step and creates the OpenStack instance. To enable CBT-based synchronization, you must specify this option:

```
import_workloads_cbt_sync: true
```


Then run the migration playbook, and OS-Migrate will copy the data from your VMware guest to the OpenStack Cinder volume.

When you are ready to perform the final cutover and migrate the virtual machine, set this option to `true`:

```
import_workloads_cutover: true
```

OS-Migrate will then synchronize the disk between the latest and current change IDs and convert the disk to run under KVM.
After that, the normal workflow continues: the OpenStack instance is created from the Cinder volume with the network options you specified.

## VMware ACLs requirements

To avoid to use the Administrator role and in order to be able to connect, parse the Vcenter datastore, manipulate the snapshots and migrate virtual machines, OS-Migrate needs the following ACLs for the Vcenter user:

| Category         | Privilege Group         | Privileges                                                                 |
|------------------|-------------------------|----------------------------------------------------------------------------|
| **Datastore**    | —                       | Browse datastore                                                           |
| **Virtual Machine** | Guest operations       | All                                                                        |
|                  | Provisioning            | Allow disk access<br>Allow file access<br>Allow read-only disk access<br>Allow virtual machine download |
|                  | Service configuration   | Allow notifications<br>Allow polling of global event notifications<br>Read service configuration |
|                  | Snapshot management     | Create snapshot<br>Remove snapshot<br>Rename snapshot<br>Revert to snapshot |

## Network requirements

### Required ports

The following table outlines the network ports required for the migration process from the perspective of the conversoin host.

| Port / Protocol | Direction | Source / Destination | Purpose |
| :--- | :--- | :--- | :--- |
| 443/TCP | Egress | VMware vCenter | **Main VMware Communication:** Used for [authentication](https://github.com/os-migrate/vmware-migration-kit/blob/main/plugins/module_utils/vmware/vmware.go#L66), VM metadata retrieval, snapshots, and VDDK operations. |
| 902/TCP | Egress | VMware ESXi Hosts | **Direct Disk Access:** Used to read a VM's disk data directly from ESXi hosts via the [NFC/NBD](https://ports.broadcom.com/home/vSphere) protocols. |
| [See Docs](https://docs.redhat.com/it/documentation/red_hat_openstack_services_on_openshift/18.0/html/firewall_rules_for_red_hat_openstack_services_on_openshift/firewall-rules) | Egress | OpenStack Destination Cloud | **Destination Cloud Control:** Used for communicating with OpenStack APIs to provision the new VM and transfer its data. |
| 22/TCP | Ingress | Ansible Controller / Admin | **Remote Management:** Allows for secure troubleshooting and management of the conversion host. |
| 10809/TCP | Internal | Conversion Host | **Local Disk Data Transfer:** Used by the internal [NBDKit server](https://github.com/os-migrate/vmware-migration-kit/blob/main/plugins/module_utils/nbdkit/nbdkit.go#L59) to stream disk data for conversion.  *Does not require a network firewall rule.* |

#### Description

The migration process relies on three primary external communication channels: one to connect to the source VMware environment, one to connect to the destination OpenStack cloud, and one for Ansible to manage the conversion host.

First, the migration tool must make two key outbound connections to your VMware environment. It uses port 443 as the mandatory, secure channel to the vCenter server for management commands like logging in and taking snapshots. It then also connects to the ESXi hosts directly on port 902, which is the data channel used to read and transfer the virtual machine's disk data.

Second, the tool requires outbound connections to the destination OpenStack cloud. These are used to communicate with various OpenStack APIs for provisioning the new virtual machine and transferring the converted disk data. The full list of required destination ports is extensive and can be found in the [official documentation](https://docs.redhat.com/it/documentation/red_hat_openstack_services_on_openshift/18.0/html/firewall_rules_for_red_hat_openstack_services_on_openshift/firewall-rules).

Third, an inbound connection on port 22 (SSH) is a prerequisite. This channel is required for the Ansible controller to connect to the conversion host to perform setup, configuration, and other automation tasks.

Additionally, the tool uses port 10809 purely for an internal process on the machine itself to help convert the disk data. This doesn't require any network firewall changes.

## Usage

You can find a "how to" here, to start from scratch with a container:
<https://gist.github.com/matbu/003c300fd99ebfbf383729c249e9956f>

Clone repository or install from ansible galaxy

```
git clone https://github.com/os-migrate/vmware-migration-kit
ansible-galaxy collection install os_migrate.vmware_migration_kit
```

### Nbdkit (default)

Edit vars.yaml file and add our own setting:

```
esxi_hostname: ********
vcenter_hostname: *******
vcenter_username: root
vcenter_password: *****
vcenter_datacenter: Datacenter
```

If you already have a conversion host, or if you want to re-used a previously deployed one:

```
already_deploy_conversion_host: true
```

Then specify the Openstack credentials:

```
# OpenStack destination cloud auth parameters:
dst_cloud:
  auth:
    auth_url: https://openstack.dst.cloud:13000/v3
    username: tenant
    project_id: xyz
    project_name: migration
    user_domain_name: osm.com
    password: password
  region_name: regionOne
  interface: public
  identity_api_version: 3

# OpenStack migration parameters:
# Use mapped networks or not:
used_mapped_networks: true
network_map:
  VM Network: private

# If no mapped network then set the openstack network:
openstack_private_network: 81cc01d2-5e47-4fad-b387-32686ec71fa4

# Security groups for the instance:
security_groups: 4f077e64-bdf6-4d2a-9f2c-c5588f4948ce
use_existing_flavor: true

os_migrate_create_network_port: false

# OS-migrate parameters:
# osm working directory:
os_migrate_vmw_data_dir: /opt/os-migrate

# Set this to true if the Openstack "dst_cloud" is a clouds.yaml file
# other, if the dest_cloud is a dict of authentication parameters, set
# this to false:
copy_openstack_credentials_to_conv_host: false

# Teardown
# Set to true if you want osm to delete everything on the destination cloud.
os_migrate_tear_down: true

# VMs list
vms_list:
  - rhel-1
  - rhel-2
```

### Virt-v2v

Provide the following additional informations:

```
# virt-v2v parameters
vddk_thumbprint: XX:XX:XX
vddk_libdir: /usr/lib/vmware-vix-disklib
```

In order to generate the thumbprint of your VMWare source cloud you need to use:

```
# thumbprint
openssl s_client -connect ESXI_SERVER_NAME:443 </dev/null |
   openssl x509 -in /dev/stdin -fingerprint -sha1 -noout
```

### Running migration from local shared NFS

OS-Migrate can migrate directly from a local shared directory mounted on the conversion host.
If the VMware virtual machines are located on an NFS datastore that is accessible to the conversion host, you can mount the NFS storage on the conversion host and provide the path to the NFS mount point.

OS-Migrate will then directly consume the disks of the virtual machines located on the NFS mount point.
Configure the Ansible variable to specify your mount point as follows:

```
import_workloads_local_disk_path: "/srv/nfs"
```

> **Note:** In this mode, only cold migration is supported.

### Ansible configuration

Create an inventory file, and replace the conv_host_ip by the ip address of your
conversion host:

```
migrator:
  hosts:
    localhost:
      ansible_connection: local
      ansible_python_interpreter: "{{ ansible_playbook_python }}"
conversion_host:
  hosts:
    conv_host_ip:
      ansible_ssh_user: cloud-user
      ansible_ssh_private_key_file: /home/stack/.ssh/conv-host

```

Then run the migration with:

```
ansible-playbook -i localhost_inventory.yml os_migrate.vmware_migration_kit.migration -e @vars.yaml
```

### Running Migration outside of Ansible

You can also run migration outside of Ansible because the Ansible module are written in Golang.
The binaries are located in the plugins directory.

From your conversion host (or an Openstack instance inside the destination cloud) you need to export
Openstack variables:

```
 export OS_AUTH_URL=https://keystone-public-openstack.apps.ocp-4-16.standalone
 export OS_PROJECT_NAME=admin
 export OS_PASSWORD=admin
 export OS_USERNAME=admin
 export OS_DOMAIN_NAME=Default
 export OS_PROJECT_ID=xyz
```

Then create the argument json file, for example:

```
cat <<EOF > args.json
{
  "user": "root",
  "password": "root",
  "server": "10.0.0.7",
  "vmname": "rhel-9.4-3",
  "cbtsync": false,
  "dst_cloud": {
   "auth": {
    "auth_url": "https://keystone-public-openstack.apps.ocp-4-16.standalone",
    "username": "admin",
    "project_id": "xyz",
    "project_name": "admin",
    "user_domain_name": "Default",
    "password": "admin"
   },
   "region_name": "regionOne",
   "interface": "public",
   "identity_api_version": 3
  }
}
EOF
```

Then execute the `migrate` binary:

```
pushd vmware-migration-kit/vmware_migration_kit
./plugins/modules/migrate/migrate
```

You can see the logs into:

```
tail -f /tmp/osm-nbdkit.log
```

### Enable Debugging Flags During Migration

OS-Migrate creates a unique log file for each migration on the conversion host.
This file is located in `/tmp` on the conversion host and, in case of failure, is pulled to the Ansible localhost in the OS-Migrate working directory (by default `/opt/os-migrate`), under a folder named after the virtual machine, for example:

```
/opt/os-migration/rhel-9.4/migration.log
```

On the conversion host, the log file follows this naming format:
`osm-nbdkit-<vm-name>-<random-id>.log` where `<random-id>` is the same as the random ID used for the process ID (PID):

```
tail -f /tmp/osm-nbdkit-rhel-9.4-28vL39tB.log
```

When a failure occurs, especially during the conversion, it can be very useful to enable debug flags to increase verbosity and capture detailed debugging information.
This can be done by setting:

```
import_workloads_debug: true
```

## Troubleshooting

### Connectivity from Conversion Host to VMware

Ensure network and name resolution are properly configured before running migrations.

- **Port 902** (used for vSphere data transfer) must be reachable from the conversion host:
  ```bash
  curl -v telnet://<vcenter_ip>:902
  # or
  nc -zv <vcenter_ip> 902
  ```
  The connection should succeed.

- **vCenter FQDN resolution**
  The vCenter hostname must be resolvable by the conversion host.
  If necessary, update `/etc/hosts`:
  ```bash
  echo "<vcenter_ip> vcenter.domain.local" | sudo tee -a /etc/hosts
  ```

  Example of a related error message:
  ```
  Host address lookup for server wmdinfesx907.mas.gov.sg failed: Name or service not known
  ```

- **OpenStack API resolution**
  Similarly, ensure the OpenStack API endpoint is resolvable, using `/etc/hosts` if needed.


### Collection Installation

Proper installation of the Ansible collection and supporting binaries is critical for successful migrations.

- **Installation source**
  - Always install from a tagged release version (e.g. `v2.0.9`) instead of `latest`.
  - When using **Ansible Automation Platform / AEE**, prefer installation via Automation Hub using the release tag.

- **Binaries**
  - Ensure that the binaries (e.g. `vmware-migration` tools) are present in the collection path.
  - If the collection was installed using `ansible-galaxy collection install` or via a Git clone, verify binaries are included.
    In AEE, binaries should already be present.

- **Architecture mismatch**
  - If you encounter:
    ```
    AnsiballZ_migrate: cannot execute binary file: Exec format error
    ```
    It indicates an architecture mismatch.
    Check the binary type:
    ```bash
    file <binary>
    ```
    Ensure it matches your conversion host architecture (`x86_64`, `arm64`, etc.).


### vCenter User Permissions

Ensure that the VMware user used for migration has the correct privileges.

- Refer to the ACL requirements documented here:
  [VMware ACLs Requirements](https://github.com/os-migrate/vmware-migration-kit?tab=readme-ov-file#vmware-acls-requirements)

- **User selection rule:**
  - If the migration target IP corresponds to a **vCenter**, use a **vCenter user**.
  - If the target IP corresponds to a **standalone ESXi host**, use an **ESXi user**.

### Guest Requirements

Before migration, ensure guest configuration is compatible with the conversion process.

- **Persistent disks**
  - All VM disks must be configured as *persistent* (default for most VMware disks).

- **Consolidate disks**
  - The disks should in a correct state: no alarms and consolidated.

- **Supported filesystems**
  - The conversion host must support the filesystem used by the guest OS.
  - If not, use an alternative conversion host that supports it (e.g., use **Fedora** for `btrfs`).


### Conversion Host Requirements

The conversion host is responsible for reading VMware disks and converting them to OpenStack volumes.

- **Dependencies**
  - Ensure `vmware-vix-disklib-8.0.0` is installed and properly configured.

- **Metadata service**
  - Verify that the OpenStack metadata service is reachable.
    Typical error when not reachable:
    ```
    Failed to fetch metadata: Get "http://169.254.169.254/openstack/latest/meta_data.json": dial tcp 169.254.169.254:80: connect: no route to host
    ```

  - **Workaround:**
    Use a manual instance UUID in the import playbook:
    ```yaml
    import_workloads_instance_uuid: <uuid>
    ```


### Debugging Tips

#### NBDKit Errors

If you encounter an error such as:
```
nbdkit: error: server has no export named '': No such file or directory
```

This error can have multiple root causes. Start by reviewing the Red Hat Knowledge Base article:
[https://access.redhat.com/solutions/7127535](https://access.redhat.com/solutions/7127535)

Common causes include:
1. Port 902 not open between conversion host and vCenter.
2. vCenter FQDN not resolvable.
3. Malformed `nbdkit` command (invalid characters or parameters).


#### Manual Debug Procedure

In the `migrate.log`, you will find both the `nbdkit` and `nbdcopy` commands used during migration.
You can replay these manually for troubleshooting.

##### Step 1 – Run `nbdkit` manually
- Run the command displayed in the logs with:
  - `--verbose` option.
  - Double quotes around the VMDK path.
  ```bash
  nbdkit --verbose vddk ".../guest-00001.vmdk"
  ```

- If the migration snapshot has been deleted, remove the snapshot option in the command.
  `nbdkit` will then attempt to consume the base disk directly.

##### Step 2 – Run `nbdcopy` in another shell
- Open a second terminal and run the `nbdcopy` command as shown in the logs.
- Observe the output from `nbdkit`. You should see a line ending with:
  ```
  vddk: config_complete.
  ```

##### Step 3 – Analyze authentication & paths
- At this point, authentication is **already verified** by the migration process.
- The VMDK path should be valid and provided by VMware API itself:
  ```
  [Datastore 1] path/to/the/guest-00001.vmdk
  ```

##### Step 4 – Migration log insights
- The log entry:
  ```
  volume not found
  ```
  is informational.
  It indicates that the destination volume (Cinder) does not exist yet and will be created during migration.

- If the volume already exists:
  - Set `import_workloads_cbt_sync: true` to perform delta synchronization.
  - If `import_workloads_cutover: false`, the migration will only sync changes.
  - If `import_workloads_cutover: true`, the final conversion will be executed.

### Summary

By ensuring correct connectivity, installation, user ACLs, and host setup, most migration issues can be avoided.
For persistent or unclear errors, reproduce the NBDKit and NBDCopy commands manually as described above to isolate the issue.

## Support

### Scope of Support

The @os-migrate team provides full support for the components included directly within this collection (all playbooks, roles, and plugins) as well as for the os_migrate.os_migrate collection, which is also developed by our team.

### External Dependencies

Our support policy does not extend to external or third-party dependencies. If an issue is found to be caused by a bug within one of these external components, we are unable to provide a resolution. We want to assure you that the collection itself has been thoroughly tested to ensure its stability and functionality.

### Versioning

Please note that support is provided exclusively for the latest version of the collection available in the repository.

### How to Get Support

For any issues related to the supported components of the collection itself, please feel free to raise an [Issue](https://github.com/os-migrate/vmware-migration-kit/issues) on our GitHub repository.

The upstream documentation is available [here](https://github.com/os-migrate/documentation). For Red Hat Certified and Supported documentation, please see [Red Hat OpenStack VMWare Migration Toolkit](https://console.redhat.com/ansible/automation-hub/repo/published/os_migrate/vmware_migration_kit/docs/).

## License

Apache License, Version 2.0

See [COPYING](https://www.apache.org/licenses/LICENSE-2.0.txt)
