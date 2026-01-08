package moduleutils

import (
	"encoding/json"
	"strings"
	"testing"
	"vmware-migration-kit/plugins/module_utils/ansible"
)

// Test 1: Field has a value
// If I give you a real value, give it back to me, don't use the default.
func TestDefaultIfEmpty_FieldHasValue(t *testing.T) {
	result := ansible.DefaultIfEmpty("test", "default")
	if result != "test" {
		t.Errorf("Expected 'test', but got '%s'", result)
	}
}

// Test 2: Field is empty
// If I give you nothing, give me the backup value.
func TestDefaultIfEmpty_FieldIsEmpty(t *testing.T) {
	result := ansible.DefaultIfEmpty("", "default")
	if result != "default" {
		t.Errorf("Expected 'default', but got '%s'", result)
	}
}

// Test 3: Both are empty
// If both are empty, I should get empty back.
func TestDefaultIfEmpty_BothAreEmpty(t *testing.T) {
	result := ansible.DefaultIfEmpty("", "")
	if result != "" {
		t.Errorf("Expected '', but got '%s'", result)
	}
}

// Test 4: Field has spaces
// Spaces count as a value. Don't replace them with the default.
func TestDefaultIfEmpty_FieldHasSpaces(t *testing.T) {
	result := ansible.DefaultIfEmpty("  ", "default")
	if result != "  " {
		t.Errorf("Expected '  '(spaces), but got '%s'", result)
	}
}

// Test 5: Field has value
// If I give you a real value, give it back to me, don't use the default.
func TestRequireField_FieldHasValue(t *testing.T) {
	result := ansible.RequireField("myvalue", "error message")
	if result != "myvalue" {
		t.Errorf("Expected 'myvalue', but got '%s'", result)
	}
}

// test 6: Field is empty
// If I give you nothing, call the fail handler with the error message.
func TestRequireField_FieldIsEmpty(t *testing.T) {
	failCalled := false
	failMessage := ""

	mockFail := func(msg string) {
		failCalled = true
		failMessage = msg
	}

	ansible.RequireFieldWithDeps("", "Field is required", mockFail)

	if !failCalled {
		t.Error("Expected fail handler to be called")
	}
	if failMessage != "Field is required" {
		t.Errorf("Expected 'Field is required', got '%s'", failMessage)
	}
}

