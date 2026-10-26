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
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	moduleutils "vmware-migration-kit/plugins/module_utils"
	"vmware-migration-kit/plugins/module_utils/logger"
	"vmware-migration-kit/plugins/module_utils/vmware"
)

// formatOptValue renders an option value coming from an Ansible dict (decoded
// from JSON) as a string. JSON numbers decode to float64, so whole numbers are
// rendered without a trailing ".0".
func formatOptValue(v interface{}) string {
	switch val := v.(type) {
	case float64:
		if val == float64(int64(val)) {
			return strconv.FormatInt(int64(val), 10)
		}
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(val)
	case string:
		return val
	default:
		return fmt.Sprintf("%v", v)
	}
}

// renderOptions turns an option map into a sorted slice of "<prefix>key=value"
// tokens. Keys are sorted so the resulting command is deterministic. It returns
// nil when there are no options, so the feature stays off unless explicitly set.
func renderOptions(opts map[string]interface{}, prefix string) []string {
	if len(opts) == 0 {
		return nil
	}
	keys := make([]string, 0, len(opts))
	for k := range opts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	tokens := make([]string, 0, len(keys))
	for _, k := range keys {
		tokens = append(tokens, fmt.Sprintf("%s%s=%s", prefix, k, formatOptValue(opts[k])))
	}
	return tokens
}

func redactedCmd(cmd *exec.Cmd) string {
	args := make([]string, len(cmd.Args))
	for i, arg := range cmd.Args {
		if strings.HasPrefix(arg, "password=") {
			args[i] = "password=***"
		} else {
			args[i] = arg
		}
	}
	return strings.Join(args, " ")
}

type NbdkitConfig struct {
	User         string
	Password     string
	Server       string
	Libdir       string
	VmName       string
	Compression  string
	UUID         string
	UseSocks     bool
	Insecure     bool
	CustomPlugin string
	VddkConfig   *vmware.VddkConfig
	// Options holds extra "key=value" parameters appended to the nbdkit command
	// (for example to enable multi-connection: multiconn=true sessions=8
	// pipeline=16). Empty by default so behaviour is unchanged unless the user
	// explicitly sets them.
	Options map[string]interface{}
}

type NbdkitServer struct {
	cmd    *exec.Cmd
	socket string
}

