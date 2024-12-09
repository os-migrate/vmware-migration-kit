# VMWare to Openstack/Openshift tool kit

This repository is a set of tools, Ansible and Python/Golang based for being able to migrate
virtual machine from an ESXi/Vcenter environment to Openstack or Openshift environment.

The code re-used os-migrate Ansible collection in order to deploy conversion host and setup
correctly the prerequists in the Openstack destination cloud.
It also re-used the vmware community collection in order to gather informations from the source
VMWare environment.

## Workflow

There is different ways to run the migration from VMWare to OpenStack.

* The first one by using virt-v2v binding with a conversion host. Here you can use a conversion
host (Openstack instance) already deployed or you can let OS-Migrate deployed a conversion host
for you.
* The second one by using nbdkit without a conversion host. The platform from where you run
OS-Migrate should have the packages correctly configure for using nbdkit because the migration
will operate on the local host.
Then there is different choices you can make, either upload the converted disk via Glance or
create directly a Cinder volume.
All of these are configurable with Ansible boolean variables.

## Usage

Clone repository or install from ansible galaxy

```
git clone https://github.com/os-migrate/vmware-migration-kit
ansible-galaxy install collection os_migrate.vmware_migration_kit
```

Edit secrets.yaml file and add our own setting:

```
esxi_hostname: ********
vcenter_hostname: *******
vcenter_username: root
vcenter_password: *****
vcenter_datacenter: Datacenter
```

Virt-v2v parameters:

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


If you want to reuse a conversion host already deployed and configured,
otherwise you can let os-migrate do it for you:

```
already_deploy_conversion_host: true
conversion_host_id: "f04deaac-a37c-47f0-a2d2-dbc02e07101e"
```

Openstack configuration parameters

Authentication parameters:
```
os_cloud_environ: psi-rhos-upgrades-ci
dst_cloud:
  auth:
    auth_url: https://openstack.dst.cloud:13000/v3
    username: tenant
    project_id: 3266192cfb2846e9bcb16ceab82bbe65
    project_name: migration
    user_domain_name: osm.com
    password: password
  region_name: regionOne
  interface: public
  identity_api_version: 3
```

And Openstack destination cloud parameters:

```
# OpenStack migration parameters:
# Use mapped networks or not:
used_mapped_networks: false
network_map:
  VM Network: provider_network_1

# If no mapped network then set the openstack network:
openstack_private_network: provider_network_1

# Security groups for the instance:
security_groups: default
use_existing_flavor: true

os_migrate_create_network_port: false
```

If you want to map the network between VMWare and OpenStack:

```
used_mapped_networks: true
# network map
network_map:
  # standard provider network maps linking whatever vmware
  # string to the name of the neutron provider net
  "vmware_network_vlan_100": "openstack_network_vlan_100"
  "vmware_network_vlan_101": "openstack_network_vlan_101"
  "vmware_network_vlan_102": "openstack_network_vlan_102"
  "vmware_network_vlan_103": "openstack_network_vlan_103"
  "vmware_network_vlan_104": "openstack_network_vlan_104"
  # clustered internal sdn network
  # when we migrate the whole vm clusters we need to provide
  # private networks for these guests
  "vmware_internal_netX": "openstack_internal_netX"
  "VM Network": "private"
```


OS-Migration parameters:

```
# osm working directory:
os_migrate_data_dir: /opt/os-migrate

# Set this to true if the Openstack "dst_cloud" is a clouds.yaml file
# other, if the dest_cloud is a dict of authentication parameters, set
# this to false:
copy_openstack_credentials_to_conv_host: false

# Teardown
# Set to true if you want osm to delete everything on the destination cloud.
os_migrate_tear_down: true
```

Then provide the list of the VMs you want to migrate:
```
# VMs list
vms:
  - rhel-1
  - rhel-2
```

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
