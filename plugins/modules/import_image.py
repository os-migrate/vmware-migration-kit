#!/usr/bin/python

DOCUMENTATION = r"""
module: import_image
short_description: Imports or updates an image in OpenStack Glance.
description:
  - This module imports a new image into OpenStack Glance from a URL, a local file path, or an existing Cinder volume.
  - It can also be used to update properties and tags of an existing Glance image if specified by C(name)
    (and C(allow_duplicates=false)) or C(existing_image_id).
  - The module handles waiting for the image to become active after creation or update.
author: "OpenStack tenant migration tools (@os-migrate)"
version_added: "1.0.0"
options:
  auth:
    description:
      - Dictionary of OpenStack authentication parameters.
      - Supports environment variables like C(OS_AUTH_URL), C(OS_USERNAME), etc.
    type: dict
    required: true
  region_name:
    description:
      - Name of the OpenStack region to use.
    type: str
    required: false
  name:
    description:
      - Name of the image to create or update.
      - If C(allow_duplicates=false) (the default) and an image with this name exists, the module will attempt to update it.
      - If C(existing_image_id) is provided, this name will be used for the update (e.g., to rename the image).
    type: str
    required: true
  state:
    description:
      - Desired state of the image.
      - Currently, only C(present) is supported.
    type: str
    default: present
    choices: ["present"]
    required: false
  source_url:
    description:
      - URL of the image file to import (e.g., HTTP, HTTPS).
      - This is one of the mutually exclusive source options for new image creation.
    type: str
    required: false
  source_volume_id:
    description:
      - ID of an existing Cinder volume to create the image from.
      - This is one of the mutually exclusive source options for new image creation.
    type: str
    required: false
  source_local_path:
    description:
      - Absolute path to an image file on the Ansible controller or the machine where the module executes (if delegated).
      - The module will read and upload this file to Glance.
      - This is one of the mutually exclusive source options for new image creation.
    type: str
    required: false
  existing_image_id:
    description:
      - UUID of an existing Glance image to update.
      - If provided, source options (C(source_url), C(source_volume_id), C(source_local_path)) are ignored.
      - The image specified by this ID will be updated with the provided parameters (name, visibility, properties, tags, etc.).
    type: str
    required: false
  disk_format:
    description:
      - Disk format of the image. Examples include C(qcow2), C(vmdk), C(raw), C(vdi), C(iso), C(aki), C(ari), C(ami).
      - Required when creating a new image. For updates, this field is generally not changeable.
    type: str
    required: true
  container_format:
    description:
      - Container format of the image. Examples include C(bare), C(ovf), C(aki), C(ari), C(ami).
      - Required when creating a new image. For updates, this field is generally not changeable.
    type: str
    required: true
  visibility:
    description:
      - Visibility of the image in Glance.
    type: str
    choices: ["private", "public", "shared", "community"]
    default: private
    required: false
  min_disk:
    description:
      - Minimum disk size required to boot the image, in gigabytes.
    type: int
    default: 0
    required: false
  min_ram:
    description:
      - Minimum RAM size required to boot the image, in megabytes.
    type: int
    default: 0
    required: false
  protected:
    description:
      - Whether the image is protected from deletion.
    type: bool
    default: false
    required: false
  tags:
    description:
      - List of tags to apply to the image.
      - When updating, these tags will replace any existing tags on the image.
        To add/remove specific tags without replacing all, retrieve current tags and merge.
    type: list
    elements: str
    required: false
  properties:
    description:
      - Dictionary of additional metadata properties to set on the image.
      - These are arbitrary key-value pairs.
      - Properties like C(architecture), C(os_distro), C(os_version), C(hw_disk_bus), etc., can also be set directly via their top-level options.
    type: dict
    required: false
  architecture:
    description:
      - CPU architecture of the image (e.g., C(x86_64), C(arm64)).
      - This will be set as an image property.
    type: str
    required: false
  os_distro:
    description:
      - Name of the OS distribution (e.g., C(ubuntu), C(centos), C(windows)).
      - This will be set as an image property.
    type: str
    required: false
  os_version:
    description:
      - Version of the OS distribution (e.g., C(20.04), C(7), C(10)).
      - This will be set as an image property.
    type: str
    required: false
  hw_disk_bus:
    description:
      - Specifies the type of disk controller device to attach the image to (e.g., C(scsi), C(ide), C(virtio)).
      - This will be set as an image property.
    type: str
    required: false
  hw_scsi_model:
    description:
      - Specifies the SCSI controller model (e.g., C(virtio-scsi), C(lsilogic)).
      - This will be set as an image property.
    type: str
    required: false
  hw_video_model:
    description:
      - Specifies the video device model (e.g., C(vga), C(cirrus), C(qxl), C(virtio)).
      - This will be set as an image property.
    type: str
    required: false
  hw_vif_model:
    description:
      - Specifies the virtual network interface card model (e.g., C(virtio), C(e1000), C(rtl8139)).
      - This will be set as an image property.
    type: str
    required: false
  wait:
    description:
      - Whether to wait for the image to reach an C(active) state before returning.
    type: bool
    default: true
    required: false
  timeout:
    description:
      - Timeout in seconds to wait for the image to become active.
    type: int
    default: 3600
    required: false
  validate_certs:
    description:
      - Whether to validate SSL certificates when C(source_url) is HTTPS.
    type: bool
    default: true
    required: false
  allow_duplicates:
    description:
      - If C(true), a new image will always be created, even if an image with the same C(name) already exists.
      - If C(false), and an image with the same C(name) exists, the module will attempt to update that image. If no parameters differ, no change occurs.
    type: bool
    default: false
    required: false
requirements:
  - openstacksdk
"""

