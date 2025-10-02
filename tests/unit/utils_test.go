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
	"testing"
	moduleutils "vmware-migration-kit/plugins/module_utils"
	"fmt"
	"io/fs"
)

// stunt doubles for file system
type MockFs struct {
	MockDirContents  []fs.DirEntry
	MockDirError     error
	MockSymlinkPath  string
	MockSymlinkError error
}

// acts like filesystem
func (m *MockFs) ReadDir(name string) ([]fs.DirEntry, error) {
	return m.MockDirContents, m.MockDirError
}

func (m *MockFs) EvalSymlinks(path string) (string, error) {
	return m.MockSymlinkPath, m.MockSymlinkError
}

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
	_, err := moduleutils.FindDevName(nil, "shortID")
	if err == nil {
		t.Fatalf("Expected an error for a short volumeID, but got nil")
	}
}

func TestFindDevName_EmptyVolumeID(t *testing.T) {
	_, err := moduleutils.FindDevName(nil, "")
	if err == nil {
		t.Fatalf("Expected an error for an empty volumeID, but got nil")
	}
}

// succesful scenario - correct device, one match
func TestFindDevName_Success(t *testing.T) {
    mockFs := &MockFs{
        MockDirContents: []fs.DirEntry{
            &mockDirEntry{name: "36001405e9f12345678-and-other-stuff"},
        },
        MockSymlinkPath: "/dev/sda",
    }

    volumeID := "36001405e9f123456789abcdef-and-more"
    expectedDevice := "/dev/sda"

    device, err := moduleutils.FindDevName(mockFs, volumeID)

    if err != nil {
        t.Fatalf("FindDevName returned an error: %v", err)
    }
    if device != expectedDevice {
        t.Errorf("Expected device %s, got %s", expectedDevice, device)
    }
}

// multiple devices, one match - correct device when more dirs
func TestFindDevName_MultipleDevicesOneMatch(t *testing.T) {
	mockFs := &MockFs{
		MockDirContents: []fs.DirEntry{
			&mockDirEntry{name: "some-other-disk-id"},
			&mockDirEntry{name: "prefix-123456789012345678-suffix"},
			&mockDirEntry{name: "another-unrelated-disk"},
		},
		MockSymlinkPath: "/dev/sdb",
	}
	volumeID := "123456789012345678-and-more"
	expectedDevice := "/dev/sdb"

	device, err := moduleutils.FindDevName(mockFs, volumeID)
	if err != nil {
		t.Fatalf("FindDevName returned an error: %v", err)
	}
	if device != expectedDevice {
		t.Errorf("Expected device %s, got %s", expectedDevice, device)
	}
}

// multiple matches - confirm it returned first correct match
func TestFindDevName_MultipleMatches(t *testing.T) {
	mockFs := &MockFs{
		MockDirContents: []fs.DirEntry{
			&mockDirEntry{name: "scsi-36001405e9f123456789abcdef"},
			&mockDirEntry{name: "scsi-36001405e9f123456789abcdef-extra"},
		},
		MockSymlinkPath: "/dev/sdc",
	}
	volumeID := "36001405e9f123456789abcdef-and-more"
	expectedDevice := "/dev/sdc"

	device, err := moduleutils.FindDevName(mockFs, volumeID)
	if err != nil {
		t.Fatalf("FindDevName returned an error: %v", err)
	}
	if device != expectedDevice {
		t.Errorf("Expected device %s, got %s", expectedDevice, device)
	}
}

 // no matches - "not found" but fails gracefully
func TestFindDevName_NoMatches(t *testing.T) {
	mockFs := &MockFs{
		MockDirContents: []fs.DirEntry{
			&mockDirEntry{name: "some-other-disk-id"},
			&mockDirEntry{name: "another-unrelated-disk"},
		},
	}
	volumeID := "36001405e9f123456789abcdef-and-more"

	device, err := moduleutils.FindDevName(mockFs, volumeID)
	if err != nil {
		t.Fatalf("FindDevName returned an error: %v", err)
	}
	if device != "" {
		t.Errorf("Expected empty device string for no matches, got %s", device)
	}
}

// dir doesnt exist or unreadable
func TestFindDevName_ReadDirError(t *testing.T) {
	mockFs := &MockFs{
		MockDirError: fmt.Errorf("simulated ReadDir error"),
	}
	volumeID := "36001405e9f123456789abcdef-and-more"

	_, err := moduleutils.FindDevName(mockFs, volumeID)
	if err == nil {
		t.Fatalf("Expected an error from ReadDir, but got nil")
	}
	if err.Error() != "simulated ReadDir error" {
		t.Errorf("Expected 'simulated ReadDir error', got %v", err)
	}
}

// empty directory
func TestFindDevName_EmptyDirectory(t *testing.T) {
	mockFs := &MockFs{
		MockDirContents: []fs.DirEntry{},
	}
	volumeID := "36001405e9f123456789abcdef-and-more"

	device, err := moduleutils.FindDevName(mockFs, volumeID)
	if err != nil {
		t.Fatalf("FindDevName returned an error: %v", err)
	}
	if device != "" {
		t.Errorf("Expected empty device string for empty directory, got %s", device)
	}
}

// broken symlink
func TestFindDevName_BrokenSymlink(t *testing.T) {
	mockFs := &MockFs{
		MockDirContents: []fs.DirEntry{
			&mockDirEntry{name: "scsi-36001405e9f123456789abcdef"},
		},
		MockSymlinkError: fmt.Errorf("simulated EvalSymlinks error"),
	}
	volumeID := "36001405e9f123456789abcdef-and-more"

	_, err := moduleutils.FindDevName(mockFs, volumeID)
	if err == nil {
		t.Fatalf("Expected an error from EvalSymlinks, but got nil")
	}
	if err.Error() != "simulated EvalSymlinks error" {
		t.Errorf("Expected 'simulated EvalSymlinks error', got %v", err)
	}
}