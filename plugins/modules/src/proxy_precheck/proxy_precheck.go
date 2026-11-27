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
 * Copyright 2026 Red Hat, Inc.
 *
 */

// proxy_precheck checks that a proxy VM can be used for the proxy transport:
// the proxy exists and is correctly configured, the conversion host can reach
// the nbdkit service running on the proxy, and every VM to migrate has disks
// the proxy can hot-add. Nothing is executed inside the proxy VM.
package proxy_precheck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"vmware-migration-kit/plugins/module_utils/proxy"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
)

// ModuleArgs are the Ansible module arguments.
type ModuleArgs struct {
	User           string   `json:"user"`
	Password       string   `json:"password"`
	Server         string   `json:"server"`
	VmwareInsecure bool     `json:"vmware_insecure"`
	Datacenter     string   `json:"datacenter"`
	ProxyVmName    string   `json:"proxy_vm_name"`
	ProxyVmIp      string   `json:"proxy_vm_ip"`
	NbdPort        int      `json:"nbd_port"`
	VmsList        []string `json:"vms_list"`
	CBTSync        bool     `json:"cbt_sync"`
	CheckNetwork   *bool    `json:"check_network"`
}

// Response is the Ansible module result.
type Response struct {
	Msg     string         `json:"msg"`
	Changed bool           `json:"changed"`
	Failed  bool           `json:"failed"`
	Checks  []proxy.Check  `json:"checks"`
	Facts   proxy.Facts    `json:"facts"`
	Summary map[string]int `json:"summary"`
}

func exit(r Response) {
	if r.Checks == nil {
		r.Checks = []proxy.Check{}
	}
	out, err := json.Marshal(r)
	if err != nil {
		out = []byte(`{"msg": "invalid response object", "failed": true}`)
		r.Failed = true
	}
	fmt.Println(string(out))
	if r.Failed {
		os.Exit(1)
	}
	os.Exit(0)
}

func fail(msg string) {
	exit(Response{Msg: msg, Failed: true})
}

// connect logs into vCenter (plain govmomi: the vmware module_utils package
// pulls libnbd/cgo, which the module_dispatcher binary must not depend on).
func connect(ctx context.Context, a ModuleArgs) (*govmomi.Client, error) {
	u, err := url.Parse("https://" + a.Server + "/sdk")
	if err != nil {
		return nil, err
	}
	u.User = url.UserPassword(a.User, a.Password)
	return govmomi.NewClient(ctx, u, a.VmwareInsecure)
}

// summarize counts checks by status and lists the failed ones.
func summarize(checks []proxy.Check) (map[string]int, []string) {
	summary := map[string]int{string(proxy.StatusOK): 0, string(proxy.StatusWarn): 0, string(proxy.StatusFail): 0}
	var failed []string
	for _, c := range checks {
		summary[string(c.Status)]++
		if c.Status == proxy.StatusFail {
			failed = append(failed, fmt.Sprintf("%s [%s]: %s", c.Name, c.Target, c.Msg))
		}
	}
	return summary, failed
}

func Run() {
	if len(os.Args) != 2 {
		fail("No argument file provided")
	}
	text, err := os.ReadFile(os.Args[1])
	if err != nil {
		fail("Could not read configuration file: " + os.Args[1])
	}
	var a ModuleArgs
	if err := json.Unmarshal(text, &a); err != nil {
		fail("Configuration file not valid JSON: " + os.Args[1])
	}
	for field, v := range map[string]string{
		"server": a.Server, "user": a.User, "password": a.Password, "proxy_vm_name": a.ProxyVmName,
	} {
		if v == "" {
			fail(field + " is required")
		}
	}
	checkNetwork := a.CheckNetwork == nil || *a.CheckNetwork

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	c, err := connect(ctx, a)
	if err != nil {
		fail("Failed to connect to vCenter " + a.Server + ": " + err.Error())
	}
	defer func() { _ = c.Logout(context.Background()) }()

	finder := find.NewFinder(c.Client, true)
	dc, err := finder.DatacenterOrDefault(ctx, a.Datacenter)
	if err != nil {
		fail("Datacenter not found: " + err.Error())
	}
	finder.SetDatacenter(dc)

	p, err := proxy.New(ctx, c.Client, finder, proxy.Config{
		VMName:  a.ProxyVmName,
		Address: a.ProxyVmIp,
		Port:    a.NbdPort,
	})
	if err != nil {
		fail(err.Error())
	}

	report := proxy.Precheck(ctx, finder, p, proxy.PrecheckOptions{
		VMs:          a.VmsList,
		CBTSync:      a.CBTSync,
		CheckNetwork: checkNetwork,
	})
	summary, failed := summarize(report.Checks)
	r := Response{Checks: report.Checks, Facts: report.Facts, Summary: summary, Failed: report.Failed}
	if report.Failed {
		r.Msg = fmt.Sprintf("Proxy VM %s pre-check failed (%d failed, %d warning(s)): %s",
			a.ProxyVmName, summary["fail"], summary["warn"], strings.Join(failed, "; "))
	} else {
		r.Msg = fmt.Sprintf("Proxy VM %s is ready (%d ok, %d warning(s))", a.ProxyVmName, summary["ok"], summary["warn"])
	}
	exit(r)
}
