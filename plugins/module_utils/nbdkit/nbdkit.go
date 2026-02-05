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
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path"
	"strings"
	"syscall"
	"time"
	moduleutils "vmware-migration-kit/plugins/module_utils"
	"vmware-migration-kit/plugins/module_utils/logger"
	"vmware-migration-kit/plugins/module_utils/vmware"
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

func (c *NbdkitConfig) RunNbdKitFromLocal(diskName, diskPath string) (*NbdkitServer, error) {
	path := path.Join(diskPath, diskName)
	safeVmName := moduleutils.SafeVmName(c.VmName)
	socket := fmt.Sprintf("/tmp/nbdkit-%s-%s.sock", safeVmName, c.UUID)
	cmd := exec.Command(
		"nbdkit",
		"--readonly",
		"--exit-with-parent",
		"--foreground",
		"--unix", socket,
		"vddk",
		fmt.Sprintf("libdir=%s", c.Libdir),
		path,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		logger.Log.Infof("Failed to start nbdkit: %v", err)
		return nil, err
	}
	logger.Log.Infof("nbdkit started...")
	logger.Log.Infof("Command: %v", cmd)
	time.Sleep(100 * time.Millisecond)
	err := WaitForNbdkit(socket, 30*time.Second)
	if err != nil {
		logger.Log.Infof("Failed to wait for nbdkit: %v", err)
		if cmd.Process != nil {
			if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
				logger.Log.Infof("Failed to kill process: %v", err)
			}
			if err := removeSocket(socket); err != nil {
				logger.Log.Infof("Failed to remove socket: %v", err)
			}
		}
		return nil, err
	}

	return &NbdkitServer{
		cmd:    cmd,
		socket: socket,
	}, nil
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
			if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
				logger.Log.Infof("Failed to kill process: %v", err)
			}
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
	safeVmName := moduleutils.SafeVmName(c.VmName)
	socket := fmt.Sprintf("/tmp/nbdkit-%s-%s.sock", safeVmName, c.UUID)
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
			if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
				logger.Log.Infof("Failed to kill process: %v", err)
			}
			if err := removeSocket(socket); err != nil {
				logger.Log.Infof("Failed to remove socket: %v", err)
			}
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
	if err := removeSocket(s.socket); err != nil {
		logger.Log.Infof("Failed to remove socket: %v", err)
		// Continue execution even if socket removal fails
	}
	return nil
}

func (s *NbdkitServer) GetSocketPath() string {
	if s.socket == "" {
		return ""
	}
	return fmt.Sprintf("nbd+unix:///?socket=%s", s.socket)
}

func removeSocket(socketPath string) error {
	if socketPath == "" {
		return nil
	}
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		logger.Log.Infof("Failed to remove Unix socket: %v", err)
		return err
	}
	logger.Log.Infof("Unix socket %s removed successfully.", socketPath)
	return nil
}

func WaitForNbdkitURI(host string, port string, timeout time.Duration) error {
	address := net.JoinHostPort(host, port)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 2*time.Second)
		if err == nil {
			if err := conn.Close(); err != nil {
				logger.Log.Infof("Failed to close connection: %v", err)
			}
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

// buildNbdCopyCommand constructs the nbdcopy command string.
func buildNbdCopyCommand(socket, device string, assumeZero bool) string {
	var zeroArg string
	if assumeZero {
		zeroArg = " --destination-is-zero "
	} else {
		zeroArg = " "
	}

	if socket == "" {
		return fmt.Sprintf("/usr/bin/nbdcopy nbd://localhost %s%s--progress", device, zeroArg)
	}
	return fmt.Sprintf("/usr/bin/nbdcopy %s %s%s--progress", socket, device, zeroArg)
}

func NbdCopy(socket, device string, assumeZero bool) error {
	nbdcopy := buildNbdCopyCommand(socket, device, assumeZero)
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

// Commented out unused function
/*
func streamLogs(pipe io.ReadCloser, label string) {
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		logger.Log.Infof("[nbdcopy %s] %s", label, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		logger.Log.Infof("Error reading %s: %v", label, err)
	}
}
*/

func findVirtV2v() (string, error) {
	paths := strings.Split(os.Getenv("PATH"), ":")
	for _, path := range paths {
		if _, err := os.Stat(path + "/virt-v2v-in-place"); err == nil {
			logger.Log.Infof("Found virt-v2v-in-place at: %s\n", path)
			return path + "/", nil
		}
	}
	logger.Log.Infof("virt-v2v-in-place not found on the file system")
	return "", fmt.Errorf("virt-v2v-in-place not found on the file system")
}

// Commented out unused function
/*
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
*/

// Commented out unused function
/*
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
*/

// buildV2VCommand constructs the virt-v2v-in-place command string.
func buildV2VCommand(path, rsPath, bsPath, extraOpts string) string {
	opts := ""
	if rsPath != "" {
		opts = opts + " --run " + rsPath
	}
	if bsPath != "" {
		opts = opts + " --firstboot " + bsPath
	}
	if extraOpts != "" {
		opts += " " + extraOpts
	}
	return "virt-v2v-in-place" + opts + " -i disk " + path
}

func V2VConversion(path, rsPath, bsPath, extraOpts string, debug bool) error {
	_, err := findVirtV2v()
	if err != nil {
		logger.Log.Infof("Failed to find virt-v2v-in-place: %v", err)
		return err
	}

	// Check for run script and boot script
	if rsPath != "" {
		_, err := os.Stat(rsPath)
		if err != nil {
			logger.Log.Infof("Failed to find run script: %v", err)
			return err
		}
	}
	if bsPath != "" {
		_, err := os.Stat(bsPath)
		if err != nil {
			logger.Log.Infof("Failed to find boot script: %v", err)
			return err
		}
	}

	if err := os.Setenv("LIBGUESTFS_BACKEND", "direct"); err != nil {
		logger.Log.Infof("Failed to set LIBGUESTFS_BACKEND: %v", err)
		// Continue even if setenv fails
	}
	if debug {
		if err := os.Setenv("LIBGUESTFS_DEBUG", "1"); err != nil {
			logger.Log.Infof("Failed to set LIBGUESTFS_DEBUG: %v", err)
		}
		if err := os.Setenv("LIBGUESTFS_TRACE", "1"); err != nil {
			logger.Log.Infof("LIBGUESTFS_TRACE: %v", err)
		}
	}

	v2vcmd := buildV2VCommand(path, rsPath, bsPath, extraOpts)
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
				logger.Log.Infof("[virt-v2v stdout] %s", line)
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
				logger.Log.Infof("[virt-v2v stderr] %s", line)
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