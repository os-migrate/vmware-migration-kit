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

package vmware

import (
	"context"
	"crypto/sha1"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

var logger *log.Logger
var logFile string = "/tmp/osm-nbdkit.log"

func init() {
	logFile, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	logger = log.New(logFile, "osm-nbdkit: ", log.LstdFlags|log.Lshortfile)
}

func ProcessUrl(u *url.URL, user string, password string) {
	if user != "" {
		u.User = url.UserPassword(user, password)
	}
}

func GetThumbprint(host string, port string) (string, error) {
	config := tls.Config{
		InsecureSkipVerify: true,
	}
	if port == "" {
		port = "443"
	}

	conn, err := tls.Dial("tcp", fmt.Sprintf("%s:%s", host, port), &config)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	if len(conn.ConnectionState().PeerCertificates) == 0 {
		logger.Printf("No certificates found")
		return "", errors.New("no certificates found")
	}

	certificate := conn.ConnectionState().PeerCertificates[0]
	sha1Bytes := sha1.Sum(certificate.Raw)

	thumbprint := make([]string, len(sha1Bytes))
	for i, b := range sha1Bytes {
		thumbprint[i] = fmt.Sprintf("%02X", b)
	}

	return strings.Join(thumbprint, ":"), nil
}

func GetDiskKey(ctx context.Context, vm *object.VirtualMachine) ([]int32, error) {
	var diskKeys []int32
	var vmProperties mo.VirtualMachine
	err := vm.Properties(ctx, vm.Reference(), []string{"config.hardware.device"}, &vmProperties)
	if err != nil {
		logger.Printf("Failed to retrieve VM properties: %v", err)
		return nil, err
	}
	for _, device := range vmProperties.Config.Hardware.Device {
		if virtualDisk, ok := device.(*types.VirtualDisk); ok {
			diskKeys = append(diskKeys, virtualDisk.VirtualDevice.Key)
		}
	}
	return diskKeys, nil
}

func GetCBTChangeID(ctx context.Context, vm *object.VirtualMachine, diskLabel string) (string, error) {
	var conf mo.VirtualMachine
	err := vm.Properties(ctx, vm.Reference(), []string{"config"}, &conf)
	if err != nil {
		logger.Printf("Failed to retrieve VM properties: %v", err)
		return "", err
	}
	if conf.Config.ChangeTrackingEnabled == nil || !*conf.Config.ChangeTrackingEnabled {
		logger.Printf("CBT not enabled")
		return "", nil
	} else {
		logger.Printf("CBT enabled")
	}
	for _, device := range conf.Config.Hardware.Device {
		if disk, ok := device.(*types.VirtualDisk); ok {
			backing := disk.Backing.(*types.VirtualDiskFlatVer2BackingInfo)
			// Match disk by its label or backing file name.
			if backing.FileName == diskLabel || backing.DiskMode == diskLabel {
				// Return the CBT ChangeID.
				return backing.ChangeId, nil
			}
		}
	}
	return "", fmt.Errorf("disk with label '%s' not found on VM", diskLabel)
}