EXAMPLES = r"""
- name: Import image from URL
  os_migrate.vmware_migration_kit.import_image:
    auth: "{{ openstack_auth_vars }}"
    name: "ubuntu-20.04-cloud"
    source_url: "http://cloud-images.ubuntu.com/focal/current/focal-server-cloudimg-amd64.img"
    disk_format: "qcow2"
    container_format: "bare"
    visibility: "public"
    tags:
      - "ubuntu"
      - "focal"
    properties:
      release: "20.04"
    hw_disk_bus: virtio
    hw_vif_model: virtio
    wait: true

- name: Upload image from local path and set custom properties
  os_migrate.vmware_migration_kit.import_image:
    auth: "{{ openstack_auth_vars }}"
    name: "my-custom-linux"
    source_local_path: "/opt/images/custom-linux.qcow2"
    disk_format: "qcow2"
    container_format: "bare"
    min_disk: 20
    min_ram: 1024
    protected: true
    architecture: x86_64
    os_distro: "MyDistro"
    tags:
      - "custom"
    properties:
      custom_build_id: "abc-123"

- name: Create image from Cinder volume
  os_migrate.vmware_migration_kit.import_image:
    auth: "{{ openstack_auth_vars }}"
    name: "image-from-volume-snapshot"
    source_volume_id: "a1b2c3d4-e5f6-7890-1234-567890abcdef"
    disk_format: "raw"
    container_format: "bare"
    visibility: "private"

- name: Update existing image properties and tags
  os_migrate.vmware_migration_kit.import_image:
    auth: "{{ openstack_auth_vars }}"
    existing_image_id: "f1e2d3c4-b5a6-7890-fedc-ba9876543210"
    name: "ubuntu-20.04-cloud-updated"
    visibility: "shared"
    protected: true
    min_disk: 15
    tags:
      - "ubuntu"
      - "focal"
      - "production-ready"
    properties:
      description: "Updated production Ubuntu image"
      tested_by: "admin"

- name: Ensure image exists, update if different, do not create duplicates by name
  os_migrate.vmware_migration_kit.import_image:
    auth: "{{ openstack_auth_vars }}"
    name: "centos-7-base"
    source_url: "http://cloud.centos.org/centos/7/images/CentOS-7-x86_64-GenericCloud.qcow2"
    disk_format: "qcow2"
    container_format: "bare"
    allow_duplicates: false
    properties:
      build_date: "2025-05-19"
    tags:
      - "centos7"
      - "base"

- name: Create image even if one with the same name exists
  os_migrate.vmware_migration_kit.import_image:
    auth: "{{ openstack_auth_vars }}"
    name: "dev-test-image"
    source_local_path: "/tmp/dev-test.img"
    disk_format: "raw"
    container_format: "bare"
    allow_duplicates: true
    tags:
      - "dev"
"""

