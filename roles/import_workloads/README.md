The role import_workloads runs the migration for a given virtual machines from a VMWare environment to an OpenStack environment.
It creates network port, OpenStack instance and rus the migration with nbdkit or virt-v2v. It has also a teardown set of tasks which cleans the OpenStack environment at the end.
