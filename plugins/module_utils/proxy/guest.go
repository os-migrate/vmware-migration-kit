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

package proxy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/vmware/govmomi/guest"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/types"
)

// GuestRunner runs shell scripts inside the proxy VM and returns their exit
// code. Output is not collected: reading it would need a file transfer
// through the ESXi host (port 443), so scripts report through exit codes.
type GuestRunner interface {
	// Validate checks the guest credentials.
	Validate(ctx context.Context) error
	// Run executes script with /bin/sh, as root through "sudo -n" when
	// asRoot is true, and waits up to timeout for it to exit.
	Run(ctx context.Context, script string, asRoot bool, timeout time.Duration) (int32, error)
}

// guestOps implements GuestRunner with vSphere guest operations (VMware Tools).
type guestOps struct {
	om   *guest.OperationsManager
	auth *types.NamePasswordAuthentication
	poll time.Duration
}

// NewGuestOps returns a GuestRunner using vSphere guest operations.
func NewGuestOps(c *vim25.Client, vm *object.VirtualMachine, user, password string) GuestRunner {
	return &guestOps{
		om:   guest.NewOperationsManager(c, vm.Reference()),
		auth: &types.NamePasswordAuthentication{Username: user, Password: password},
		poll: time.Second,
	}
}

func (g *guestOps) Validate(ctx context.Context) error {
	am, err := g.om.AuthManager(ctx)
	if err != nil {
		return err
	}
	return am.ValidateCredentials(ctx, g.auth)
}

func (g *guestOps) Run(ctx context.Context, script string, asRoot bool, timeout time.Duration) (int32, error) {
	pm, err := g.om.ProcessManager(ctx)
	if err != nil {
		return -1, err
	}
	spec := guestProgramSpec(script, asRoot)
	pid, err := pm.StartProgram(ctx, g.auth, &spec)
	if err != nil {
		return -1, fmt.Errorf("failed to start program in proxy guest: %w", err)
	}
	deadline := time.Now().Add(timeout)
	for {
		procs, err := pm.ListProcesses(ctx, g.auth, []int64{pid})
		if err != nil {
			return -1, fmt.Errorf("failed to query guest process %d: %w", pid, err)
		}
		if len(procs) == 1 && procs[0].EndTime != nil {
			return procs[0].ExitCode, nil
		}
		if time.Now().After(deadline) {
			_ = pm.TerminateProcess(ctx, g.auth, pid)
			return -1, fmt.Errorf("guest process %d did not finish within %s", pid, timeout)
		}
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-time.After(g.poll):
		}
	}
}

// guestProgramSpec builds the program spec running script with /bin/sh,
// optionally through "sudo -n" (fails instead of prompting for a password).
func guestProgramSpec(script string, asRoot bool) types.GuestProgramSpec {
	args := "-c " + shellQuote(script)
	if asRoot {
		return types.GuestProgramSpec{ProgramPath: "/usr/bin/sudo", Arguments: "-n /bin/sh " + args}
	}
	return types.GuestProgramSpec{ProgramPath: "/bin/sh", Arguments: args}
}

// shellQuote quotes s as a single POSIX shell word.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
