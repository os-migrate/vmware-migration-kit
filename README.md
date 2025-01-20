# VMWare to Openstack/Openshift tool kit

This repository is a set of tools, Ansible and Python/Golang based for being able to migrate
virtual machine from an ESXi/Vcenter environment to Openstack or Openshift environment.

The code used os-migrate Ansible collection in order to deploy conversion host and setup
correctly the prerequists in the Openstack destination cloud.
It also used the vmware community collection in order to gather informations from the source
VMWare environment.

The Ansible collection provides different steps to scale your migration from VMWare to Openstack
and Openshift:

* A discovery phase where it analizes the VMWare source environment and provides collected data
to help for the migration.
* A pre-migration phase where it make sure the destionation cloud is ready to perform the migration,
by creating the conversion host for example or the required network if needed.
* A migration phase with different workflow where the user can basicaly scale the migration with
a very number of virtual machines as entry point, or can migrate sensitive virtual machine by using
a near zero down time with the change block tracking VMWare option (CBT) and so perform the virtual
machine migration in two steps. The migration can also be done without conversion host.

## Workflow

There is different ways to run the migration from VMWare to OpenStack.

* The default is by using nbdkit server with a conversion host (an Openstack instance hosted in the destination cloud).
This way allow the user to use the CBT option and approach a zero downtime. It can also run the migration in one time cycle.
* The second one by using virt-v2v binding with a conversion host. Here you can use a conversion
host (Openstack instance) already deployed or you can let OS-Migrate deployed a conversion host
for you.
* A third way is available where you can skip the conversion host and perform the migration on a Linux machine, the volume
migrated and converted will be upload a Glance image or can be use later as a Cinder volume. This way is not recommended if
you have big disk or a huge amount of VMs to migrate: the performance are really slower than with the other ways.

All of these are configurable with Ansible boolean variables.


## Features and supported OS

### Features

The following features are availables:

* Discovery mode
* Network mapping
* Port creation and mac addresses mapping
* Openstack flavor mapping and creation
* Migration with nbdkit server with change block tracking feature (CBT)
* Migration with virt-v2v
* Upload migrate volume via Glance
* Multi disks migration
* Ansible Automation Platform (AAP)


### Supported OS

Currently we are supporting the following matrice:

| OS Family       | Version         | Supported & Tested | Not Tested Yet |
|-----------------|-----------------|--------------------|----------------|
| RHEL            | 9.4            | Yes                | -              |
| RHEL            | 9.3 and lower  | Yes                | -              |
| RHEL            | 8.5            | Yes                | -              |
| RHEL            | 8.4 and lower  | -                  | Yes            |
| CentOS          | 9              | Yes                | -              |
| CentOS          | 8              | Yes                | -              |
| Ubuntu Server   | -              | Yes                | -              |
| Windows         | 10             | Yes                | -              |
| Windows Server  | X              | -                  | Yes            |
| Suse            | X              | -                  | Yes            |

## Usage

You can find a "how to" here, to start from sratch with a container:
https://gist.github.com/matbu/003c300fd99ebfbf383729c249e9956f 

Clone repository or install from ansible galaxy

```
git clone https://github.com/os-migrate/vmware-migration-kit
ansible-galaxy install collection os_migrate.vmware_migration_kit
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
vddk_libdir: /usr/lib/vmware-vix-disklib
already_deploy_conversion_host: true
```

Then specify the Openstack credentials:

```
# OpenStack destination cloud auth parameters:
os_cloud_environ: psi-rhos-upgrades-ci
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
used_mapped_networks: false
network_map:
  VM Network: provider_network_1

# If no mapped network then set the openstack network:
openstack_private_network: provider_network_1

# Security groups for the instance:
security_groups: 4f077e64-bdf6-4d2a-9f2c-c5588f4948ce
use_existing_flavor: true

os_migrate_create_network_port: false

# OS-migrate parameters:
# osm working directory:
os_migrate_data_dir: /opt/os-migrate

# Set this to true if the Openstack "dst_cloud" is a clouds.yaml file
# other, if the dest_cloud is a dict of authentication parameters, set
# this to false:
copy_openstack_credentials_to_conv_host: false

# Teardown
# Set to true if you want osm to delete everything on the destination cloud.
os_migrate_tear_down: true

# VMs list
vms_lisr:
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

### Ansible configuration

Create an invenvoty file, and replace the conv_host_ip by the ip address of your
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
pushd vmware_migration_kit
ansible-playbook -i localhost_inventory.yml migration.yml -e @vars.yaml
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
  "vmname": "rhel-9.4-1"
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