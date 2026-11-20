//go:build windows

package privatefs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestEnsurePrivateDirProtectsAndInheritsCurrentUserACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	setTestDACL(t, path, "D:P(A;OICI;FA;;;WD)")
	if err := EnsurePrivateDir(path); err != nil {
		t.Fatal(err)
	}
	inheritable := uint8(windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE)
	assertCurrentUserDACL(t, path, true, inheritable, windows.INHERITED_ACE)

	child := filepath.Join(path, "child")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	assertCurrentUserDACL(t, child, false, inheritable|windows.INHERITED_ACE, 0)

	childFile, err := os.OpenFile(filepath.Join(path, "secret"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := childFile.Close(); err != nil {
		t.Fatal(err)
	}
	assertCurrentUserDACL(t, filepath.Join(path, "secret"), false, windows.INHERITED_ACE, 0)
}

func TestRestrictFileProtectsOnlyCurrentUser(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	setTestDACL(t, path, "D:P(A;;FA;;;WD)")
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := RestrictFile(file); err != nil {
		t.Fatal(err)
	}
	assertCurrentUserDACL(t, path, true, 0,
		windows.OBJECT_INHERIT_ACE|windows.CONTAINER_INHERIT_ACE|windows.INHERITED_ACE)
}

func TestEnsurePrivateDirSecuresExistingTree(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private")
	childDir := filepath.Join(path, "child")
	if err := os.MkdirAll(childDir, 0700); err != nil {
		t.Fatal(err)
	}
	files := []string{"job.json", "database", "database-wal", "database-shm"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(path, name), []byte("private state"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	nestedFile := filepath.Join(childDir, "nested.json")
	if err := os.WriteFile(nestedFile, []byte("private state"), 0600); err != nil {
		t.Fatal(err)
	}
	setTestDACL(t, path, "D:P(A;OICI;FA;;;WD)")
	setTestDACL(t, childDir, "D:P(A;OICI;FA;;;WD)")
	for _, name := range files {
		setTestDACL(t, filepath.Join(path, name), "D:P(A;;FA;;;WD)")
	}
	setTestDACL(t, nestedFile, "D:P(A;;FA;;;WD)")

	if err := EnsurePrivateDir(path); err != nil {
		t.Fatal(err)
	}
	inheritable := uint8(windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE)
	assertCurrentUserDACL(t, path, true, inheritable, windows.INHERITED_ACE)
	assertCurrentUserDACL(t, childDir, true, inheritable, windows.INHERITED_ACE)
	for _, name := range files {
		assertCurrentUserDACL(t, filepath.Join(path, name), true, 0, inheritable|windows.INHERITED_ACE)
	}
	assertCurrentUserDACL(t, nestedFile, true, 0, inheritable|windows.INHERITED_ACE)
	if got, err := os.ReadFile(nestedFile); err != nil || string(got) != "private state" {
		t.Fatalf("existing state file = %q, %v; want unchanged contents", got, err)
	}
}

func TestEnsurePrivateDirSecuresOpenFile(t *testing.T) {
	path := t.TempDir()
	filePath := filepath.Join(path, "owner.lock")
	if err := os.WriteFile(filePath, nil, 0600); err != nil {
		t.Fatal(err)
	}
	setTestDACL(t, filePath, "D:P(A;;FA;;;WD)")
	file, err := os.OpenFile(filePath, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := EnsurePrivateDir(path); err != nil {
		t.Fatal(err)
	}
	assertCurrentUserDACL(t, filePath, true, 0,
		windows.OBJECT_INHERIT_ACE|windows.CONTAINER_INHERIT_ACE|windows.INHERITED_ACE)
}

func TestEnsurePrivateDirSupportsLongRootPath(t *testing.T) {
	path := t.TempDir()
	for len(path) <= 300 {
		path = filepath.Join(path, strings.Repeat("d", 100))
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := EnsurePrivateDir(path); err != nil {
		t.Fatal(err)
	}
}

func TestEnsurePrivateDirRejectsJunctionRootWithoutTouchingTarget(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "private")
	target := filepath.Join(base, "outside")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	targetFile := filepath.Join(target, "secret")
	if err := os.WriteFile(targetFile, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	setTestDACL(t, target, "D:P(A;OICI;FA;;;WD)")
	setTestDACL(t, targetFile, "D:P(A;;FA;;;WD)")
	createTestJunction(t, path, target)

	err := EnsurePrivateDir(path)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "reparse") {
		t.Fatalf("EnsurePrivateDir error = %v, want a reparse-point error", err)
	}
	assertEveryoneDACL(t, target, windows.OBJECT_INHERIT_ACE|windows.CONTAINER_INHERIT_ACE)
	assertEveryoneDACL(t, targetFile, 0)
}

func TestEnsurePrivateDirRejectsJunctionWithoutTouchingTarget(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "private")
	target := filepath.Join(base, "outside")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	targetFile := filepath.Join(target, "secret")
	if err := os.WriteFile(targetFile, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	setTestDACL(t, target, "D:P(A;OICI;FA;;;WD)")
	setTestDACL(t, targetFile, "D:P(A;;FA;;;WD)")
	junction := filepath.Join(path, "outside")
	createTestJunction(t, junction, target)

	err := EnsurePrivateDir(path)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "reparse") {
		t.Fatalf("EnsurePrivateDir error = %v, want a reparse-point error", err)
	}
	assertEveryoneDACL(t, target, windows.OBJECT_INHERIT_ACE|windows.CONTAINER_INHERIT_ACE)
	assertEveryoneDACL(t, targetFile, 0)
}

func TestEnsurePrivateDirRejectsExternalHardlinkWithoutChangingACL(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "private")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	stateFile := filepath.Join(path, "job.json")
	if err := os.WriteFile(stateFile, []byte("private state"), 0600); err != nil {
		t.Fatal(err)
	}
	externalAlias := filepath.Join(base, "external-secret")
	if err := os.Link(stateFile, externalAlias); err != nil {
		t.Fatalf("create external hard link: %v", err)
	}
	setTestDACL(t, stateFile, "D:P(A;;FA;;;WD)")
	sd, err := windows.GetNamedSecurityInfo(stateFile, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(stateFile, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.UNPROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
	before, err := windows.GetNamedSecurityInfo(externalAlias, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(stateFile, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	err = EnsurePrivateDir(path)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "hard link") {
		t.Fatalf("EnsurePrivateDir error = %v, want a hard-link error", err)
	}
	after, err := windows.GetNamedSecurityInfo(externalAlias, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil || after.String() != before.String() {
		t.Fatalf("external hard-link ACL changed: before=%s, after=%v, error=%v", before, after, err)
	}
}

func createTestJunction(t *testing.T, path, target string) {
	t.Helper()
	output, err := exec.Command("cmd.exe", "/c", "mklink", "/J", path, target).CombinedOutput()
	if err != nil {
		t.Fatalf("create directory junction: %v: %s", err, output)
	}
}

func setTestDACL(t *testing.T, path, sddl string) {
	t.Helper()
	sd, err := windows.SecurityDescriptorFromString(sddl)
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

func assertEveryoneDACL(t *testing.T, path string, flags uint8) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := sd.Control()
	if err != nil {
		t.Fatal(err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("DACL is not protected")
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if dacl == nil || dacl.AceCount != 1 {
		t.Fatalf("DACL ACE count = %v, want one", dacl)
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil {
		t.Fatal(err)
	}
	if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
		t.Fatalf("ACE type = %d, want allow", ace.Header.AceType)
	}
	const fileAllAccess = windows.ACCESS_MASK(0x001F01FF)
	if ace.Mask != fileAllAccess {
		t.Fatalf("ACE access mask = %#x, want %#x", ace.Mask, fileAllAccess)
	}
	everyone, err := windows.StringToSid("S-1-1-0")
	if err != nil {
		t.Fatal(err)
	}
	if sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart)); !sid.Equals(everyone) {
		t.Fatalf("ACE SID %s is not Everyone %s", sid, everyone)
	}
	if ace.Header.AceFlags != flags {
		t.Fatalf("ACE flags %#x, want %#x", ace.Header.AceFlags, flags)
	}
}

func assertCurrentUserDACL(t *testing.T, path string, protected bool, requiredFlags, forbiddenFlags uint8) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := sd.Control()
	if err != nil {
		t.Fatal(err)
	}
	if got := control&windows.SE_DACL_PROTECTED != 0; got != protected {
		t.Fatalf("DACL protected = %t, want %t", got, protected)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if dacl == nil || dacl.AceCount != 1 {
		t.Fatalf("DACL ACE count = %v, want one", dacl)
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil {
		t.Fatal(err)
	}
	if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
		t.Fatalf("ACE type = %d, want allow", ace.Header.AceType)
	}
	const fileAllAccess = windows.ACCESS_MASK(0x001F01FF)
	if ace.Mask != fileAllAccess {
		t.Fatalf("ACE access mask = %#x, want %#x", ace.Mask, fileAllAccess)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	if sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart)); !sid.Equals(user.User.Sid) {
		t.Fatalf("ACE SID %s is not the current user %s", sid, user.User.Sid)
	}
	if got := ace.Header.AceFlags; got&requiredFlags != requiredFlags || got&forbiddenFlags != 0 {
		t.Fatalf("ACE flags %#x, required %#x and forbidden %#x", got, requiredFlags, forbiddenFlags)
	}
}