func (c *NbdkitConfig) RunNbdKitFromLocal(diskName, diskPath string) (*NbdkitServer, error) {
	path := path.Join(diskPath, diskName)
	safeVmName := moduleutils.SafeVmName(c.VmName)
	socket := fmt.Sprintf("/tmp/nbdkit-%s-%s.sock", safeVmName, c.UUID)
	args := []string{
		"--readonly",
		"--exit-with-parent",
		"--foreground",
		"--unix", socket,
		"vddk",
		fmt.Sprintf("libdir=%s", c.Libdir),
	}
	args = append(args, renderOptions(c.Options, "")...)
	args = append(args, path)
	cmd := exec.Command("nbdkit", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		logger.Log.Infof("Failed to start nbdkit: %v", err)
		return nil, err
	}
	logger.Log.Infof("nbdkit started...")
	logger.Log.Infof("Command: %s", redactedCmd(cmd))
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
	thumbprint, err := vmware.GetThumbprint(c.Server, "443", c.Insecure)
	if err != nil {
		return nil, err
	}

	var args []string
	if c.CustomPlugin != "" {
		args = []string{
			"--readonly",
			"--exit-with-parent",
			"--foreground",
			c.CustomPlugin,
			fmt.Sprintf("server=%s", c.Server),
			fmt.Sprintf("user=%s", c.User),
			fmt.Sprintf("password=%s", c.Password),
			fmt.Sprintf("thumbprint=%s", thumbprint),
			fmt.Sprintf("vm=moref=%s", c.VddkConfig.VirtualMachine.Reference().Value),
			fmt.Sprintf("snapshot=%s", c.VddkConfig.SnapshotReference.Value),
		}
		args = append(args, renderOptions(c.Options, "")...)
	} else {
		args = []string{
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
		}
		args = append(args, renderOptions(c.Options, "")...)
		args = append(args, diskName)
	}
	cmd := exec.Command("nbdkit", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		logger.Log.Infof("Failed to start nbdkit: %v", err)
		return nil, err
	}
	logger.Log.Infof("nbdkit started...")
	logger.Log.Infof("Command: %s", redactedCmd(cmd))

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
	thumbprint, err := vmware.GetThumbprint(c.Server, "443", c.Insecure)
	if err != nil {
		return nil, err
	}
	safeVmName := moduleutils.SafeVmName(c.VmName)
	socket := fmt.Sprintf("/tmp/nbdkit-%s-%s.sock", safeVmName, c.UUID)

	var args []string
	if c.CustomPlugin != "" {
		args = []string{
			"--readonly",
			"--exit-with-parent",
			"--foreground",
			"--unix", socket,
			c.CustomPlugin,
			fmt.Sprintf("server=%s", c.Server),
			fmt.Sprintf("user=%s", c.User),
			fmt.Sprintf("password=%s", c.Password),
			fmt.Sprintf("thumbprint=%s", thumbprint),
			fmt.Sprintf("vm=moref=%s", c.VddkConfig.VirtualMachine.Reference().Value),
			fmt.Sprintf("snapshot=%s", c.VddkConfig.SnapshotReference.Value),
		}
		args = append(args, renderOptions(c.Options, "")...)
	} else {
		args = []string{
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
		}
		args = append(args, renderOptions(c.Options, "")...)
		args = append(args, diskName)
	}
	cmd := exec.Command("nbdkit", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		logger.Log.Infof("Failed to start nbdkit: %v", err)
		return nil, err
	}
	logger.Log.Infof("nbdkit started...")
	logger.Log.Infof("Command: %s", redactedCmd(cmd))

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

// buildNbdCopyCommand constructs the nbdcopy command string. extraOpts, when
// provided, are rendered as "--key=value" flags (for example --connections=16
// --requests=64) and inserted right after the nbdcopy binary. It is empty by
// default, leaving the command unchanged unless the user explicitly sets them.
func buildNbdCopyCommand(socket, device string, assumeZero bool, extraOpts map[string]interface{}) string {
	var zeroArg string
	if assumeZero {
		zeroArg = " --destination-is-zero "
	} else {
		zeroArg = " "
	}

	extra := strings.Join(renderOptions(extraOpts, "--"), " ")
	if extra != "" {
		extra += " "
	}

	if socket == "" {
		return fmt.Sprintf("/usr/bin/nbdcopy %snbd://localhost %s%s--progress", extra, device, zeroArg)
	}
	return fmt.Sprintf("/usr/bin/nbdcopy %s%s %s%s--progress", extra, socket, device, zeroArg)
}

func NbdCopy(socket, device string, assumeZero bool, extraOpts map[string]interface{}) error {
	nbdcopy := buildNbdCopyCommand(socket, device, assumeZero, extraOpts)
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

// shellQuote wraps s in single quotes so it is passed to the shell as a single
// argument, even when it contains spaces or other special characters. Any
// embedded single quote is closed, escaped, and reopened so the result stays a
// single shell token.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// buildV2VCommand constructs the virt-v2v-in-place command string.
// Paths are shell-quoted so VM names (and therefore script/disk paths)
// containing spaces are not split into multiple shell tokens when the command
// is executed via "bash -c".
func buildV2VCommand(path, rsPath, bsPath, extraOpts string, multiDisk bool) string {
	opts := ""
	if rsPath != "" {
		opts = opts + " --run " + shellQuote(rsPath)
	}
	if bsPath != "" {
		opts = opts + " --firstboot " + shellQuote(bsPath)
	}
	if extraOpts != "" {
		opts += " " + extraOpts
	}

	inputOpt := "-i disk"
	if multiDisk {
		inputOpt = "-i libvirtxml"
	}

	return "virt-v2v-in-place" + opts + " " + inputOpt + " " + shellQuote(path)
}

func V2VConversion(path, rsPath, bsPath, extraOpts string, debug bool, multiDisk bool) error {
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

	v2vcmd := buildV2VCommand(path, rsPath, bsPath, extraOpts, multiDisk)
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
