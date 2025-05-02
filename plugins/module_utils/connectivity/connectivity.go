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

package connectivity

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"vmware-migration_kit/vmware_migration_kit/plugins/module_utils/ansible"
	"vmware-migration_kit/vmware_migration_kit/plugins/module_utils/logger"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

func CheckVCenterConnectivity(ctx context.Context, finder *find.Finder, c *vim25.Client, vmpath string) (*object.VirtualMachine, error) {
	var response ansible.Response
	var props mo.VirtualMachine
	statusColors := map[types.ManagedEntityStatus]string{
		types.ManagedEntityStatusGray:   "Unknown",
		types.ManagedEntityStatusGreen:  "Normal",
		types.ManagedEntityStatusYellow: "Warning",
		types.ManagedEntityStatusRed:    "Alert",
	}

	// set managed object reference
	vm, err := finder.VirtualMachine(ctx, vmpath)
	if err != nil {
		// list entire path here
		logger.Log.Infof("Failed to find VM: %v, in vm path: %v", err, vmpath)
		vms, _ := finder.VirtualMachineList(ctx, "*")

		for _, vm := range vms {
			logger.Log.Info(" - ", vm.InventoryPath)
		}
		response.Msg = "Failed to find VM: " + err.Error() + " in vm path: " + vmpath
		ansible.FailJson(response)
	}
	mor := vm.Reference()

	// fetch status of managed object instance
	if err := c.RetrieveOne(ctx, mor, []string{"guestHeartbeatStatus"}, &props); err != nil {
		logger.Log.Infof("Failed to fetch heartbeat")
		response.Msg = "Failed to fetch heartbeat: " + err.Error()
		ansible.FailJson(response)
	}

	logger.Log.Infof("vCenter connectivity check passed with status code: %s", statusColors[props.GuestHeartbeatStatus])
	return vm, nil
}
