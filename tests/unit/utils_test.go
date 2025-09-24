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

package moduleutils

import (
	"testing"
	moduleutils "vmware-migration-kit/plugins/module_utils"
)

func TestGenRandom(t *testing.T) {
	length := 10
	str, err := moduleutils.GenRandom(length)
	if err != nil {
		t.Fatalf("GenRandom returned an error: %v", err)
	}
	if len(str) != length {
		t.Errorf("Expected string length %d, got %d", length, len(str))
	}
}
