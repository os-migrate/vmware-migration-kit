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
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"vmware-migration-kit/vmware_migration_kit/plugins/module_utils/logger"
	"vmware-migration-kit/vmware_migration_kit/plugins/module_utils/vmware"
)

type NbdkitConfig struct {
	User        string
	Password    string
	Server      string
	Libdir      string
	VmName      string
	Compression string
	UUID        string
	UseSocks    bool
	VddkConfig  *vmware.VddkConfig
}

type NbdkitServer struct {
	cmd    *exec.Cmd
	socket string
}

func (c *NbdkitConfig) RunNbdKit(diskName string) (*NbdkitServer, error) {
	if c.UseSocks {
		return c.RunNbdKitSocks(diskName)
	} else {
		return c.RunNbdKitURI(diskName)
	}
}

func (c *NbdkitConfig) RunNbdKitURI(diskName string) (*NbdkitServer, error) {
	thumbprint, err := vmware.GetThumbprint(c.Server, "443")
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(
		"nbdkit",
		"--readonly",
		"--exit-with-parent",
		"--foreground",
		"vddk",
		fmt.Sprintf("server=%s", c.Server),
		fmt.Sprintf("user=%s", c.User),
		fmt.Sprintf("password=%s", c.Password),
		fmt.Sprintf("thumbprint=%s", thumbprint),
		fmt.Sprintf("libdir=%s", c.Libdir),
		fmt.Sprintf("vm=moref=%s", c.VddkConfig.VirtualMachine.Reference().Value),
		fmt.Sprintf("snapshot=%s", c.VddkConfig.SnapshotReference.Value),
		fmt.Sprintf("compression=%s", c.Compression),
		"transports=file:nbdssl:nbd",
		diskName,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		logger.Log.Infof("Failed to start nbdkit: %v", err)
		return nil, err
	}
	logger.Log.Infof("nbdkit started...")
	logger.Log.Infof("Command: %v", cmd)

	time.Sleep(100 * time.Millisecond)
	err = WaitForNbdkitURI("localhost", "10809", 30*time.Second)
	if err != nil {
		logger.Log.Infof("Failed to wait for nbdkit: %v", err)
		if cmd.Process != nil {
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil, err
	}

	return &NbdkitServer{
		cmd:    cmd,
		socket: "",
	}, nil
}

func (c *NbdkitConfig) RunNbdKitSocks(diskName string) (*NbdkitServer, error) {
	thumbprint, err := vmware.GetThumbprint(c.Server, "443")
	if err != nil {
		return nil, err
	}
	socket := fmt.Sprintf("/tmp/nbdkit-%s-%s.sock", c.VmName, c.UUID)
	cmd := exec.Command(
		"nbdkit",
		"--readonly",
		"--exit-with-parent",
		"--foreground",
		"--unix", socket,
		"vddk",
		fmt.Sprintf("server=%s", c.Server),
		fmt.Sprintf("user=%s", c.User),
		fmt.Sprintf("password=%s", c.Password),
		fmt.Sprintf("thumbprint=%s", thumbprint),
		fmt.Sprintf("libdir=%s", c.Libdir),
		fmt.Sprintf("vm=moref=%s", c.VddkConfig.VirtualMachine.Reference().Value),
		fmt.Sprintf("snapshot=%s", c.VddkConfig.SnapshotReference.Value),
		fmt.Sprintf("compression=%s", c.Compression),
		"transports=file:nbdssl:nbd",
		diskName,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		logger.Log.Infof("Failed to start nbdkit: %v", err)
		return nil, err
	}
	logger.Log.Infof("nbdkit started...")
	logger.Log.Infof("Command: %v", cmd)

	time.Sleep(100 * time.Millisecond)
	err = WaitForNbdkit(socket, 30*time.Second)
	if err != nil {
		logger.Log.Infof("Failed to wait for nbdkit: %v", err)
		if cmd.Process != nil {
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			removeSocket(socket)
		}
		return nil, err
	}

	return &NbdkitServer{
		cmd:    cmd,
		socket: socket,
	}, nil
}

func (s *NbdkitServer) Stop() error {
	if err := syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL); err != nil {
		logger.Log.Infof("Failed to stop nbdkit server: %v", err)
		return fmt.Errorf("failed to stop nbdkit server: %w", err)
	}
	logger.Log.Infof("Nbdkit server stopped.")
	removeSocket(s.socket)
	return nil
}

