#!/usr/bin/python

DOCUMENTATION = r"""
module: create_server
short_description: Creates an OpenStack server based on VMware source VM information
description:
  - This module creates an OpenStack server/instance using parameters from a JSON file containing information about a source VM from VMware.
  - It handles the creation of an OpenStack instance with specified configurations like flavor, availability zone, name, etc.
  - Attaches network interfaces to the server based on source VM configuration.
  - Attaches volumes to the server based on source VM disks.
  - Sets security groups for the instance.
author: "OpenStack tenant migration tools (@os-migrate)"
version_added: "1.0.0"
options:
  name:
    description:
      - Name of the server to create.
    required: true
    type: str
  state:
    description:
      - Desired state of the server.
      - Only 'present' is currently supported as this module is focused on creation.
    required: false
    default: present
    choices: ["present"]
    type: str
  auth:
    description:
      - Authentication parameters for OpenStack.
      - Can include auth_url, username, password, project_name, domain_name, etc.
    required: true
    type: dict
  region_name:
    description:
      - OpenStack region to create the server in.
    required: false
    type: str
  availability_zone:
    description:
      - Availability zone to create the server in.
    required: false
    type: str
  networks:
    description:
      - List of networks to attach to the server.
      - Each network should be specified as a dict with at least a 'uuid' key.
      - Can also include 'fixed_ip' if a specific IP address is requested.
    required: false
    type: list
    elements: dict
    suboptions:
      uuid:
        description: The UUID of the network.
        type: str
        required: true
      fixed_ip:
        description: A specific IP address to assign from the network.
        type: str
        required: false
  flavor_id:
    description:
      - ID of the flavor to use for the server.
    required: true
    type: str
  key_name:
    description:
      - Name of the SSH key to inject into the server.
    required: false
    type: str
  security_groups:
    description:
      - List of security group names to apply to the server.
    required: false
    type: list
    elements: str
  source_vm_json_path:
    description:
      - Path to the JSON file containing source VM information from VMware.
      - Used to configure the server based on the source VM's specifications.
    required: true
    type: str
  user_data:
    description:
      - User data to pass to the instance.
      - This is generally used for cloud-init scripts.
    required: false
    type: str
  image_id:
    description:
      - ID of the image to use for the server.
      - Only used when not booting from volume.
    required: false
    type: str
  volumes:
    description:
      - List of volume specifications to attach to the server.
      - Each volume should be specified as a dict with keys like 'id', 'device', 'boot_index', etc.
    required: false
    type: list
    elements: dict
    suboptions:
      id:
        description: The ID of the volume to attach.
        type: str
        required: true
      device:
        description: The device name for the volume on the server (e.g., /dev/vdb).
        type: str
        required: false
      boot_index:
        description: Integer used to sort bootable volumes (e.g., 0 for boot, -1 for non-boot).
        type: int
        required: false
  metadata:
    description:
      - Dictionary of metadata to apply to the server.
    required: false
    type: dict
  boot_from_volume:
    description:
      - Whether to boot the server from a volume.
      - If true, must specify either image_id (to create boot volume from image) or volume_id in the first volume entry.
    required: false
    type: bool
    default: false
  boot_from_cinder:
    description:
      - Boot from existing cinder volume
    required: false
    type: bool
    default: false
  timeout:
    description:
      - Timeout in seconds for server creation operation.
    required: false
    type: int
    default: 1800
  wait:
    description:
      - Whether to wait for the server to reach active state before returning.
    required: false
    type: bool
    default: true
  instance_scheduler:
    description:
      - Decides whether the instance should be scheduled somewhere by the scheduler or directly built on a specified host.
    required: false
    type: dict
    suboptions:
      instance_host:
        description:
          - The instance host to build the instance on.
        required: false
        type: str
      scheduler_hints:
        description:
          - Hints for the nova scheduler when placing the instance.
        required: false
        type: dict
requirements:
  - openstacksdk
"""

