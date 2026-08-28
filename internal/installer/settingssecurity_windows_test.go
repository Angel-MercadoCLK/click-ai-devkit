//go:build windows

package installer

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestOwnerOnly_UnsecuredFileReportsFalse(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())

	testFile := filepath.Join(t.TempDir(), "test-settings.json")
	if err := os.WriteFile(testFile, []byte(`{"test": "data"}`), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	only, err := OwnerOnly(testFile)
	if err != nil {
		t.Fatalf("OwnerOnly() returned an error for an unsecured file: %v", err)
	}
	if only {
		t.Error("OwnerOnly() reported true for an unsecured file, want false")
	}
}

func TestOwnerOnly_AppliedFileReportsTrue(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())

	testFile := filepath.Join(t.TempDir(), "test-settings.json")
	if err := os.WriteFile(testFile, []byte(`{"test": "data"}`), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := Apply(testFile); err != nil {
		t.Fatalf("Apply() failed: %v", err)
	}

	only, err := OwnerOnly(testFile)
	if err != nil {
		t.Fatalf("OwnerOnly() failed: %v", err)
	}
	if !only {
		t.Error("OwnerOnly() reported false for an applied file, want true")
	}
}

func TestOwnerOnly_ForeignACEInDACLReportsFalse(t *testing.T) {
	testFile := filepath.Join(t.TempDir(), "test-settings.json")
	if err := os.WriteFile(testFile, []byte(`{"test": "data"}`), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	token := windows.GetCurrentProcessToken()
	defer token.Close()
	tokenUser, err := token.GetTokenUser()
	if err != nil {
		t.Fatalf("GetTokenUser() error = %v", err)
	}
	foreignSID, err := windows.StringToSid("S-1-5-18")
	if err != nil {
		t.Fatalf("StringToSid() error = %v", err)
	}
	dacl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{
		{AccessPermissions: windows.GENERIC_ALL, AccessMode: windows.GRANT_ACCESS, Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeValue: windows.TrusteeValueFromSID(tokenUser.User.Sid)}},
		{AccessPermissions: windows.GENERIC_ALL, AccessMode: windows.GRANT_ACCESS, Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeValue: windows.TrusteeValueFromSID(foreignSID)}},
	}, nil)
	if err != nil {
		t.Fatalf("ACLFromEntries() error = %v", err)
	}
	if err := windows.SetNamedSecurityInfo(testFile, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		t.Fatalf("SetNamedSecurityInfo() error = %v", err)
	}

	only, err := OwnerOnly(testFile)
	if err != nil {
		t.Fatalf("OwnerOnly() error = %v", err)
	}
	if only {
		t.Fatal("OwnerOnly() = true for a DACL containing a foreign ACE, want false")
	}
}

func TestApplyOwnerOnlySecurity_ProtectedDACL(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test-settings.json")

	if err := os.WriteFile(testFile, []byte(`{"test": "data"}`), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	if err := Apply(testFile); err != nil {
		t.Fatalf("Apply() failed: %v", err)
	}

	only, err := OwnerOnly(testFile)
	if err != nil {
		t.Fatalf("OwnerOnly() failed: %v", err)
	}
	if !only {
		t.Errorf("OwnerOnly() reported false, want true")
	}
}
