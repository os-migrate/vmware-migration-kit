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
	"net"
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
// Test 1: RunNbdKitFromLocal successfully starts nbdkit server with fake binary
func TestRunNbdKitFromLocal_Success(t *testing.T) {
	tempDir := t.TempDir()
	origPath := os.Getenv("PATH")
	// Write fake nbdkit and prepend to PATH
	writeFakeNbdkit(t, tempDir)
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

// Test 2: RunNbdKitFromLocal fails when nbdkit binary not found in PATH
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

// Test 3: WaitForNbdkit succeeds when socket file exists
func TestWaitForNbdkit_Success(t *testing.T) {
	// Create a temporary socket file
	tempDir := t.TempDir()
	socketPath := filepath.Join(tempDir, "test.sock")
	if err := os.WriteFile(socketPath, []byte(""), 0o644); err != nil {
		t.Fatalf("failed to create socket file: %v", err)
	}

	// Should succeed immediately since file exists
	err := nbdkit.WaitForNbdkit(socketPath, 5*time.Second)
	if err != nil {
		t.Fatalf("expected WaitForNbdkit to succeed, got error: %v", err)
	}
}

// Test 4: WaitForNbdkit times out when socket file doesn't exist
func TestWaitForNbdkit_Timeout(t *testing.T) {
	// Use a non-existent path
	socketPath := "/tmp/nonexistent-socket-" + fmt.Sprintf("%d", time.Now().UnixNano()) + ".sock"

	// Should timeout quickly
	err := nbdkit.WaitForNbdkit(socketPath, 1*time.Second)
	if err == nil {
		t.Fatalf("expected WaitForNbdkit to timeout, but it succeeded")
	}
	if err.Error() != "timed out waiting for nbdkit to be ready" {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// Test 5: WaitForNbdkitURI succeeds when TCP port is listening
func TestWaitForNbdkitURI_Success(t *testing.T) {
	// Start a TCP listener on a random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer func() {
		if err := listener.Close(); err != nil {
			t.Logf("Failed to close listener: %v", err)
		}
	}()

	// Get the actual port assigned
	addr := listener.Addr().(*net.TCPAddr)
	port := fmt.Sprintf("%d", addr.Port)

	// Should succeed since port is listening
	err = nbdkit.WaitForNbdkitURI("127.0.0.1", port, 5*time.Second)
	if err != nil {
		t.Fatalf("expected WaitForNbdkitURI to succeed, got error: %v", err)
	}
}

// Test 6: WaitForNbdkitURI times out when TCP port is not open
func TestWaitForNbdkitURI_Timeout(t *testing.T) {
	// Use a port that's definitely not listening
	// Port 0 is reserved and won't have anything listening
	err := nbdkit.WaitForNbdkitURI("127.0.0.1", "0", 1*time.Second)
	if err == nil {
		t.Fatalf("expected WaitForNbdkitURI to timeout, but it succeeded")
	}
	if err.Error() != "timed out waiting for nbdkit to be ready" {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// Test 7: RunNbdKit routes to RunNbdKitSocks when UseSocks=true
func TestRunNbdKit_RouteToSocks(t *testing.T) {
	config := &nbdkit.NbdkitConfig{
		User:        "test-user",
		Password:    "test-pass",
		Server:      "invalid-server.test",
		Libdir:      "/usr/lib/vmware-vix-disklib",
		VmName:      "test-vm",
		Compression: "zlib",
		UUID:        "test-uuid",
		UseSocks:    true,
		VddkConfig:  nil,
	}

	_, err := config.RunNbdKit("disk-0")

	if err == nil {
		t.Error("Expected error due to invalid config, got nil")
	}
}

// Test 8: RunNbdKit routes to RunNbdKitURI when UseSocks=false
func TestRunNbdKit_RouteToURI(t *testing.T) {
	config := &nbdkit.NbdkitConfig{
		User:        "test-user",
		Password:    "test-pass",
		Server:      "invalid-server.test",
		Libdir:      "/usr/lib/vmware-vix-disklib",
		VmName:      "test-vm",
		Compression: "zlib",
		UUID:        "test-uuid",
		UseSocks:    false,
		VddkConfig:  nil,
	}

	_, err := config.RunNbdKit("disk-0")

	if err == nil {
		t.Error("Expected error due to invalid config, got nil")
	}
}

// TODOs: potential tests - need refactoring or rather integration testing approach
// NbdCopy - success case with mocked command, start failure, wait failure
// V2VConversion - success with all options, findVirtV2v failure, missing run script file, missing boot script file, command execution failure, debug mode sets environment variables