# Let's use bash!
SHELL := /bin/bash
.SHELLFLAGS := -euo pipefail -c

# Directory structure variables
COLLECTION_ROOT := $(CURDIR)
MODULES_DIR := $(COLLECTION_ROOT)/plugins/modules
VENV_DIR := $(COLLECTION_ROOT)/.venv/

# Configuration variables
CONTAINER_ENGINE := podman
# Use CentoS Stream 9 as base image because UPX is not available in CentOS 10
# @TODO: move to 10 when the payload size will be increase in Galaxy
CONTAINER_IMAGE := quay.io/centos/centos:stream9
BUILD_SCRIPT := /code/scripts/build.sh
PYTHON_VERSION := 3.12
MOUNT_PATH := $(COLLECTION_ROOT):/code/

# Check if SELinux is enabled by testing if getenforce exists and returns "Enforcing"
GETENFORCE_CMD := $(shell command -v getenforce 2>/dev/null)
SELINUX_ENFORCING := $(shell $(GETENFORCE_CMD) 2>/dev/null | grep -q "Enforcing" && echo "yes" || echo "no")

# Set security options only if SELinux is enforcing
ifeq ($(SELINUX_ENFORCING),yes)
    SECURITY_OPT := --security-opt label=disable
else
    SECURITY_OPT :=
endif

# Define a function to verify we're in the ansible collection root
define verify_collection_root
	@if [ ! -f "$(COLLECTION_ROOT)/galaxy.yml" ]; then \
		echo "Error: Must be run from the ansible collection root directory."; \
		echo "Missing galaxy.yml file."; \
		exit 1; \
	fi
endef

# Extract collection metadata from galaxy.yml (if it exists)
GALAXY_YML := $(COLLECTION_ROOT)/galaxy.yml
ifneq ($(wildcard $(GALAXY_YML)),)
    COLLECTION_NAMESPACE := $(shell grep -E "^namespace:" $(GALAXY_YML) | sed 's/namespace: *//g')
    COLLECTION_NAME := $(shell grep -E "^name:" $(GALAXY_YML) | sed 's/name: *//g')
    COLLECTION_VERSION := $(shell grep -E "^version:" $(GALAXY_YML) | sed 's/version: *//g')
    COLLECTION_TARBALL := $(COLLECTION_NAMESPACE)-$(COLLECTION_NAME)-$(COLLECTION_VERSION).tar.gz
else
    COLLECTION_TARBALL := *.tar.gz
endif

# Define phony targets (targets that don't create files)
.PHONY: all binaries clean-binaries help check-root tests test-pytest test-ansible-sanity build clean-build \
        create-venv clean-venv check-python-version

# Makes `make` less verbose :)
ifndef VERBOSE
MAKEFLAGS += --no-print-directory
endif

# Validate collection root directory
check-root:
	$(call verify_collection_root)

# Default target
all: binaries

# Help target
help:
	@echo "Available targets:"
	@echo "  help                - Display this help message"
	@echo "  binaries            - Build binaries using container"
	@echo "  clean-binaries      - Remove all built binaries from plugins/modules/"
	@echo "  build               - Build the collection using ansible-galaxy"
	@echo "  clean-build         - Remove the built collection tarball"
	@echo "  test-ansible-lint   - Launch ansible-lint test"
	@echo "  test-ansible-sanity - Launch ansible-sanity tests"
	@echo "  test-pytest         - Launch pytest test"
	@echo "  tests               - Launch all tests"
	@echo "  create-venv         - Create the virtualenv environment"
	@echo "  clean-venv          - Remove the virtualenv environment"
	@echo "  all                 - Same as binaries (default target)"
	@echo ""
	@echo "Customizable variables:"
	@echo "  CONTAINER_ENGINE - Container runtime (default: $(CONTAINER_ENGINE))"
	@echo "  CONTAINER_IMAGE  - Container image (default: $(CONTAINER_IMAGE))"
	@echo "  BUILD_SCRIPT     - Path to build script in container (default: $(BUILD_SCRIPT))"
	@echo "  SECURITY_OPT     - Security options (based on SELinux status)"
	@echo "                     Current value: $(SECURITY_OPT)"
	@echo ""
	@echo "Collection root: $(COLLECTION_ROOT)"
	@echo "Modules directory: $(MODULES_DIR)"
	@echo "SELinux status: $(if $(SELINUX_ENFORCING:yes=),Disabled or Permissive,Enforcing)"
	@echo ""
	@echo "Collection information:"
	@if [ "$(COLLECTION_TARBALL)" != "*.tar.gz" ]; then \
		echo "  Namespace: $(COLLECTION_NAMESPACE)"; \
		echo "  Name: $(COLLECTION_NAME)"; \
		echo "  Version: $(COLLECTION_VERSION)"; \
		echo "  Tarball: $(COLLECTION_TARBALL)"; \
	else \
		echo "  (galaxy.yml not found or incomplete)"; \
	fi

