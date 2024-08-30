# VMWare to Openstack/Openshift tool kit

This repository is a set of tools, Ansible and Python/Golang based for being able to migrate
virtual machine from an ESXi/Vcenter environment to Openstack or Openshift environment.

The code re-used os-migrate Ansible collection in order to deploy conversion host and setup
correctly the prerequists in the Openstack destination cloud.
It also re-used the vmware community collection in order to gather informations from the source
VMWare environment.

## Usage

