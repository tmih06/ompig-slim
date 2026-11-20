//go:build windows

package broker

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func assertPrivatePath(t testing.TB, path string) {
	t.Helper()
	assertPrivateACL(t, path, true)
}

func assertPrivateFile(t testing.TB, path string) {
	t.Helper()
	assertPrivateACL(t, path, false)
}

func assertPrivateACL(t testing.TB, path string, directory bool) {
	t.Helper()
	sd, dacl, ace := testDACL(t, path)
	control, _, err := sd.Control()
	if err != nil {
		t.Fatal(err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 || dacl.AceCount != 1 || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
		t.Fatalf("data directory DACL is not protected to one allowed user: %s", sd)
	}
	const fileAllAccess = windows.ACCESS_MASK(0x001F01FF)
	if ace.Mask != fileAllAccess {
		t.Fatalf("data directory ACE mask = %#x, want %#x", ace.Mask, fileAllAccess)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	if sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart)); !sid.Equals(user.User.Sid) {
		t.Fatalf("data directory ACE SID %v is not current user %v", sid, user.User.Sid)
	}
	mask := uint8(windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE | windows.INHERITED_ACE)
	want := uint8(0)
	if directory {
		want = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
	}
	if ace.Header.AceFlags&mask != want {
		t.Fatalf("private path ACE flags = %#x, want %#x", ace.Header.AceFlags, want)
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
