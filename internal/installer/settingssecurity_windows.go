//go:build windows

package installer

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Apply installs a protected DACL granting only the current user full control
// to the file at the given path. The security descriptor is protected, meaning
// it cannot inherit from the parent directory.
func Apply(path string) error {
	// Get the current user's SID
	token := windows.GetCurrentProcessToken()
	defer token.Close()
	tokenUser, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	sid := tokenUser.User.Sid

	// Build an EXPLICIT_ACCESS entry granting full control to the current user
	explicitEntries := []windows.EXPLICIT_ACCESS{
		{
			AccessPermissions: windows.GENERIC_ALL,
			AccessMode:        windows.GRANT_ACCESS,
			Inheritance:       windows.NO_INHERITANCE,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_USER,
				TrusteeValue: windows.TrusteeValueFromSID(sid),
			},
		},
	}

	// Create ACL from entries
	dacl, err := windows.ACLFromEntries(explicitEntries, nil)
	if err != nil {
		return err
	}
	// Don't LocalFree - SetNamedSecurityInfo takes ownership

	// Set the protected DACL directly on the file
	err = windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil,
	)
	if err != nil {
		return os.NewSyscallError("SetNamedSecurityInfo", err)
	}

	return nil
}

// OwnerOnly reports whether the file at the given path has a protected DACL
// granting only the current user full control (i.e., no inherited permissions).
func OwnerOnly(path string) (bool, error) {
	// Get security descriptor
	sd, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return false, os.NewSyscallError("GetNamedSecurityInfo", err)
	}
	if sd == nil {
		return false, fmt.Errorf("no security descriptor")
	}

	control, _, err := sd.Control()
	if err != nil {
		return false, err
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		return false, nil
	}

	// Get current user SID
	token := windows.GetCurrentProcessToken()
	defer token.Close()
	tokenUser, err := token.GetTokenUser()
	if err != nil {
		return false, err
	}
	currentSid := tokenUser.User.Sid

	// Check DACL
	dacl, defaulted, err := sd.DACL()
	if err != nil {
		return false, err
	}
	if dacl == nil || defaulted {
		return false, nil
	}

	// Count ACEs
	aceCount := dacl.AceCount
	if aceCount == 0 {
		return false, nil
	}

	hasFullControl := false
	for i := uint32(0); i < uint32(aceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		err = windows.GetAce(dacl, i, &ace)
		if err != nil {
			return false, fmt.Errorf("GetAce failed at index %d: %w", i, err)
		}

		aceSid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || !windows.EqualSid(aceSid, currentSid) {
			return false, nil
		}

		// Check if the ACE grants full control (GENERIC_ALL or FILE_ALL_ACCESS)
		// GENERIC_ALL gets mapped to specific rights for files, so we check for either
		isFullControl := ace.Mask == windows.GENERIC_ALL || ace.Mask == 0x001F01FF
		if isFullControl {
			hasFullControl = true
		}
	}

	return hasFullControl, nil
}
