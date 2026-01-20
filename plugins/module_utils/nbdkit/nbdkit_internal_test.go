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

package nbdkit

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// This file contains tests for the private functions in the nbdkit package.
// This is easiest fastest way to  test the private functions without excessive mocking.
// It gets run in ci/cd since we run all tests in makefile, so no worries about skipping.
// Test 1: findVirtV2v finds virt-v2v-in-place binary in PATH
func TestFindVirtV2v_Success(t *testing.T) {
	tempDir := t.TempDir()
	origPath := os.Getenv("PATH")

	// Create fake virt-v2v-in-place binary
	v2vPath := filepath.Join(tempDir, "virt-v2v-in-place")
	if err := os.WriteFile(v2vPath, []byte("#!/bin/bash\necho fake"), 0o755); err != nil {
		t.Fatalf("failed to create fake virt-v2v-in-place: %v", err)
	}

	if err := os.Setenv("PATH", tempDir+string(os.PathListSeparator)+origPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", origPath)
	})

	path, err := findVirtV2v()
	if err != nil {
		t.Fatalf("expected findVirtV2v to succeed, got error: %v", err)
	}

	expectedPath := tempDir + "/"
	if path != expectedPath {
		t.Fatalf("unexpected path: got %s, want %s", path, expectedPath)
	}
}

// Test 2: findVirtV2v returns error when virt-v2v-in-place not in PATH
func TestFindVirtV2v_NotFound(t *testing.T) {
	origPath := os.Getenv("PATH")
	emptyDir := t.TempDir()

	if err := os.Setenv("PATH", emptyDir); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", origPath)
	})

	_, err := findVirtV2v()
	if err == nil {
		t.Fatalf("expected findVirtV2v to fail, but it succeeded")
	}

	expectedMsg := "virt-v2v-in-place not found on the file system"
	if err.Error() != expectedMsg {
		t.Fatalf("unexpected error: got %q, want %q", err.Error(), expectedMsg)
	}
}

// Test 3: removeSocket successfully deletes existing socket file
func TestRemoveSocket_Success(t *testing.T) {
	tempDir := t.TempDir()
	socketPath := filepath.Join(tempDir, "test.sock")

	if err := os.WriteFile(socketPath, []byte(""), 0o644); err != nil {
		t.Fatalf("failed to create socket file: %v", err)
	}

	if _, err := os.Stat(socketPath); err != nil {
		t.Fatalf("socket file should exist: %v", err)
	}

	err := removeSocket(socketPath)
	if err != nil {
		t.Fatalf("expected removeSocket to succeed, got error: %v", err)
	}

	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Fatalf("socket file should be removed")
	}
}

// Test 4: removeSocket handles missing socket file gracefully
func TestRemoveSocket_MissingFile(t *testing.T) {
	err := removeSocket("")
	if err != nil {
		t.Fatalf("expected removeSocket('') to succeed, got error: %v", err)
	}

	nonExistentPath := "/tmp/nonexistent-" + fmt.Sprintf("%d", time.Now().UnixNano()) + ".sock"
	err = removeSocket(nonExistentPath)
	if err != nil {
		t.Fatalf("expected removeSocket with non-existent file to succeed, got error: %v", err)
	}
}

// Test 5: GetSocketPath returns empty string when socket is empty
func TestGetSocketPath_EmptySocket(t *testing.T) {
	server := &NbdkitServer{
		cmd:    nil,
		socket: "",
	}

	path := server.GetSocketPath()
	if path != "" {
		t.Fatalf("expected empty path, got: %s", path)
	}
}

// Test 6: GetSocketPath returns formatted NBD URI when socket is set
func TestGetSocketPath_WithSocket(t *testing.T) {
	socketPath := "/tmp/test.sock"
	server := &NbdkitServer{
		cmd:    nil,
		socket: socketPath,
	}

	path := server.GetSocketPath()
	expected := "nbd+unix:///?socket=/tmp/test.sock"
	if path != expected {
		t.Fatalf("unexpected path: got %s, want %s", path, expected)
	}
}

