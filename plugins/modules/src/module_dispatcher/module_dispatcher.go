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

// module_dispatcher is a single binary that dispatches to all pure-Go Ansible modules
// based on the basename of argv[0].  Each module is installed as a symlink to
// this binary so the tarball stores the executable data only once, staying
// well under the Ansible Galaxy 20 MB upload limit.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"vmware-migration-kit/plugins/modules/src/best_match_flavor"
	"vmware-migration-kit/plugins/modules/src/create_heat_stack"
	"vmware-migration-kit/plugins/modules/src/create_network_port"
	"vmware-migration-kit/plugins/modules/src/create_server"
	"vmware-migration-kit/plugins/modules/src/flavor_info"
	"vmware-migration-kit/plugins/modules/src/generate_heat_template"
	"vmware-migration-kit/plugins/modules/src/import_flavor"
	"vmware-migration-kit/plugins/modules/src/volume_info"
	"vmware-migration-kit/plugins/modules/src/volume_metadata_info"
)

var dispatch = map[string]func(){
	"best_match_flavor":      best_match_flavor.Run,
	"create_heat_stack":      create_heat_stack.Run,
	"create_network_port":    create_network_port.Run,
	"create_server":          create_server.Run,
	"flavor_info":            flavor_info.Run,
	"generate_heat_template": generate_heat_template.Run,
	"import_flavor":          import_flavor.Run,
	"volume_info":            volume_info.Run,
	"volume_metadata_info":   volume_metadata_info.Run,
}

func main() {
	name := filepath.Base(os.Args[0])
	if fn, ok := dispatch[name]; ok {
		fn()
		return
	}
	fmt.Fprintf(os.Stderr, "module_dispatcher: unknown module %q — "+
		"must be invoked via a symlink named after a supported module\n", name)
	os.Exit(1)
}