EXAMPLES = r"""
# Create a server from VMware source VM information with basic settings
- name: Create OpenStack server from VMware VM
  os_migrate.vmware_migration_kit.create_server:
    name: new-server-01
    auth:
      auth_url: http://openstack:5000/v3
      username: admin
      password: secret
      project_name: project1
      user_domain_name: Default
      project_domain_name: Default
    region_name: RegionOne
    availability_zone: nova
    flavor_id: m1.small
    source_vm_json_path: /tmp/vm_info.json
    wait: true
    timeout: 1800

# Create a server with specific network configuration
- name: Create server with network configuration
  os_migrate.vmware_migration_kit.create_server:
    name: web-server
    auth: "{{ openstack_auth }}"
    flavor_id: "{{ openstack_flavor_id }}"
    source_vm_json_path: "{{ source_vm_json }}"
    networks:
      - uuid: "{{ network_uuid }}"
        fixed_ip: 192.168.1.100
    security_groups:
      - default
      - web-server
    key_name: admin-key

# Create a server with custom metadata and boot from volume
- name: Create server booting from volume
  os_migrate.vmware_migration_kit.create_server:
    name: database-server
    auth: "{{ openstack_auth }}"
    flavor_id: "{{ openstack_flavor_id }}"
    source_vm_json_path: "{{ source_vm_json }}"
    boot_from_volume: true
    volumes:
      - id: "{{ boot_volume_id }}"
        device: /dev/vda
        boot_index: 0
      - id: "{{ data_volume_id }}"
        device: /dev/vdb
    metadata:
      environment: production
      application: database
      owner: dbteam

# Create a server with user data for cloud-init
- name: Create server with cloud-init configuration
  os_migrate.vmware_migration_kit.create_server:
    name: app-server
    auth: "{{ openstack_auth }}"
    flavor_id: "{{ openstack_flavor_id }}"
    source_vm_json_path: "{{ source_vm_json }}"
    user_data: |
      #cloud-config
      package_upgrade: true
      packages:
        - nginx
        - python3
      write_files:
        - path: /etc/nginx/sites-available/default
          content: |
            server {
              listen 80 default_server;
              listen [::]:80 default_server;
              root /var/www/html;
              index index.html;
              server_name _;
              location / {
                try_files $uri $uri/ =404;
              }
            }

# Create a server with specific instance scheduling
- name: Create server with specific host
  os_migrate.vmware_migration_kit.create_server:
    name: compute-server
    auth: "{{ openstack_auth }}"
    flavor_id: "{{ openstack_flavor_id }}"
    source_vm_json_path: "{{ source_vm_json }}"
    instance_scheduler:
      instance_host: compute-host-01
"""

RETURN = r"""
server:
  description: Dictionary containing detailed information about the created server.
  returned: success
  type: complex
  contains:
    id:
      description: The OpenStack UUID of the server.
      returned: success
      type: str
      sample: "f8f47a10-d68a-4b12-a9ba-c8f13ef3fc39"
    name:
      description: The name of the server.
      returned: success
      type: str
      sample: "web-server-01"
    status:
      description: The current status of the server.
      returned: success
      type: str
      sample: "ACTIVE"
    addresses:
      description: Dictionary of network configurations with associated IP addresses.
      returned: success
      type: dict
      sample:
        "private":
          - addr: "10.0.0.10"
            version: 4
            OS-EXT-IPS-MAC:mac_addr: "fa:16:3e:12:34:56"
            OS-EXT-IPS:type: "fixed"
        "public":
          - addr: "172.24.0.10"
            version: 4
            OS-EXT-IPS-MAC:mac_addr: "fa:16:3e:78:9a:bc"
            OS-EXT-IPS:type: "floating"
    flavor:
      description: Dictionary containing flavor information.
      returned: success
      type: dict
      sample:
        id: "1"
        name: "m1.small"
        links:
          - href: "..."
            rel: "bookmark"
    image:
      description: Dictionary containing image information if the server was booted from an image (not a pre-existing volume).
      returned: success when booted from image
      type: dict
      sample:
        id: "7af5c7f5-15d4-4ceb-96f9-d9d9420f3c1d"
        name: "centos-7"
        links:
          - href: "..."
            rel: "bookmark"
    volumes_attached:
      description: List of volumes attached to the server.
      returned: success
      type: list
      elements: dict
      contains:
        id:
          description: The ID of the attached volume.
          type: str
          sample: "4a67fd3a-344d-4e6b-9f3b-4eddd8c0d1d4"
        device:
          description: The device path where the volume is attached (e.g., /dev/vda). This specific field might vary based on API version.
          type: str
          sample: "/dev/vda"
      sample:
        - id: "4a67fd3a-344d-4e6b-9f3b-4eddd8c0d1d4"
          device: "/dev/vda"
        - id: "1af5c7f5-15d4-4ceb-96f9-d9d9420f3c1e"
          device: "/dev/vdb"
    metadata:
      description: Dictionary of metadata applied to the server.
      returned: success
      type: dict
      sample: { "environment": "production", "application": "database" }
    key_name:
      description: Name of the SSH key added to the server.
      returned: success when key is applied
      type: str
      sample: "admin-key"
    created:
      description: Creation timestamp of the server.
      returned: success
      type: str
      sample: "2023-01-15T09:10:23Z"
    availability_zone:
      description: Availability zone where the server is deployed.
      returned: success
      type: str
      sample: "nova"
    security_groups:
      description: List of security groups applied to the server.
      returned: success
      type: list
      elements: dict
      contains:
        name:
          description: Name of the security group.
          type: str
          sample: "default"
      sample:
        - name: "default"
        - name: "web-server"
    power_state:
      description: The power state of the server (e.g., 1 for RUNNING, 4 for SHUTDOWN).
      returned: success
      type: int
      sample: 1
    task_state:
      description: The task state of the server during transitions (e.g., 'spawning', 'deleting', null).
      returned: success
      type: str # Can be null
      sample: null
"""
