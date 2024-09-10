# VMWare to Openstack/Openshift tool kit

This repository is a set of tools, Ansible and Python/Golang based for being able to migrate
virtual machine from an ESXi/Vcenter environment to Openstack or Openshift environment.

The code re-used os-migrate Ansible collection in order to deploy conversion host and setup
correctly the prerequists in the Openstack destination cloud.
It also re-used the vmware community collection in order to gather informations from the source
VMWare environment.

## Usage

Clone repository or install from ansible galaxy

```
git clone https://github.com/os-migrate/vmware-migration-kit
ansible-galaxy install collection os_migrate.vmware_migration_kit
```

Edit secrets.yaml file and add our own setting:

```
esxi_hostname: ********
esxi_username: root
esxi_password: *****
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

If you want to reuse a conversion host already deployed and configured,
otherwise you can let os-migrate do it for you:

```
already_deploy_conversion_host: true
conversion_host_id: "f04deaac-a37c-47f0-a2d2-dbc02e07101e"
conv_host_user: cloud-user
conv_host: "192.168.18.201"
```

Openstack configuration:

```
vm_list:
  - ubuntu-2
vm_name: ubuntu-2
dst_cloud: dst
security_groups: "f1c340e6-2242-4700-b6a4-45f50b65c9bc"
use_existing_flavor: true
flavor_name: m1.xtiny
openstack_private_network: private
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


Then, if you use an already deployed conversion host, you need to edit this file:

```
vmware_migration_kit/localhost_inventory.yml
```

Then run the migration with:

```
pushd vmware_migration_kit
ansible-playbook -i localhost_inventory.yml run_migration.yml -e os_migrate_data_dir=/opt/os-migrate -e @secrets.yaml
```
