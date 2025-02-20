#! /bin/bash

img_registry=quay.io/rhn_engineering_mbultel/osm-fedora:latest
tag_name=osm-fedora

ansible-builder build --tag $tag_name
podman push localhost/$tag_name $img_registry
