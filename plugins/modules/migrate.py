DOCUMENTATION = r"""
module: migrate
short_description: Migrates a VMware virtual machine to OpenStack.
description:
  - This module orchestrates the migration of a specified VMware virtual machine (C(vmname)) to OpenStack.
  - It requires connection details for both the source vSphere environment (C(server), C(user), C(password))
    and the destination OpenStack cloud (C(dst_cloud)).
  - The module handles disk discovery and data transfer, potentially using VMware VDDK (path specified by C(vddkpath)).
  - It can associate the migrated VM with an existing OpenStack instance UUID (C(instanceuuid)) or manage it as a new entity.
  - Options for CBT sync, SOCKS proxy, compression, and a first boot script are available.
author: "OpenStack tenant migration tools (@os-migrate)"
version_added: "1.0.0"
options:
  dst_cloud:
    description:
      - A dictionary containing authentication and connection parameters for the destination OpenStack cloud.
      - This should include details like C(auth_url), C(username), C(password), C(project_name), C(user_domain_name),
        C(project_domain_name), C(region_name), etc., or a C(cloud) key to use a clouds.yaml profile.
    type: dict
    required: true
  user:
    description: Username for authenticating with the vSphere server (vCenter or ESXi).
    type: str
    required: true
  password:
    description: Password for authenticating with the vSphere server.
    type: str
    required: true
  server:
    description: Hostname or IP address of the vSphere server (vCenter or ESXi).
    type: str
    required: true
  vmname:
    description: The name of the source VMware virtual machine to be migrated.
    type: str
    required: true
  osmdatadir:
    description: Path to the os-migrate data directory, used for storing migration-related data, logs, or state.
    type: str
    required: true
  firstboot:
    description:
      - Path to a script file that will be configured to run on the first boot of the migrated virtual machine in OpenStack.
      - This is typically used for guest OS customization (e.g., network configuration via cloud-init).
    type: str
    required: false
  vddkpath:
    description: Path to the VMware VDDK (Virtual Disk Development Kit) installation directory.
    type: str
    required: true # Assuming VDDK is the primary mechanism if NBD params are not exposed here
  usesocks:
    description: If C(true), a SOCKS proxy (typically configured via environment variables) will be used for relevant network connections.
    type: bool
    required: false
    default: false
  cbtsync:
    description: If C(true), attempts to use VMware's Changed Block Tracking (CBT) for the synchronization/migration process.
    type: bool
    required: false
    default: false
  instanceuuid:
    description:
      - UUID of an OpenStack instance. This can be the UUID of an existing placeholder instance to which the migrated disks/VM should
        be associated, or it might be used as a reference for the newly created OpenStack VM.
    type: str
    required: true # Based on its presence in the focused example
  convhostname:
    description:
      - Optional. Hostname or IP address of a specific conversion host.
      - This might be used if the migration process involves a helper VM or a specific host for conversion tasks orchestrated by the module.
    type: str
    required: false
  compression: # Added from your Go struct list
    description:
      - Specifies the compression method to be used during data transfer (e.g., C(none), C(zstd), C(gzip)), if supported.
    type: str
    required: false
  debug_mode: # Added from your Go struct list, common helpful param
    description: If C(true), enables verbose debug logging for the module.
    type: bool
    default: false
    required: false
  vsphere_insecure: # Common optional parameter for vSphere connections
    description: If C(true), SSL certificate verification for the vSphere C(server) will be skipped.
    type: bool
    default: false
    required: false
  wait: # Common operational parameter for long tasks
    description: If C(true), the module will wait for the migration operation to complete before returning.
    type: bool
    default: true
    required: false
  timeout: # Common operational parameter for long tasks
    description: Overall timeout in seconds for the migration operation.
    type: int
    default: 3600 # 1 hour, as an example
    required: false

requirements:
  - openstacksdk # For OpenStack interaction with dst_cloud
  - VMware VDDK (specified by C(vddkpath))
  - Python libraries for vSphere interaction (e.g., pyvmomi) may be needed by underlying logic.
"""

EXAMPLES = r"""
- name: Migrate Guest from VMware using os-migrate
  os_migrate.vmware_migration_kit.migrate:
    dst_cloud: "{{ my_dst_cloud_auth_details }}" # This var should be a dict
    user: "{{ my_vcenter_username }}"
    password: "{{ my_vcenter_password }}"
    server: "{{ my_vcenter_hostname }}"
    vmname: "{{ target_vm_name_to_migrate }}"
    osmdatadir: "{{ os_migrate_data_directory_path }}"
    firstboot: "{{ path_to_first_boot_script_for_vm }}" # e.g., "/opt/os_migrate_data/{{ target_vm_name_to_migrate }}/network_config.sh"
    vddkpath: "{{ global_vddk_path }}"
    usesocks: "{{ migration_use_socks_proxy | bool }}"
    cbtsync: "{{ use_cbt_for_this_migration | bool }}"
    instanceuuid: "{{ target_openstack_instance_uuid }}" # UUID of the target server in OpenStack
    # convhostname: "{{ specific_conversion_host | default(omit) }}"
    compression: "zstd"
    debug_mode: true
    vsphere_insecure: true
    wait: true
    timeout: 7200
  register: migrate_vm_output
"""

RETURN = r"""
Changed:
  description: Indicates whether the migration operation made any changes.
  returned: always
  type: bool
  sample: true
Msg:
  description: A message describing the outcome of the migration.
  returned: always
  type: str
  sample: "VM migrated successfully"
ID:
  description:
    - Identifier related to the migrated resource. Based on the internal Go code snippet 'response.ID = volume',
      this likely refers to the ID of the primary/boot volume created or associated in OpenStack for the migrated VM.
  returned: on success
  type: str
  sample: "a1b2c3d4-e5f6-7890-1234-567890abcdef" # Example Cinder Volume ID
"""
