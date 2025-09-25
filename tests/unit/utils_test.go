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
	"errors"
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
func TestFindDevName_ShortVolumeID(t *testing.T)
{
	_, err := moduleutils.FindDevName("shortID")
	if err == nil {
		t.Fatalf("Expected an error for a short volumeID, but got nil")
	}
}

func TestFindDevName_EmptyVolumeID(t *testing.T)
{
	_, err := moduleutils.FindDevName("")
	if err == nil {
		t.Fatalf("Expected an error for an empty volumeID, but got nil")
	}
}

// TODO: (possibly with mocking)
// succesful scenario - correct device, one match
// multiple devices, one match - correct device when more dirs
// multiple matches - confirm it returned first correct match
// no matches - "not found" but fails gracefully
// empty directory - ??
// dir doesnt exist or unreadable - report it probably
// broken symlink - ??