func (s *NbdkitServer) GetSocketPath() string {
	if s.socket == "" {
		return ""
	}
	return fmt.Sprintf("nbd+unix:///?socket=%s", s.socket)
}

func removeSocket(socketPath string) {
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		logger.Log.Infof("Failed to remove Unix socket: %v", err)
	} else {
		logger.Log.Infof("Unix socket %s removed successfully.", socketPath)
	}
}

func WaitForNbdkitURI(host string, port string, timeout time.Duration) error {
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

func WaitForNbdkit(socket string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if _, err := os.Stat(socket); err == nil {
			logger.Log.Infof("nbdkit is ready.")
			return nil
		}
		logger.Log.Infof("Waiting for nbdkit to be ready...")
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timed out waiting for nbdkit to be ready")
}

func NbdCopy(socket, device string) error {
	var nbdcopy string
	if socket == "" {
		nbdcopy = fmt.Sprintf("/usr/bin/nbdcopy nbd://localhost %s --destination-is-zero --progress", device)
	} else {
		nbdcopy = fmt.Sprintf("/usr/bin/nbdcopy %s %s --destination-is-zero --progress", socket, device)
	}
	cmd := exec.Command("bash", "-c", nbdcopy)
	logger.Log.Infof("Running nbdcopy: %v", cmd)
	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		logger.Log.Infof("Failed to run nbdcopy: %v", err)
		return err
	}

	go func() {
		reader := bufio.NewReader(stdoutPipe)
		for {
			line, err := reader.ReadString('\n')
			if line != "" {
				logger.Log.Infof("[nbdcopy stdout] %s", line)
			}
			if err != nil {
				if err != io.EOF {
					logger.Log.Infof("Error reading stdout: %v", err)
				}
				break
			}
		}
	}()

	go func() {
		reader := bufio.NewReader(stderrPipe)
		for {
			line, err := reader.ReadString('\n')
			if line != "" {
				logger.Log.Infof("[nbdcopy stderr] %s", line)
			}
			if err != nil {
				if err != io.EOF {
					logger.Log.Infof("Error reading stderr: %v", err)
				}
				break
			}
		}
	}()
	if err := cmd.Wait(); err != nil {
		logger.Log.Infof("Failed to run nbdcopy: %v", err)
		return err
	}
	return nil
}

func streamLogs(pipe io.ReadCloser, label string) {
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		logger.Log.Infof("[nbdcopy %s] %s", label, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		logger.Log.Infof("Error reading %s: %v", label, err)
	}
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

func V2VConversion(path, bsPath string, debug bool) error {
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
	if debug {
		os.Setenv("LIBGUESTFS_DEBUG", "1")
		os.Setenv("LIBGUESTFS_TRACE", "1")
	}
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
		reader := bufio.NewReader(stdoutPipe)
		for {
			line, err := reader.ReadString('\n')
			if line != "" {
				logger.Log.Infof("[nbdcopy stdout] %s", line)
			}
			if err != nil {
				if err != io.EOF {
					logger.Log.Infof("Error reading stdout: %v", err)
				}
				break
			}
		}
	}()

	go func() {
		reader := bufio.NewReader(stderrPipe)
		for {
			line, err := reader.ReadString('\n')
			if line != "" {
				logger.Log.Infof("[nbdcopy stderr] %s", line)
			}
			if err != nil {
				if err != io.EOF {
					logger.Log.Infof("Error reading stderr: %v", err)
				}
				break
			}
		}
	}()
	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}
