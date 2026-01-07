#!/bin/bash
# Heat Stack Adoption POC Runner
# This script runs the Heat adoption proof of concept

set -e

echo "======================================================================"
echo "  Heat Stack Adoption POC - Runner Script"
echo "======================================================================"
echo ""

# Check if OpenStack credentials are set
if [ -z "$OS_AUTH_URL" ]; then
    echo "Error: OpenStack credentials not found in environment"
    echo ""
    echo "Please set the following environment variables:"
    echo "  export OS_AUTH_URL=https://your-openstack-cloud:5000/v3"
    echo "  export OS_USERNAME=your-username"
    echo "  export OS_PASSWORD=your-password"
    echo "  export OS_PROJECT_NAME=your-project"
    echo "  export OS_DOMAIN_NAME=Default"
    echo "  export OS_PROJECT_ID=your-project-id"
    echo "  export OS_REGION_NAME=RegionOne  # Optional"
    echo ""
    echo "Or source your OpenStack RC file:"
    echo "  source ~/openrc.sh"
    echo ""
    exit 1
fi

echo "OpenStack credentials found:"
echo "  Auth URL: $OS_AUTH_URL"
echo "  Username: $OS_USERNAME"
echo "  Project:  $OS_PROJECT_NAME"
echo "  Region:   ${OS_REGION_NAME:-<not set>}"
echo ""

# Check if Heat service is available (optional check)
echo "Checking if Heat service is available..."
if command -v openstack &> /dev/null; then
    if openstack orchestration service list &> /dev/null 2>&1; then
        echo "✓ Heat service is available"
    else
        echo "Warning: Could not verify Heat service (continuing anyway)"
    fi
else
    echo "Info: OpenStack CLI not available, skipping service check"
fi
echo ""

# Run the POC
echo "Running Heat adoption POC..."
echo "======================================================================"
echo ""

cd "$(dirname "$0")"
# go run heat_adoption_simple_poc.go
go run test_manual_adoption.go

echo ""
echo "======================================================================"
echo "  POC execution completed"
echo "======================================================================"
