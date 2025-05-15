# Configuration variables
CONTAINER_ENGINE := podman
CONTAINER_IMAGE := quay.io/centos/centos:stream10
BUILD_SCRIPT := /code/scripts/build.sh
MOUNT_PATH := $(PWD):/code/

# Check if SELinux is enabled by testing if getenforce exists and returns "Enforcing"
GETENFORCE_CMD := $(shell command -v getenforce 2>/dev/null)
SELINUX_ENFORCING := $(shell $(GETENFORCE_CMD) 2>/dev/null | grep -q "Enforcing" && echo "yes" || echo "no")

# Set security options only if SELinux is enforcing
ifeq ($(SELINUX_ENFORCING),yes)
    SECURITY_OPT := --security-opt label=disable
else
    SECURITY_OPT :=
endif

# Define phony targets (targets that don't create files)
.PHONY: all binaries clean-binaries help

# Default target
all: binaries

# Help target
help:
	@echo "Available targets:"
	@echo "  help          - Display this help message"
	@echo "  binaries      - Build binaries using container"
	@echo "  clean-binaries - Remove all built binaries from plugins/modules/"
	@echo "  all           - Same as binaries (default target)"
	@echo ""
	@echo "Customizable variables:"
	@echo "  CONTAINER_ENGINE - Container runtime (default: $(CONTAINER_ENGINE))"
	@echo "  CONTAINER_IMAGE  - Container image (default: $(CONTAINER_IMAGE))"
	@echo "  BUILD_SCRIPT     - Path to build script in container (default: $(BUILD_SCRIPT))"
	@echo "  SECURITY_OPT     - Security options (based on SELinux status)"
	@echo "                     Current value: $(SECURITY_OPT)"
	@echo ""
	@echo "SELinux status: $(if $(SELINUX_ENFORCING:yes=),Disabled or Permissive,Enforcing)"

# Target to build binaries using container
binaries:
	$(CONTAINER_ENGINE) run --rm -v $(MOUNT_PATH) $(SECURITY_OPT) $(CONTAINER_IMAGE) $(BUILD_SCRIPT)

# Target to clean built binaries (removes all non .go and non .py files from plugins/modules/)
clean-binaries:
	find plugins/modules/ -type f ! -name '*.go' -a ! -name '*.py' -delete
