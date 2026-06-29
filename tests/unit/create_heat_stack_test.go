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
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	createheatstack "vmware-migration-kit/plugins/modules/src/create_heat_stack"
)

func TestWriteStackInfoFile(t *testing.T) {
	dir := t.TempDir()
	stack := createheatstack.StackInfo{
		Name:   "os-migrate-test",
		ID:     "abc-123",
		Status: "CREATE_COMPLETE",
	}
	templatePath := "/opt/os-migrate/template.yaml"

	infoPath, err := createheatstack.WriteStackInfoFile(dir, stack, templatePath)
	if err != nil {
		t.Fatalf("WriteStackInfoFile failed: %v", err)
	}

	expectedPath := filepath.Join(dir, "heat_stack_info.txt")
	if infoPath != expectedPath {
		t.Errorf("expected info path %q, got %q", expectedPath, infoPath)
	}

	content, err := os.ReadFile(infoPath)
	if err != nil {
		t.Fatalf("failed to read info file: %v", err)
	}

	want := strings.Join([]string{
		"Stack Name: os-migrate-test",
		"Stack ID: abc-123",
		"Status: CREATE_COMPLETE",
		"Template: /opt/os-migrate/template.yaml",
	}, "\n") + "\n"
	if string(content) != want {
		t.Errorf("unexpected content:\n%s", string(content))
	}
}

func TestWriteStackInfoFile_InvalidDir(t *testing.T) {
	stack := createheatstack.StackInfo{
		Name:   "os-migrate-test",
		ID:     "abc-123",
		Status: "CREATE_COMPLETE",
	}

	_, err := createheatstack.WriteStackInfoFile("/nonexistent/path/that/does/not/exist", stack, "/t.yaml")
	if err == nil {
		t.Fatal("expected error writing to invalid directory")
	}
}

func TestModuleArgsUnmarshalOutputDir(t *testing.T) {
	raw := `{"output_dir": "/opt/os-migrate", "stack_name": "test-stack"}`
	var args createheatstack.ModuleArgs
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if args.OutputDir != "/opt/os-migrate" {
		t.Errorf("expected output_dir '/opt/os-migrate', got %q", args.OutputDir)
	}
}

func TestResponseInfoPathJSON(t *testing.T) {
	withPath := createheatstack.Response{
		Msg:      "Heat stack created successfully",
		Changed:  true,
		InfoPath: "/opt/os-migrate/heat_stack_info.txt",
		Stack: createheatstack.StackInfo{
			ID:     "stack-id",
			Name:   "os-migrate-test",
			Status: "CREATE_COMPLETE",
		},
	}

	data, err := json.Marshal(withPath)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if !strings.Contains(string(data), `"info_path":"/opt/os-migrate/heat_stack_info.txt"`) {
		t.Errorf("expected info_path in JSON, got %s", string(data))
	}

	withoutPath := createheatstack.Response{
		Msg:     "Heat stack created successfully",
		Changed: true,
	}
	data, err = json.Marshal(withoutPath)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if strings.Contains(string(data), "info_path") {
		t.Errorf("expected info_path omitted when empty, got %s", string(data))
	}
}