# Target to build binaries using container
binaries: check-root clean-binaries
	$(CONTAINER_ENGINE) run --rm -v $(MOUNT_PATH) $(SECURITY_OPT) $(CONTAINER_IMAGE) $(BUILD_SCRIPT)

# Target to clean built binaries
clean-binaries: check-root
	@# Check if modules directory exists
	@if [ ! -d "$(MODULES_DIR)" ]; then \
		echo "*** Error: $(MODULES_DIR) directory not found. ***"; \
		exit 1; \
	fi
	@# Count files that would be deleted
	@files_to_delete=$$(find $(MODULES_DIR) -type f ! -name "*.py" -a ! -name "*.go" | wc -l); \
	if [ $$files_to_delete -eq 0 ]; then \
		echo "*** No binary files found to delete in $(MODULES_DIR) ***"; \
	else \
		echo "*** Found $$files_to_delete files to delete ***"; \
		echo "*** Removing binary files from $(MODULES_DIR) ... ***"; \
		find $(MODULES_DIR) -type f ! -name "*.go" -a ! -name "*.py" -delete; \
		echo "*** Cleanup complete. ***"; \
	fi

# Target to build the collection
build: check-root clean-build clean-binaries binaries
	@echo "*** Building Ansible collection...***"
	@ansible-galaxy collection build
	@echo "*** Built collection: $(COLLECTION_TARBALL) ***"

# Target to build the collection for production (without teardown tasks)
build-prod: check-root clean-build clean-binaries binaries
    @echo "*** Building Ansible collection for production...***"
	@echo "*** Cleaning the teardown tasks as they are not needed in production ***"
	sed -i '22,$$d' $(COLLECTION_ROOT)/roles/import_workloads/tasks/main.yml
	truncate -s -1 $(COLLECTION_ROOT)/roles/import_workloads/tasks/main.yml
	rm -f $(COLLECTION_ROOT)/roles/import_workloads/tasks/teardown.yml
	sed -i '/plugins\/modules\/delete_/d' tests/sanity/ignore-2.*.txt
	@echo "*** Remove AEE and scripts from the build...***"
	rm -f $(MODULES_DIR)/delete_*
	rm -rf $(COLLECTION_ROOT)/aee
	rm -rf $(COLLECTION_ROOT)/scripts
	@ANSIBLE_GALAXY_DISABLE_GIT_CHECKSUM=1 ansible-galaxy collection build
	@echo "*** Built collection: $(COLLECTION_TARBALL) ***"

# Target to clean the built collection
clean-build:
	@echo "*** Cleaning built collection...***"
	@if [ -n "$(COLLECTION_TARBALL)" ] && [ "$(COLLECTION_TARBALL)" != "*.tar.gz" ]; then \
		if [ -f "$(COLLECTION_TARBALL)" ]; then \
			echo "*** Removing $(COLLECTION_TARBALL) ***"; \
			rm -f "$(COLLECTION_TARBALL)"; \
		else \
			echo "*** Collection tarball $(COLLECTION_TARBALL) not found ***"; \
		fi; \
	else \
		echo "*** Removing all collection tarballs ***"; \
		rm -f *.tar.gz; \
	fi

check-python-version:
	@echo "*** Check if Python $(PYTHON_VERSION) is available ***"
	@if [[ ! -x $$(command -v python$(PYTHON_VERSION)) ]]; then \
	  echo "*** Installing Python $(PYTHON_VERSION) ***"; \
		sudo dnf -y install python$(PYTHON_VERSION)-devel 2>/dev/null || \
			echo "package python$(PYTHON_VERSION) is unavailable"; exit 1; \
	else \
		echo "*** Python $(PYTHON_VERSION) is already available ***"; \
	fi

create-venv: clean-venv check-python-version
	@echo "*** Creating venv... ***"
	@python$(PYTHON_VERSION) -m venv $(VENV_DIR)

clean-venv:
	@if [ -d "$(VENV_DIR)" ]; then \
		echo "*** Removing virtual environment at $(VENV_DIR) ***" && \
		rm -fr "$(VENV_DIR)"; \
	fi

