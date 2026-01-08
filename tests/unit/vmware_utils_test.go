package moduleutils

import (
	"net/url"
	"testing"
	"vmware-migration-kit/plugins/module_utils/vmware"
)

// =============================================
// ProcessUrl Tests
// =============================================

// Test 1: ProcessUrl sets credentials when username provided
func TestProcessUrl_WithCredentials(t *testing.T) {
	u, _ := url.Parse("https://vcenter.example.com/sdk")

	vmware.ProcessUrl(u, "admin", "secret123")

	if u.User == nil {
		t.Fatal("Expected User to be set, got nil")
	}
	if u.User.Username() != "admin" {
		t.Errorf("Expected username 'admin', got '%s'", u.User.Username())
	}
	password, _ := u.User.Password()
	if password != "secret123" {
		t.Errorf("Expected password 'secret123', got '%s'", password)
	}
}

// Test 2: ProcessUrl does nothing when username is empty
func TestProcessUrl_EmptyUsername(t *testing.T) {
	u, _ := url.Parse("https://vcenter.example.com/sdk")

	vmware.ProcessUrl(u, "", "secret123")

	if u.User != nil {
		t.Errorf("Expected User to be nil when username empty, got %v", u.User)
	}
}

// Test 3: ProcessUrl handles empty password
func TestProcessUrl_EmptyPassword(t *testing.T) {
	u, _ := url.Parse("https://vcenter.example.com/sdk")

	vmware.ProcessUrl(u, "admin", "")

	if u.User == nil {
		t.Fatal("Expected User to be set")
	}
	password, hasPassword := u.User.Password()
	if !hasPassword || password != "" {
		t.Errorf("Expected empty password to be set")
	}
}

// Test 4: ProcessUrl with special characters in password
func TestProcessUrl_SpecialCharsInPassword(t *testing.T) {
	u, _ := url.Parse("https://vcenter.example.com/sdk")

	vmware.ProcessUrl(u, "admin@domain", "P@ss!w0rd#$%")

	password, _ := u.User.Password()
	if password != "P@ss!w0rd#$%" {
		t.Errorf("Expected 'P@ss!w0rd#$%%', got '%s'", password)
	}
}

// =============================================
// DetectWindowsFromStrings Tests
// =============================================

// Test 5: DetectWindowsFromStrings returns true when Windows in full name
func TestDetectWindowsFromStrings_WindowsInFullName(t *testing.T) {
	result := vmware.DetectWindowsFromStrings("Microsoft Windows 10 (64-bit)", "windows9_64Guest")
	if !result {
		t.Error("Expected true for Windows 10")
	}
}

// Test 6: DetectWindowsFromStrings returns true for Windows Server
func TestDetectWindowsFromStrings_WindowsServer(t *testing.T) {
	result := vmware.DetectWindowsFromStrings("Microsoft Windows Server 2019", "windows2019srv_64Guest")
	if !result {
		t.Error("Expected true for Windows Server")
	}
}

// Test 7: DetectWindowsFromStrings returns true when only 'microsoft' in name
func TestDetectWindowsFromStrings_MicrosoftOnly(t *testing.T) {
	result := vmware.DetectWindowsFromStrings("Microsoft OS", "otherGuest")
	if !result {
		t.Error("Expected true when 'microsoft' in name")
	}
}

// Test 8: DetectWindowsFromStrings returns true when 'windows' in guest ID only
func TestDetectWindowsFromStrings_WindowsInIdOnly(t *testing.T) {
	result := vmware.DetectWindowsFromStrings("Unknown OS", "windows10Guest")
	if !result {
		t.Error("Expected true when 'windows' in guest ID")
	}
}

// Test 9: DetectWindowsFromStrings is case insensitive
func TestDetectWindowsFromStrings_CaseInsensitive(t *testing.T) {
	result := vmware.DetectWindowsFromStrings("MICROSOFT WINDOWS", "WINDOWS10")
	if !result {
		t.Error("Expected true - should be case insensitive")
	}
}

// Test 10: DetectWindowsFromStrings returns false for Linux system
func TestDetectWindowsFromStrings_LinuxSystem(t *testing.T) {
	result := vmware.DetectWindowsFromStrings("Red Hat Enterprise Linux 8", "rhel8_64Guest")
	if result {
		t.Error("Expected false for Linux system")
	}
}

// Test 11: DetectWindowsFromStrings returns false for empty strings
func TestDetectWindowsFromStrings_EmptyStrings(t *testing.T) {
	result := vmware.DetectWindowsFromStrings("", "")
	if result {
		t.Error("Expected false for empty strings")
	}
}

// =============================================
// DetectRhelCentosFromStrings Tests
// =============================================

// Test 12: DetectRhelCentosFromStrings returns true for RHEL 8
func TestDetectRhelCentosFromStrings_Rhel8(t *testing.T) {
	result := vmware.DetectRhelCentosFromStrings("Red Hat Enterprise Linux 8 (64-bit)", "rhel8_64Guest")
	if !result {
		t.Error("Expected true for RHEL 8")
	}
}

// Test 13: DetectRhelCentosFromStrings returns true for RHEL 9
func TestDetectRhelCentosFromStrings_Rhel9(t *testing.T) {
	result := vmware.DetectRhelCentosFromStrings("Red Hat Enterprise Linux 9", "rhel9_64Guest")
	if !result {
		t.Error("Expected true for RHEL 9")
	}
}

