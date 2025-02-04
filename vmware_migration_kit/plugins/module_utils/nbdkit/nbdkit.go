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

package nbdkit

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"vmware-migration-kit/vmware_migration_kit/plugins/module_utils/logger"

	"github.com/gophercloud/gophercloud"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
)

type VddkConfig struct {
	VirtualMachine *object.VirtualMachine
	SnapshotRef    types.ManagedObjectReference
}

type MigrationConfig struct {
	User         string
	Password     string
	Server       string
	Libdir       string
	VmName       string
	OSMDataDir   string
	VddkConfig   *VddkConfig
	CBTSync      bool
	OSClient     *gophercloud.ProviderClient
	ConvHostName string
}

type NbdkitServer struct {
	cmd *exec.Cmd
}

func WaitForNbdkit(host string, port string, timeout time.Duration) error {
	address := net.JoinHostPort(host, port)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 2*time.Second)
		if err == nil {
			conn.Close()
			logger.Log.Infof("nbdkit is ready.")
			return nil
		}
		logger.Log.Infof("Waiting for nbdkit to be ready...")
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timed out waiting for nbdkit to be ready")
}

func NbdCopy(device string) error {
	nbdcopy := "/usr/bin/nbdcopy nbd://localhost " + device + " --destination-is-zero --progress"
	cmd := exec.Command("bash", "-c", nbdcopy)
	logger.Log.Infof("Running nbdcopy: %v", cmd)
	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		logger.Log.Infof("Failed to run nbdcopy: %v", err)
		return err
	}
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			logger.Log.Infof("[nbdcopy stdout] %s\n", scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			logger.Log.Infof("Error reading stdout: %v\n", err)
		}
	}()
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			logger.Log.Infof("[nbdcopy stderr] %s\n", scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			logger.Log.Infof("Error reading stderr: %v\n", err)
		}
	}()
	if err := cmd.Wait(); err != nil {
		logger.Log.Infof("Failed to run nbdcopy: %v", err)
		return err
	}
	return nil
}

func findVirtV2v() (string, error) {
	paths := strings.Split(os.Getenv("PATH"), ":")
	for _, path := range paths {
		if _, err := os.Stat(path + "/virt-v2v-in-place"); err == nil {
			logger.Log.Infof("Found virt-v2v-in-place at: %s\n", path)
			return path + "/", nil
		}
	}
	logger.Log.Infof("virt-v2v-in-place not found on the file system...")
	return "", fmt.Errorf("virt-v2v-in-place not found on the file system...")
}

func checkLibvirtVersion() string {
	cmd := exec.Command("libvirtd", "--version")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		logger.Log.Infof("Error checking libvirt version: %v\n", err)
		return ""
	}
	output := strings.TrimSpace(out.String())
	versionParts := strings.Fields(output)
	if len(versionParts) > 1 {
		logger.Log.Infof("Libvirt version: %s\n", versionParts[len(versionParts)-1])
		return versionParts[len(versionParts)-1]
	}
	return ""
}

func versionIsLower(cVersion, rVersion string) bool {
	currentParts := strings.Split(cVersion, ".")
	requiredParts := strings.Split(rVersion, ".")
	for i := 0; i < len(requiredParts); i++ {
		if i >= len(currentParts) {
			return true
		}
		currentPart, _ := strconv.Atoi(currentParts[i])
		requiredPart, _ := strconv.Atoi(requiredParts[i])
		if currentPart < requiredPart {
			return true
		} else if currentPart > requiredPart {
			return false
		}
	}
	return false
}

func V2VConversion(path, bsPath string) error {
	var opts string = ""
	_, err := findVirtV2v()
	if err != nil {
		logger.Log.Infof("Failed to find virt-v2v-in-place: %v", err)
		return err
	}
	if bsPath != "" {
		_, err := os.Stat(bsPath)
		if err != nil {
			logger.Log.Infof("Failed to find firstboot script: %v", err)
			return err
		}
		opts = opts + " --run " + bsPath
	}
	os.Setenv("LIBGUESTFS_BACKEND", "direct")
	v2vcmd := "virt-v2v-in-place " + opts + " -i disk " + path
	cmd := exec.Command("bash", "-c", v2vcmd)
	logger.Log.Infof("Running virt-v2v: %v", cmd)
	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		logger.Log.Infof("Failed to run virt-v2v: %v", err)
		return err
	}
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			logger.Log.Infof("[virt-v2v stdout] %s\n", scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			logger.Log.Infof("Error reading stdout: %v\n", err)
		}
	}()
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			logger.Log.Infof("[virt-v2v stderr] %s\n", scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			logger.Log.Infof("Error reading stderr: %v\n", err)
		}
	}()
	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}
