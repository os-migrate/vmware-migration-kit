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
	"log"
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"
	"vmware-migration-kit/vmware_migration_kit/plugins/module_utils/vmware"

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

var logger *log.Logger
var logFile string = "/tmp/osm-nbdkit.log"

func init() {
	logFile, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	logger = log.New(logFile, "osm-nbdkit: ", log.LstdFlags|log.Lshortfile)
}

func WaitForNbdkit(host string, port string, timeout time.Duration) error {
	address := net.JoinHostPort(host, port)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 2*time.Second)
		if err == nil {
			conn.Close()
			logger.Printf("nbdkit is ready.")
			return nil
		}
		logger.Printf("Waiting for nbdkit to be ready...")
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timed out waiting for nbdkit to be ready")
}

func RunNbdKit(c MigrationConfig, diskName string) (*NbdkitServer, error) {
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
		fmt.Sprintf("snapshot=%s", c.VddkConfig.SnapshotRef.Value),
		"compression=zlib",
		"transports=file:nbdssl:nbd",
		diskName,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		logger.Printf("Failed to start nbdkit: %v", err)
		return nil, err
	}
	logger.Printf("nbdkit started...")
	logger.Printf("Command: %v", cmd)

	time.Sleep(100 * time.Millisecond)
	err = WaitForNbdkit("localhost", "10809", 30*time.Second)
	if err != nil {
		logger.Printf("Failed to wait for nbdkit: %v", err)
		if cmd.Process != nil {
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil, err
	}

	return &NbdkitServer{
		cmd: cmd,
	}, nil
}

func (s *NbdkitServer) Stop() error {
	if err := syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL); err != nil {
		logger.Printf("Failed to stop nbdkit server: %v", err)
		return fmt.Errorf("failed to stop nbdkit server: %w", err)
	}
	return nil
}

func NbdCopy(device string) error {
	nbdcopy := "/usr/bin/nbdcopy nbd://localhost " + device + " --destination-is-zero --progress"
	cmd := exec.Command("bash", "-c", nbdcopy)
	logger.Printf("Running nbdcopy: %v", cmd)
	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		logger.Printf("Failed to run nbdcopy: %v", err)
		return err
	}
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			logger.Printf("[nbdcopy stdout] %s\n", scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			logger.Printf("Error reading stdout: %v\n", err)
		}
	}()
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			logger.Printf("[nbdcopy stderr] %s\n", scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			logger.Printf("Error reading stderr: %v\n", err)
		}
	}()
	if err := cmd.Wait(); err != nil {
		logger.Printf("Failed to run nbdcopy: %v", err)
		return err
	}
	return nil
}

func V2VConversion(v2vpath string, path string) error {
	//@TODO: add function to search for virt-v2v-in-place, depending on the OS
	// the binary could be in different locations
	v2vcmd := "virt-v2v-in-place --no-selinux-relabel -i disk " + path
	cmd := exec.Command("bash", "-c", v2vcmd)
	logger.Printf("Running virt-v2v: %v", cmd)
	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		logger.Printf("Failed to run virt-v2v: %v", err)
		return err
	}
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			logger.Printf("[virt-v2v stdout] %s\n", scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			logger.Printf("Error reading stdout: %v\n", err)
		}
	}()
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			logger.Printf("[virt-v2v stderr] %s\n", scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			logger.Printf("Error reading stderr: %v\n", err)
		}
	}()
	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}