// test 7: jsonmarshal
// Can the Response struct survive being converted to JSON and back, is the data preserved
func TestResponse_JSONMarshal(t *testing.T) {
	response := ansible.Response{
		Msg:     "Success",
		Changed: true,
		Failed:  false,
		ID:      []string{"vol-123"},
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded ansible.Response
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Msg != "Success" {
		t.Errorf("Expected 'Success', got '%s'", decoded.Msg)
	}
}

// test 8: jsonmarshal
// Can MigrateResponse with its nested Disk array survive JSON conversion
func TestMigrateResponse_JSONMarshal(t *testing.T) {
	response := ansible.MigrateResponse{
		Msg:     "Done",
		Changed: true,
		Disks:   []ansible.Disk{{ID: "disk-1", Primary: true}},
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded ansible.MigrateResponse
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(decoded.Disks) != 1 {
		t.Errorf("Expected 1 disk, got %d", len(decoded.Disks))
	}
}

// Test 9: ExitJson exits with code 0
// If I give you a success message, exit with code 0 and print the message.
func TestExitJson_ExitsWithCode0(t *testing.T) {
	var exitCode int
	var output string

	mockExit := func(code int) { exitCode = code }
	mockPrint := func(s string) { output = s }

	response := ansible.Response{
		Msg:     "Success",
		Changed: true,
		Failed:  false,
	}

	ansible.ExitJsonWithDeps(response, mockExit, mockPrint)

	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(output, "Success") {
		t.Errorf("Output should contain 'Success', got '%s'", output)
	}
}

// Test 10: FailJson sets Failed=true and exits with 1
// If I give you an error message, set Failed=true and exit with code 1.
func TestFailJson_SetsFailedAndExitsWithCode1(t *testing.T) {
	var exitCode int
	var output string

	mockExit := func(code int) { exitCode = code }
	mockPrint := func(s string) { output = s }

	response := ansible.Response{
		Msg:    "Error occurred",
		Failed: false, // Will be set to true by FailJson
	}

	ansible.FailJsonWithDeps(response, mockExit, mockPrint)

	if exitCode != 1 {
		t.Errorf("Expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(output, `"failed":true`) {
		t.Errorf("Output should contain '\"failed\":true', got '%s'", output)
	}
}

// Test 11: FailWithMessage creates error response
// If I just give an error message, create a full error response and exit.
func TestFailWithMessage_CreatesErrorResponse(t *testing.T) {
	var exitCode int
	var output string

	mockExit := func(code int) { exitCode = code }
	mockPrint := func(s string) { output = s }

	ansible.FailWithMessageWithDeps("Database connection failed", mockExit, mockPrint)

	if exitCode != 1 {
		t.Errorf("Expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(output, "Database connection failed") {
		t.Errorf("Output should contain error message, got '%s'", output)
	}
}

// Test 12: ReturnResponse outputs valid JSON
// If I give you a valid Response struct, convert it to JSON and print it.
func TestReturnResponse_OutputsValidJSON(t *testing.T) {
	var output string

	mockExit := func(code int) {}
	mockPrint := func(s string) { output = s }

	response := ansible.Response{
		Msg:     "Test",
		Changed: true,
		Failed:  false,
		ID:      []string{"id-1"},
	}

	ansible.ReturnResponseWithDeps(response, mockExit, mockPrint)

	// Verify it's valid JSON by trying to parse it
	var decoded ansible.Response
	err := json.Unmarshal([]byte(output), &decoded)
	if err != nil {
		t.Errorf("Output is not valid JSON: %v", err)
	}
	if decoded.Msg != "Test" {
		t.Errorf("Expected Msg 'Test', got '%s'", decoded.Msg)
	}
}

// Test 13: ReturnResponse exits with code 1 when Failed=true
func TestReturnResponse_FailedExitsWithCode1(t *testing.T) {
	var exitCode int

	mockExit := func(code int) { exitCode = code }
	mockPrint := func(s string) {}

	response := ansible.Response{
		Msg:    "Error",
		Failed: true,
	}

	ansible.ReturnResponseWithDeps(response, mockExit, mockPrint)

	if exitCode != 1 {
		t.Errorf("Expected exit code 1 for Failed=true, got %d", exitCode)
	}
}

// Test 14: ExitJson preserves all fields in output
func TestExitJson_PreservesAllFields(t *testing.T) {
	var output string

	mockExit := func(code int) {}
	mockPrint := func(s string) { output = s }

	response := ansible.Response{
		Msg:     "All fields test",
		Changed: true,
		Failed:  false,
		ID:      []string{"vol-1", "vol-2"},
	}

	ansible.ExitJsonWithDeps(response, mockExit, mockPrint)

	if !strings.Contains(output, "vol-1") {
		t.Error("Output should contain ID 'vol-1'")
	}
	if !strings.Contains(output, "vol-2") {
		t.Error("Output should contain ID 'vol-2'")
	}
	if !strings.Contains(output, `"changed":true`) {
		t.Error("Output should contain 'changed':true")
	}
}

// Test 15: Response with empty ID array marshals correctly
func TestResponse_EmptyIDArray(t *testing.T) {
	response := ansible.Response{
		Msg:     "No IDs",
		Changed: false,
		Failed:  false,
		ID:      []string{},
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded ansible.Response
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(decoded.ID) != 0 {
		t.Errorf("Expected empty ID array, got %d items", len(decoded.ID))
	}
}

// Test 16: RequireField with only spaces does NOT fail (spaces are not empty)
func TestRequireField_SpacesAreNotEmpty(t *testing.T) {
	result := ansible.RequireField("   ", "error message")
	if result != "   " {
		t.Errorf("Expected '   ' (spaces), but got '%s'", result)
	}
}

// Test 17: MigrateResponse with multiple disks
func TestMigrateResponse_MultipleDisks(t *testing.T) {
	response := ansible.MigrateResponse{
		Msg:     "Migration complete",
		Changed: true,
		Disks: []ansible.Disk{
			{ID: "disk-1", Primary: true},
			{ID: "disk-2", Primary: false},
			{ID: "disk-3", Primary: false},
		},
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded ansible.MigrateResponse
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(decoded.Disks) != 3 {
		t.Errorf("Expected 3 disks, got %d", len(decoded.Disks))
	}
	if decoded.Disks[0].Primary != true {
		t.Error("First disk should be primary")
	}
	if decoded.Disks[1].Primary != false {
		t.Error("Second disk should not be primary")
	}
}

// Test 18: Response with special characters in message
func TestResponse_SpecialCharactersInMessage(t *testing.T) {
	response := ansible.Response{
		Msg:     `Error: "file not found" at path /home/user`,
		Changed: false,
		Failed:  true,
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded ansible.Response
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Msg != `Error: "file not found" at path /home/user` {
		t.Errorf("Message not preserved correctly, got: %s", decoded.Msg)
	}
}