install:
	@echo "*** Installing dependencies... ***"
	@$(MAKE) create-venv && \
	source $(VENV_DIR)/bin/activate && \
	pip install -q --upgrade pip && \
	pip install -q -r requirements.txt && \
	$(MAKE) build
	@echo "*** Dependencies installed successfully ***"
	ansible-galaxy collection install $(COLLECTION_TARBALL) --force-with-deps

test-pytest:
	@$(MAKE) create-venv && \
	source $(VENV_DIR)/bin/activate && \
	pip install -q --upgrade pip && \
	pip install -q -r requirements-tests.txt && \
	pytest -q && \
	deactivate && \
	$(MAKE) clean-venv

test-ansible-lint:
	@$(MAKE) create-venv && \
	source $(VENV_DIR)/bin/activate && \
	pip install -q --upgrade pip && \
	pip install -q -r requirements.txt && \
	echo "*** Launching ansible-lint ***" && \
	ansible-lint && \
	deactivate && \
	$(MAKE) clean-venv

test-ansible-sanity:
	@$(MAKE) create-venv && \
	source $(VENV_DIR)/bin/activate && \
	pip install -q --upgrade pip && \
	pip install -q -r requirements.txt && \
	export TMPDIR="$$(mktemp -d)" && \
	export ANSIBLE_COLLECTIONS_PATH="$$TMPDIR/ansible_collections/" && \
	echo "*** Using temporary collections path: $$ANSIBLE_COLLECTIONS_PATH ***" && \
	$(MAKE) build && \
	echo "*** Installing collection dependencies... ***" && \
	ansible-galaxy collection install $(COLLECTION_TARBALL) --force-with-deps --collections-path "$$ANSIBLE_COLLECTIONS_PATH" && \
	cd "$$ANSIBLE_COLLECTIONS_PATH/$(COLLECTION_NAMESPACE)/$(COLLECTION_NAME)" && \
	echo "*** Running Ansible sanity tests...***" && \
	ansible-test sanity --python $(PYTHON_VERSION) --requirements \
	  --exclude aee/ \
		--exclude scripts/ \
	  --exclude plugins/modules/best_match_flavor \
	  --exclude plugins/modules/create_network_port \
	  --exclude plugins/modules/create_server \
	  --exclude plugins/modules/import_flavor \
	  --exclude plugins/modules/delete_flavor \
	  --exclude plugins/modules/delete_port \
	  --exclude plugins/modules/delete_server \
	  --exclude plugins/modules/delete_volume \
	  --exclude plugins/modules/flavor_info \
	  --exclude plugins/modules/migrate \
	  --exclude plugins/modules/volume_info \
	  --exclude plugins/modules/volume_metadata_info && \
	cd $(COLLECTION_ROOT) && \
	echo "*** Sanity tests completed successfully ***" && \
	rm -fr $$TMPDIR && \
	deactivate
	@$(MAKE) clean-venv

integration-test:
	@$(MAKE) create-venv && \
	source $(VENV_DIR)/bin/activate && \
	pip install -q --upgrade pip && \
	pip install -q -r requirements.txt && \
	export TMPDIR="$$(mktemp -d)" && \
	export ANSIBLE_COLLECTIONS_PATH="$$TMPDIR/ansible_collections/" && \
	echo "*** Using temporary collections path: $$ANSIBLE_COLLECTIONS_PATH ***" && \
	$(MAKE) build && \
	echo "*** Installing collection dependencies... ***" && \
	ansible-galaxy collection install $(COLLECTION_TARBALL) --force-with-deps --collections-path "$$ANSIBLE_COLLECTIONS_PATH" && \
	echo "*** Running integration tests... ***" && \
	ansible-playbook -i $(COLLECTION_ROOT)/localhost_inventory.yml $(COLLECTION_ROOT)/tests/integration/test_flavor_info.yml && \
	echo "*** Integration tests completed successfully ***" && \
	rm -rf $$TMPDIR && \
	deactivate
	@$(MAKE) clean-venv

test-golangci-lint: check-root
	@echo "*** Running golangci-lint in container ***"
	$(CONTAINER_ENGINE) run --rm -t \
		-v $(MOUNT_PATH) \
		-w /code \
		$(SECURITY_OPT) \
		golangci/golangci-lint:v2.5.0 \
		golangci-lint run --timeout 5m

tests: test-pytest test-ansible-sanity test-ansible-lint test-golangci-lint
