#!/usr/bin/env bash

img_registry=quay.io/os-migrate/vmware-migration-kit:latest
tag_name=vmware-migration-kit

ansible-builder build --tag $tag_name
podman push localhost/$tag_name $img_registry
