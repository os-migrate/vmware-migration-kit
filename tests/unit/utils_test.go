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
	"io/fs"
	"os/exec"
	"strings"
	"testing"
	moduleutils "vmware-migration-kit/plugins/module_utils"
)

// populating contents of a directory
type mockDirEntry struct {
	name string
	fs.DirEntry
}

func (m *mockDirEntry) Name() string { return m.name }

func TestGenRandom(t *testing.T) {
	length := 10
	str, err := moduleutils.GenRandom(length)
	if err != nil {
		t.Fatalf("GenRandom returned an error: %v", err)
	}
	if len(str) != length {
		t.Errorf("Expected string length %d, got %d", length, len(str))
	}
}

// unexpected inputs werent handled
func TestFindDevName_ShortVolumeID(t *testing.T) {
	_, err := moduleutils.FDevName(nil, nil, "shortID")
	if err == nil {
		t.Fatalf("Expected an error for a short volumeID, but got nil")
	}
}

func TestFindDevName_EmptyVolumeID(t *testing.T) {
	_, err := moduleutils.FDevName(nil, nil, "")
	if err == nil {
		t.Fatalf("Expected an error for an empty volumeID, but got nil")
	}
}

// succesful scenario - correct device, one match
func TestFindDevName_Success(t *testing.T) {
	mockReadDir := func(path string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{
			&mockDirEntry{name: "prefix-36001405e9f12345678-suffix"},
		}, nil
	}

	mockEvalSymlinks := func(path string) (string, error) {
		return "/dev/sda", nil
	}
	volumeID := "36001405e9f12345678-and-more"
	expectedDevice := "/dev/sda"

	device, err := moduleutils.FDevName(mockReadDir, mockEvalSymlinks, volumeID)

	if err != nil {
		t.Fatalf("FindDevName returned an error: %v", err)
	}
	if device != expectedDevice {
		t.Errorf("Expected device %s, got %s", expectedDevice, device)
	}
}

// multiple devices, one match - correct device when more dirs
func TestFindDevName_MultipleDevicesOneMatch(t *testing.T) {
	mockReadDir := func(path string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{
			&mockDirEntry{name: "some-other-disk-id"},
			&mockDirEntry{name: "prefix-123456789012345678-suffix"},
			&mockDirEntry{name: "another-unrelated-disk"},
		}, nil
	}
	mockEvalSymlinks := func(path string) (string, error) {
		return "/dev/sdb", nil
	}
	volumeID := "123456789012345678-and-more"
	expectedDevice := "/dev/sdb"

	device, err := moduleutils.FDevName(mockReadDir, mockEvalSymlinks, volumeID)
	if err != nil {
		t.Fatalf("FindDevName returned an error: %v", err)
	}
	if device != expectedDevice {
		t.Errorf("Expected device %s, got %s", expectedDevice, device)
	}
}

// multiple matches - confirm it returned first correct match
func TestFindDevName_MultipleMatches(t *testing.T) {
	mockReadDir := func(path string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{
			&mockDirEntry{name: "scsi-36001405e9f123456789abcdef"},
			&mockDirEntry{name: "scsi-36001405e9f123456789abcdef-extra"},
		}, nil
	}
	mockEvalSymlinks := func(path string) (string, error) {
		return "/dev/sdc", nil
	}
	volumeID := "36001405e9f123456789abcdef-and-more"
	expectedDevice := "/dev/sdc"

	device, err := moduleutils.FDevName(mockReadDir, mockEvalSymlinks, volumeID)
	if err != nil {
		t.Fatalf("FindDevName returned an error: %v", err)
	}
	if device != expectedDevice {
		t.Errorf("Expected device %s, got %s", expectedDevice, device)
	}
}

// no matches - "not found" but fails gracefully
func TestFindDevName_NoMatches(t *testing.T) {
	mockReadDir := func(path string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{
			&mockDirEntry{name: "some-other-disk-id"},
			&mockDirEntry{name: "another-unrelated-disk"},
		}, nil
	}
	mockEvalSymlinks := func(path string) (string, error) {
		return "", nil
	}
	volumeID := "36001405e9f123456789abcdef-and-more"

	device, err := moduleutils.FDevName(mockReadDir, mockEvalSymlinks, volumeID)
	if err != nil {
		t.Fatalf("FindDevName returned an error: %v", err)
	}
	if device != "" {
		t.Errorf("Expected empty device string for no matches, got %s", device)
	}
}

// dir doesnt exist or unreadable
func TestFindDevName_ReadDirError(t *testing.T) {
	mockReadDir := func(path string) ([]fs.DirEntry, error) {
		return nil, fmt.Errorf("simulated ReadDir error")
	}
	mockEvalSymlinks := func(path string) (string, error) {
		return "", nil
	}
	volumeID := "36001405e9f123456789abcdef-and-more"

	_, err := moduleutils.FDevName(mockReadDir, mockEvalSymlinks, volumeID)
	if err == nil {
		t.Fatalf("Expected an error from ReadDir, but got nil")
	}
	if err.Error() != "simulated ReadDir error" {
		t.Errorf("Expected 'simulated ReadDir error', got %v", err)
	}
}

// empty directory
func TestFindDevName_EmptyDirectory(t *testing.T) {
	mockReadDir := func(path string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{}, nil
	}
	mockEvalSymlinks := func(path string) (string, error) {
		return "", nil
	}
	volumeID := "36001405e9f123456789abcdef-and-more"

	device, err := moduleutils.FDevName(mockReadDir, mockEvalSymlinks, volumeID)
	if err != nil {
		t.Fatalf("FindDevName returned an error: %v", err)
	}
	if device != "" {
		t.Errorf("Expected empty device string for empty directory, got %s", device)
	}
}

// broken symlink
func TestFindDevName_BrokenSymlink(t *testing.T) {
	mockReadDir := func(path string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{
			&mockDirEntry{name: "scsi-36001405e9f123456789abcdef"},
		}, nil
	}
	mockEvalSymlinks := func(path string) (string, error) {
		return "", fmt.Errorf("simulated EvalSymlinks error")
	}
	volumeID := "36001405e9f123456789abcdef-and-more"

	_, err := moduleutils.FDevName(mockReadDir, mockEvalSymlinks, volumeID)
	if err == nil {
		t.Fatalf("Expected an error from EvalSymlinks, but got nil")
	}
	if err.Error() != "simulated EvalSymlinks error" {
		t.Errorf("Expected 'simulated EvalSymlinks error', got %v", err)
	}
}

func TestSafeVmName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"vm1", "vm1"},
		{"vm-01", "vm_01"},
		{"My VM", "My_VM"},
		{"vm@#$%", "vm____"},
		{"_already_ok_", "_already_ok_"},
		{" ", "_"},
		{"Mi-VM.2025", "Mi_VM_2025"},
	}

	for _, tt := range tests {
		//Run each test in a subtest for better isolation and reporting
		t.Run(tt.input, func(t *testing.T) {
			result := moduleutils.SafeVmName(tt.input)
			if result != tt.expected {
				t.Errorf("SafeVmName(%q) = %q; want %q", tt.input, result, tt.expected)
			}
			//Verify it works in a shell command context
			cmd := exec.Command("echo", result)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("error executing command: %v", err)
			}

			output := strings.TrimSpace(string(out))
			if output != result {
				t.Errorf("exec output = %q; want %q", output, result)
			}
		})
	}
}
