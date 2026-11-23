/*
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright 2026 Red Hat, Inc.
 *
 */

// Package proxy implements the "proxy VM" transport: the snapshot disk of a
// source VM is hot-added (read-only, independent-nonpersistent) to a Linux
// proxy VM running in vSphere, and exported with nbdkit over TCP. The proxy is
// driven only through the vSphere API (ReconfigVM + guest operations): no SSH
// to ESXi, no VDDK, no NFC.
//
// This package must stay free of cgo dependencies (no libnbd): it is linked
// into the module_dispatcher binary. Do not import the vmware or nbdkit
// module_utils packages from here.
package proxy

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
)

const (
	// DefaultPortMin is the first NBD port of a proxy.
	DefaultPortMin = 10809
	// PortsPerProxy is one port per SCSI slot (4 controllers x 16 units).
	// To scale further, use several proxy VMs.
	PortsPerProxy = 64
)

// Config describes how to reach and drive the proxy VM.
type Config struct {
	// VMName is the proxy VM name or inventory path.
	VMName string
	// Address is the IP the conversion host uses to reach nbdkit on the proxy.
	// If empty, the IP reported by VMware Tools is used.
	Address string
	// PortMin/PortMax is the NBD port range opened on the proxy firewall.
	PortMin int
	PortMax int
	// GuestUser/GuestPassword authenticate vSphere guest operations. The user
	// needs passwordless sudo on the proxy.
	GuestUser     string
	GuestPassword string
}

// withDefaults fills the port range and validates the config.
func (c Config) withDefaults() (Config, error) {
	if c.VMName == "" {
		return c, fmt.Errorf("proxy VM name is required")
	}
	if c.GuestUser == "" || c.GuestPassword == "" {
		return c, fmt.Errorf("proxy guest username and password are required (vSphere guest operations)")
	}
	if c.PortMin == 0 {
		c.PortMin = DefaultPortMin
	}
	if c.PortMax == 0 {
		c.PortMax = c.PortMin + PortsPerProxy - 1
	}
	if c.PortMin < 1 || c.PortMax > 65535 || c.PortMax < c.PortMin {
		return c, fmt.Errorf("invalid NBD port range %d-%d", c.PortMin, c.PortMax)
	}
	return c, nil
}

// Proxy is a proxy VM ready to export disks.
type Proxy struct {
	Client *vim25.Client
	VM     *object.VirtualMachine
	Config Config
	Guest  GuestRunner
}

// New finds the proxy VM and prepares the guest operations runner.
// The finder must already have its datacenter set.
func New(ctx context.Context, c *vim25.Client, finder *find.Finder, cfg Config) (*Proxy, error) {
	cfg, err := cfg.withDefaults()
	if err != nil {
		return nil, err
	}
	vm, err := finder.VirtualMachine(ctx, cfg.VMName)
	if err != nil {
		return nil, fmt.Errorf("proxy VM %q not found: %w", cfg.VMName, err)
	}
	if cfg.Address == "" {
		var o mo.VirtualMachine
		if err := vm.Properties(ctx, vm.Reference(), []string{"guest.ipAddress"}, &o); err != nil {
			return nil, fmt.Errorf("failed to read proxy VM IP: %w", err)
		}
		if o.Guest == nil || o.Guest.IpAddress == "" {
			return nil, fmt.Errorf("proxy VM %q has no IP reported by VMware Tools, set the proxy address", cfg.VMName)
		}
		cfg.Address = o.Guest.IpAddress
	}
	return &Proxy{
		Client: c,
		VM:     vm,
		Config: cfg,
		Guest:  NewGuestOps(c, vm, cfg.GuestUser, cfg.GuestPassword),
	}, nil
}

// NewInDatacenterOf prepares the proxy VM, looking it up in the datacenter of
// the given VM inventory path (the VM being migrated).
func NewInDatacenterOf(ctx context.Context, c *vim25.Client, vmInventoryPath string, cfg Config) (*Proxy, error) {
	finder := find.NewFinder(c, true)
	dc, err := finder.DatacenterOrDefault(ctx, datacenterPath(vmInventoryPath))
	if err != nil {
		return nil, fmt.Errorf("cannot find the datacenter of %s: %w", vmInventoryPath, err)
	}
	finder.SetDatacenter(dc)
	return New(ctx, c, finder, cfg)
}

// datacenterPath returns the datacenter part of a VM inventory path:
// "/DC/vm/folder/name" -> "/DC", "/Folder/DC/vm/name" -> "/Folder/DC".
// It returns "" (default datacenter) when the path has no "/vm/" element.
func datacenterPath(inventoryPath string) string {
	p := path.Clean("/" + inventoryPath)
	if i := strings.Index(p, "/vm/"); i > 0 {
		return p[:i]
	}
	return ""
}
