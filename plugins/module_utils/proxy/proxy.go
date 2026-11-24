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
// proxy VM running in vSphere, and read over TCP from an nbdkit server already
// running there.
//
// Nothing is ever executed inside the proxy VM. It permanently runs
//
//	nbdkit --readonly --port 10809 file dir=/dev/disk/by-id
//
// as a system service. The file plugin resolves the NBD export name to a file
// of that directory when the client connects, so a disk hot-added after nbdkit
// started is served straight away, under the export name of its by-id link
// ("wwn-0x<disk uuid>", available when disk.EnableUUID is TRUE on the proxy).
//
// A migration therefore only needs the vSphere API (ReconfigVM to attach and
// detach) and a TCP connection to the proxy: no guest credentials, no guest
// operations privilege, no SSH, no VDDK and no NFC.
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

// DefaultPort is the port the nbdkit service of the proxy listens on.
const DefaultPort = 10809

// Config describes how to reach and drive the proxy VM.
type Config struct {
	// VMName is the proxy VM name or inventory path.
	VMName string
	// Address is the IP the conversion host uses to reach nbdkit on the proxy.
	// If empty, the IP reported by VMware Tools is used.
	Address string
	// Port is the port of the nbdkit service of the proxy.
	Port int
}

// withDefaults fills the port range and validates the config.
func (c Config) withDefaults() (Config, error) {
	if c.VMName == "" {
		return c, fmt.Errorf("proxy VM name is required")
	}
	if c.Port == 0 {
		c.Port = DefaultPort
	}
	if c.Port < 1 || c.Port > 65535 {
		return c, fmt.Errorf("invalid NBD port %d", c.Port)
	}
	return c, nil
}

// Proxy is a proxy VM ready to export disks.
type Proxy struct {
	Client *vim25.Client
	VM     *object.VirtualMachine
	Config Config
	// HWVersion is the hardware version of the proxy VM ("vmx-15" -> 15). It
	// decides how many disks a PVSCSI controller can hold at once.
	HWVersion int
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
	var o mo.VirtualMachine
	if err := vm.Properties(ctx, vm.Reference(), []string{"guest.ipAddress", "config.version"}, &o); err != nil {
		return nil, fmt.Errorf("failed to read proxy VM properties: %w", err)
	}
	if cfg.Address == "" {
		if o.Guest == nil || o.Guest.IpAddress == "" {
			return nil, fmt.Errorf("proxy VM %q has no IP reported by VMware Tools, set the proxy address", cfg.VMName)
		}
		cfg.Address = o.Guest.IpAddress
	}
	hw := 0
	if o.Config != nil {
		hw = hardwareVersion(o.Config.Version)
	}
	return &Proxy{Client: c, VM: vm, Config: cfg, HWVersion: hw}, nil
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
