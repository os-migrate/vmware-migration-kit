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
 * Copyright 2024 Red Hat, Inc.
 *
 */

package moduleutils

import (
	"crypto/rand"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"regexp"
	"fmt"
	"io/fs"
)

// real filesystem implementation
type RealFs struct{}

func (r *RealFs) ReadDir(name string) ([]fs.DirEntry, error) {
    return os.ReadDir(name)
}

func (r *RealFs) EvalSymlinks(path string) (string, error) {
    return filepath.EvalSymlinks(path)
}

// allows to fake filesystem
type Filesystem interface {
    ReadDir(name string) ([]fs.DirEntry, error)
    EvalSymlinks(path string) (string, error)
}

func FindDevName(fs Filesystem, volumeID string) (string, error) {
	if len(volumeID) < 18 {
    return "", fmt.Errorf("volumeID must be at least 18 characters long")
	}
	files, err := fs.ReadDir("/dev/disk/by-id/")
	if err != nil {
		return "", err
	}
	for _, file := range files {
		if strings.Contains(file.Name(), volumeID[:18]) {
			devicePath, err := fs.EvalSymlinks(filepath.Join("/dev/disk/by-id/", file.Name()))
			if err != nil {
				return "", err
			}

			return devicePath, nil
		}
	}
	return "", nil
}

func GenRandom(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		result[i] = charset[num.Int64()]
	}
	return string(result), nil
}

func SafeVmName(vmName string) string {
    re := regexp.MustCompile(`[^a-zA-Z0-9_]`)
    return re.ReplaceAllString(vmName, "_")
}