RETURN = r"""
changed:
  description: Indicates whether any change was made to the image (creation or update).
  returned: always
  type: bool
  sample: true
msg:
  description: An optional message describing the action taken or errors.
  returned: on success or failure
  type: str
  sample: "Image 'ubuntu-20.04-cloud' created successfully."
image:
  description: Dictionary containing details of the imported or updated image.
  returned: on success
  type: complex
  contains:
    id:
      description: The UUID of the image.
      type: str
      sample: "da43a140-5303-4b60-919f-8f8a3a0f7d8f"
    name:
      description: The name of the image.
      type: str
      sample: "ubuntu-20.04-cloud"
    status:
      description: The current status of the image (e.g., C(active), C(queued), C(saving)).
      type: str
      sample: "active"
    visibility:
      description: The visibility of the image (e.g., C(private), C(public)).
      type: str
      sample: "public"
    protected:
      description: Whether the image is protected from deletion.
      type: bool
      sample: false
    disk_format:
      description: The disk format of the image.
      type: str
      sample: "qcow2"
    container_format:
      description: The container format of the image.
      type: str
      sample: "bare"
    min_disk:
      description: Minimum disk size in GB.
      type: int
      sample: 10
    min_ram:
      description: Minimum RAM size in MB.
      type: int
      sample: 512
    size:
      description: Size of the image file in bytes.
      type: int
      sample: 478150656
    tags:
      description: List of tags associated with the image.
      type: list
      elements: str
      sample: ["ubuntu", "focal"]
    properties:
      description: Dictionary of additional metadata properties of the image.
      type: dict
      sample:
        os_distro: "ubuntu"
        release: "20.04"
        architecture: "x86_64"
    architecture:
      description: CPU architecture of the image. (Often reflected in properties as well)
      type: str
      sample: "x86_64"
    os_distro:
      description: OS distribution. (Often reflected in properties as well)
      type: str
      sample: "ubuntu"
    os_version:
      description: OS version. (Often reflected in properties as well)
      type: str
      sample: "20.04"
    hw_disk_bus:
      description: Disk bus type. (Often reflected in properties as well)
      type: str
      sample: "virtio"
    hw_scsi_model:
      description: SCSI model. (Often reflected in properties as well)
      type: str
      sample: "virtio-scsi"
    hw_video_model:
      description: Video model. (Often reflected in properties as well)
      type: str
      sample: "virtio"
    hw_vif_model:
      description: Network interface model. (Often reflected in properties as well)
      type: str
      sample: "virtio"
    created_at:
      description: Timestamp of when the image was created.
      type: str
      sample: "2023-05-19T10:30:00Z"
    updated_at:
      description: Timestamp of when the image was last updated.
      type: str
      sample: "2023-05-19T10:35:00Z"
    file:
      description: The URI for the image file.
      type: str
      sample: "/v2/images/da43a140-5303-4b60-919f-8f8a3a0f7d8f/file"
    schema:
      description: The schema URI for the image.
      type: str
      sample: "/v2/schemas/image"
    owner:
      description: The tenant ID that owns the image.
      type: str
      sample: "1234567890abcdef1234567890abcdef"
"""
