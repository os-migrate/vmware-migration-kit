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
 * Copyright 2025 Red Hat, Inc.
 *
 */

package moduleutils

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	moduleutils "vmware-migration-kit/plugins/module_utils"
	"vmware-migration-kit/plugins/module_utils/nbdkit"
)

func writeFakeNbdkit(t *testing.T, dir string) string {
	t.Helper()
	script := `#!/bin/bash
prev=""
socket=""
for arg in "$@"; do
  if [ "$prev" = "--unix" ]; then
	socket="$arg"
	break
  fi
  prev="$arg"
done
if [ -n "$socket" ]; then
  mkdir -p "$(dirname "$socket")"
  touch "$socket"
fi
# keep process alive so tests can stop it
sleep 60
`
	path := filepath.Join(dir, "nbdkit")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write fake nbdkit: %v", err)
	}
	return path
}

func TestRunNbdKitFromLocal_Success(t *testing.T) {
	tempDir := t.TempDir()
	origPath := os.Getenv("PATH")
	// // write fake nbdkit and prepend to PATH
	// fake := writeFakeNbdkit(t, tempDir)
	if err := os.Setenv("PATH", tempDir+string(os.PathListSeparator)+origPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", origPath)
	})

	// prepare a dummy disk file (not used by fake but mirrors real usage)
	diskDir := t.TempDir()
	diskName := "disk.vmdk"
	diskPath := filepath.Join(diskDir, diskName)
	if err := os.WriteFile(diskPath, []byte("dummy"), 0o644); err != nil {
		t.Fatalf("failed to write dummy disk: %v", err)
	}

	cfg := &nbdkit.NbdkitConfig{
		User:        "u",
		Password:    "p",
		Server:      "s",
		Libdir:      "/lib",
		VmName:      "My VM",
		Compression: "none",
		UUID:        "uuid-1234",
	}

	s, err := cfg.RunNbdKitFromLocal(diskName, diskDir)
	if err != nil {
		t.Fatalf("expected RunNbdKitFromLocal to succeed, got error: %v", err)
	}
	if s == nil {
		t.Fatalf("expected non-nil server")
	}

	expectedSocket := fmt.Sprintf("/tmp/nbdkit-%s-%s.sock", moduleutils.SafeVmName(cfg.VmName), cfg.UUID)
	unixSockPath := fmt.Sprintf("nbd+unix:///?socket=/tmp/nbdkit-%s-%s.sock", moduleutils.SafeVmName(cfg.VmName), cfg.UUID)
	if s.GetSocketPath() != unixSockPath {
		t.Fatalf("unexpected socket path: got %s want %s", s.GetSocketPath(), unixSockPath)
	}
	// ensure file exists
	if _, err := os.Stat(expectedSocket); err != nil {
		t.Fatalf("expected socket file to exist: %v", err)
	}

	// stop the server and ensure socket removed
	if err := s.Stop(); err != nil {
		t.Fatalf("failed to stop server: %v", err)
	}
	// give a moment for cleanup
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(expectedSocket); !os.IsNotExist(err) {
		t.Fatalf("expected socket file to be removed after stop, stat err: %v", err)
	}
}

func TestRunNbdKitFromLocal_StartFailure(t *testing.T) {
	// Ensure PATH does not contain nbdkit so Start() fails
	origPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", ""); err != nil {
		t.Fatalf("failed to clear PATH: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", origPath)
	})

	diskDir := t.TempDir()
	diskName := "disk.vmdk"
	if err := os.WriteFile(filepath.Join(diskDir, diskName), []byte("dummy"), 0o644); err != nil {
		t.Fatalf("failed to write dummy disk: %v", err)
	}

	cfg := &nbdkit.NbdkitConfig{
		Libdir: "/lib",
		VmName: "VM",
		UUID:   "u",
	}

	_, err := cfg.RunNbdKitFromLocal(diskName, diskDir)
	if err == nil {
		t.Fatalf("expected RunNbdKitFromLocal to fail when nbdkit not present")
	}
}
