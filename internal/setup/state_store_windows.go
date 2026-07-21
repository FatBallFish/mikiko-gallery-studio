//go:build windows

package setup

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func secureStateDirectory(path string) error {
	if err := applyRestrictedStateDACL(path); err != nil {
		return fmt.Errorf("protect directory DACL: %w", err)
	}
	return nil
}

func secureStateFile(path string, _ *os.File) error {
	if err := applyRestrictedStateDACL(path); err != nil {
		return fmt.Errorf("protect file DACL: %w", err)
	}
	return nil
}

func replaceStateFile(source, destination string) error {
	sourcePointer, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return fmt.Errorf("encode source path: %w", err)
	}
	destinationPointer, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return fmt.Errorf("encode destination path: %w", err)
	}
	flags := uint32(windows.MOVEFILE_REPLACE_EXISTING | windows.MOVEFILE_WRITE_THROUGH)
	if err := windows.MoveFileEx(sourcePointer, destinationPointer, flags); err != nil {
		return fmt.Errorf("MoveFileEx replace with write-through: %w", err)
	}
	return nil
}

func syncStateDirectory(string) error {
	// MOVEFILE_WRITE_THROUGH makes the replacement durable on Windows.
	return nil
}

func applyRestrictedStateDACL(path string) error {
	currentUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("read current process identity: %w", err)
	}
	administrators, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return fmt.Errorf("create administrators SID: %w", err)
	}
	localSystem, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return fmt.Errorf("create local system SID: %w", err)
	}
	access := []windows.EXPLICIT_ACCESS{
		fullControlForStateSID(currentUser.User.Sid, windows.TRUSTEE_IS_USER),
		fullControlForStateSID(administrators, windows.TRUSTEE_IS_GROUP),
		fullControlForStateSID(localSystem, windows.TRUSTEE_IS_USER),
	}
	acl, err := windows.ACLFromEntries(access, nil)
	if err != nil {
		return fmt.Errorf("build restricted DACL: %w", err)
	}
	if err := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		acl,
		nil,
	); err != nil {
		return fmt.Errorf("apply restricted DACL: %w", err)
	}
	return nil
}

func fullControlForStateSID(sid *windows.SID, trusteeType windows.TRUSTEE_TYPE) windows.EXPLICIT_ACCESS {
	return windows.EXPLICIT_ACCESS{
		AccessPermissions: windows.ACCESS_MASK(windows.GENERIC_ALL),
		AccessMode:        windows.GRANT_ACCESS,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  trusteeType,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
}