// Test 14: DetectRhelCentosFromStrings returns true for CentOS 8
func TestDetectRhelCentosFromStrings_CentOS8(t *testing.T) {
	result := vmware.DetectRhelCentosFromStrings("CentOS 8 (64-bit)", "centos8_64Guest")
	if !result {
		t.Error("Expected true for CentOS 8")
	}
}

// Test 15: DetectRhelCentosFromStrings returns true for CentOS 9
func TestDetectRhelCentosFromStrings_CentOS9(t *testing.T) {
	result := vmware.DetectRhelCentosFromStrings("CentOS Stream 9", "centos9_64Guest")
	if !result {
		t.Error("Expected true for CentOS 9")
	}
}

// Test 16: DetectRhelCentosFromStrings returns false for RHEL 7 (only 8+ supported)
func TestDetectRhelCentosFromStrings_Rhel7_NotSupported(t *testing.T) {
	result := vmware.DetectRhelCentosFromStrings("Red Hat Enterprise Linux 7", "rhel7_64Guest")
	if result {
		t.Error("Expected false for RHEL 7 (only 8+ supported)")
	}
}

// Test 17: DetectRhelCentosFromStrings returns false for CentOS 7 (only 8+ supported)
func TestDetectRhelCentosFromStrings_CentOS7_NotSupported(t *testing.T) {
	result := vmware.DetectRhelCentosFromStrings("CentOS 7", "centos7_64Guest")
	if result {
		t.Error("Expected false for CentOS 7 (only 8+ supported)")
	}
}

// Test 18: DetectRhelCentosFromStrings returns false for RHEL 6
func TestDetectRhelCentosFromStrings_Rhel6_NotSupported(t *testing.T) {
	result := vmware.DetectRhelCentosFromStrings("Red Hat Enterprise Linux 6", "rhel6_64Guest")
	if result {
		t.Error("Expected false for RHEL 6")
	}
}

// Test 19: DetectRhelCentosFromStrings returns false for Ubuntu
func TestDetectRhelCentosFromStrings_Ubuntu(t *testing.T) {
	result := vmware.DetectRhelCentosFromStrings("Ubuntu Linux (64-bit)", "ubuntu64Guest")
	if result {
		t.Error("Expected false for Ubuntu")
	}
}

// Test 20: DetectRhelCentosFromStrings returns false for Windows
func TestDetectRhelCentosFromStrings_Windows(t *testing.T) {
	result := vmware.DetectRhelCentosFromStrings("Microsoft Windows 10", "windows10Guest")
	if result {
		t.Error("Expected false for Windows")
	}
}

// Test 21: DetectRhelCentosFromStrings returns false for empty strings
func TestDetectRhelCentosFromStrings_EmptyStrings(t *testing.T) {
	result := vmware.DetectRhelCentosFromStrings("", "")
	if result {
		t.Error("Expected false for empty strings")
	}
}

// =============================================
// DetectLinuxFromStrings Tests
// =============================================

// Test 22: DetectLinuxFromStrings returns true for generic Linux
func TestDetectLinuxFromStrings_GenericLinux(t *testing.T) {
	result := vmware.DetectLinuxFromStrings("Other Linux (64-bit)", "otherLinux64Guest")
	if !result {
		t.Error("Expected true for generic Linux")
	}
}

// Test 23: DetectLinuxFromStrings returns true for Ubuntu Linux
func TestDetectLinuxFromStrings_Ubuntu(t *testing.T) {
	result := vmware.DetectLinuxFromStrings("Ubuntu Linux (64-bit)", "ubuntu64Guest")
	if !result {
		t.Error("Expected true for Ubuntu Linux")
	}
}

// Test 24: DetectLinuxFromStrings returns false for RHEL (no 'linux' in name/ID)
func TestDetectLinuxFromStrings_Rhel(t *testing.T) {
	result := vmware.DetectLinuxFromStrings("Red Hat Enterprise 8", "rhel8_64Guest")
	if result {
		t.Error("Expected false - RHEL doesn't have 'linux' in the name/ID")
	}
}

// Test 25: DetectLinuxFromStrings returns true when 'linux' in guest ID only
func TestDetectLinuxFromStrings_LinuxInIdOnly(t *testing.T) {
	result := vmware.DetectLinuxFromStrings("Unknown OS", "otherLinuxGuest")
	if !result {
		t.Error("Expected true when 'linux' in guest ID")
	}
}

// Test 26: DetectLinuxFromStrings is case insensitive
func TestDetectLinuxFromStrings_CaseInsensitive(t *testing.T) {
	result := vmware.DetectLinuxFromStrings("LINUX SERVER", "LINUX64")
	if !result {
		t.Error("Expected true - should be case insensitive")
	}
}

// Test 27: DetectLinuxFromStrings returns false for Windows
func TestDetectLinuxFromStrings_Windows(t *testing.T) {
	result := vmware.DetectLinuxFromStrings("Microsoft Windows 10", "windows10Guest")
	if result {
		t.Error("Expected false for Windows")
	}
}

// Test 28: DetectLinuxFromStrings returns false for empty strings
func TestDetectLinuxFromStrings_EmptyStrings(t *testing.T) {
	result := vmware.DetectLinuxFromStrings("", "")
	if result {
		t.Error("Expected false for empty strings")
	}
}