// Test 7: buildNbdCopyCommand with socket and assumeZero=true
func TestBuildNbdCopyCommand_SocketWithAssumeZero(t *testing.T) {
	result := buildNbdCopyCommand("nbd+unix:///?socket=/tmp/test.sock", "/dev/vda", true)
	expected := "/usr/bin/nbdcopy nbd+unix:///?socket=/tmp/test.sock /dev/vda --destination-is-zero --progress"
	if result != expected {
		t.Fatalf("unexpected command:\ngot:  %q\nwant: %q", result, expected)
	}
}

// Test 8: buildNbdCopyCommand with socket and assumeZero=false
func TestBuildNbdCopyCommand_SocketWithoutAssumeZero(t *testing.T) {
	result := buildNbdCopyCommand("nbd+unix:///?socket=/tmp/test.sock", "/dev/vda", false)
	expected := "/usr/bin/nbdcopy nbd+unix:///?socket=/tmp/test.sock /dev/vda --progress"
	if result != expected {
		t.Fatalf("unexpected command:\ngot:  %q\nwant: %q", result, expected)
	}
}

// Test 9: buildNbdCopyCommand without socket and assumeZero=true
func TestBuildNbdCopyCommand_NoSocketWithAssumeZero(t *testing.T) {
	result := buildNbdCopyCommand("", "/dev/vda", true)
	expected := "/usr/bin/nbdcopy nbd://localhost /dev/vda --destination-is-zero --progress"
	if result != expected {
		t.Fatalf("unexpected command:\ngot:  %q\nwant: %q", result, expected)
	}
}

// Test 10: buildNbdCopyCommand without socket and assumeZero=false
func TestBuildNbdCopyCommand_NoSocketWithoutAssumeZero(t *testing.T) {
	result := buildNbdCopyCommand("", "/dev/vda", false)
	expected := "/usr/bin/nbdcopy nbd://localhost /dev/vda --progress"
	if result != expected {
		t.Fatalf("unexpected command:\ngot:  %q\nwant: %q", result, expected)
	}
}

// Test 11: buildV2VCommand with only path (minimal options)
func TestBuildV2VCommand_MinimalOptions(t *testing.T) {
	result := buildV2VCommand("/path/to/disk", "", "", "")
	expected := "virt-v2v-in-place -i disk /path/to/disk"
	if result != expected {
		t.Fatalf("unexpected command:\ngot:  %q\nwant: %q", result, expected)
	}
}

// Test 12: buildV2VCommand with run script and boot script
func TestBuildV2VCommand_WithScripts(t *testing.T) {
	result := buildV2VCommand("/path/to/disk", "/run.sh", "/boot.sh", "")
	expected := "virt-v2v-in-place --run /run.sh --firstboot /boot.sh -i disk /path/to/disk"
	if result != expected {
		t.Fatalf("unexpected command:\ngot:  %q\nwant: %q", result, expected)
	}
}

// Test 13: buildV2VCommand with extra options only
func TestBuildV2VCommand_ExtraOptionsOnly(t *testing.T) {
	result := buildV2VCommand("/path/to/disk", "", "", "--verbose")
	expected := "virt-v2v-in-place --verbose -i disk /path/to/disk"
	if result != expected {
		t.Fatalf("unexpected command:\ngot:  %q\nwant: %q", result, expected)
	}
}

// Test 14: buildV2VCommand with all options
func TestBuildV2VCommand_AllOptions(t *testing.T) {
	result := buildV2VCommand("/path/to/disk", "/run.sh", "/boot.sh", "--verbose --debug")
	expected := "virt-v2v-in-place --run /run.sh --firstboot /boot.sh --verbose --debug -i disk /path/to/disk"
	if result != expected {
		t.Fatalf("unexpected command:\ngot:  %q\nwant: %q", result, expected)
	}
}
