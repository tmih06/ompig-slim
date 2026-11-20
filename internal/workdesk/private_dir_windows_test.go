//go:build windows

package workdesk_test

import (
	"os"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func assertPrivatePath(t testing.TB, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	sd, dacl, ace := testDACL(t, path)
	if dacl.AceCount != 1 || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
		t.Fatalf("path DACL is not one allowed user entry: %s", sd)
	}
	const fileAllAccess = windows.ACCESS_MASK(0x001F01FF)
	if ace.Mask != fileAllAccess {
		t.Fatalf("path ACE mask = %#x, want %#x", ace.Mask, fileAllAccess)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	if sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart)); !sid.Equals(user.User.Sid) {
		t.Fatalf("path ACE SID %v is not current user %v", sid, user.User.Sid)
	}
	control, _, err := sd.Control()
	if err != nil {
		t.Fatal(err)
	}
	if info.IsDir() {
		inherit := uint8(windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE)
		if control&windows.SE_DACL_PROTECTED == 0 || ace.Header.AceFlags&inherit != inherit || ace.Header.AceFlags&windows.INHERITED_ACE != 0 {
			t.Fatalf("directory DACL does not protect an explicit inheritable user ACE: %s", sd)
		}
	} else if control&windows.SE_DACL_PROTECTED == 0 && ace.Header.AceFlags&windows.INHERITED_ACE == 0 {
		t.Fatalf("file DACL does not have a protected or inherited private user ACE: %s", sd)
	}
}

func testDACL(t testing.TB, path string) (*windows.SECURITY_DESCRIPTOR, *windows.ACL, *windows.ACCESS_ALLOWED_ACE) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if dacl == nil || dacl.AceCount == 0 {
		t.Fatalf("path has no DACL entries: %s", sd)
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil {
		t.Fatal(err)
	}
	return sd, dacl, ace
}

func makeWidePath(t testing.TB, path string) {
	t.Helper()
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
}

func assertWidePath(t testing.TB, path string) {
	t.Helper()
	sd, dacl, ace := testDACL(t, path)
	world, err := windows.StringToSid("S-1-1-0")
	if err != nil {
		t.Fatal(err)
	}
	const fileAllAccess = windows.ACCESS_MASK(0x001F01FF)
	inherit := uint8(windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE)
	if dacl.AceCount != 1 || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Mask != fileAllAccess || !(*windows.SID)(unsafe.Pointer(&ace.SidStart)).Equals(world) || ace.Header.AceFlags&inherit != inherit {
		t.Fatalf("status changed directory permissions: %s", sd)
	}
}
