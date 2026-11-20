//go:build windows

package privatefs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// EnsurePrivateDir creates path and restricts its existing state tree to the current Windows user.
func EnsurePrivateDir(path string) error {
	if path == "" {
		return os.ErrInvalid
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	fileDACL, err := currentUserDACL(false)
	if err != nil {
		return err
	}
	directoryDACL, err := currentUserDACL(true)
	if err != nil {
		return err
	}
	acls := privateDACLs{file: fileDACL, directory: directoryDACL}
	handle, err := openPrivateRoot(path)
	if err != nil {
		if !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) && !errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
			return fmt.Errorf("open private state directory %q: %w", path, err)
		}
		if err := os.MkdirAll(path, 0700); err != nil {
			return err
		}
		handle, err = openPrivateRoot(path)
		if err != nil {
			return fmt.Errorf("open private state directory %q: %w", path, err)
		}
	}
	root := os.NewFile(uintptr(handle), path)
	if root == nil {
		_ = windows.CloseHandle(handle)
		return fmt.Errorf("open private state directory %q: invalid handle", path)
	}
	defer root.Close()
	return securePrivateTree(root, path, acls)
}

// RestrictFile limits an opened file to the current Windows user.
func RestrictFile(file *os.File) error {
	if file == nil {
		return os.ErrInvalid
	}
	dacl, err := currentUserDACL(false)
	if err != nil {
		return err
	}
	// Go's file handles lack WRITE_DAC. ReOpenFile adds it without resolving a path.
	handle, _, callErr := reOpenFile.Call(uintptr(file.Fd()), windows.WRITE_DAC,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_FLAG_OPEN_REPARSE_POINT)
	runtime.KeepAlive(file)
	if windows.Handle(handle) == windows.InvalidHandle {
		return fmt.Errorf("reopen private file for ACL update: %w", callErr)
	}
	defer windows.CloseHandle(windows.Handle(handle))
	if err := setPrivateDACL(windows.Handle(handle), dacl); err != nil {
		return fmt.Errorf("restrict private file: %w", err)
	}
	return nil
}

var reOpenFile = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReOpenFile")

// MAXIMUM_ALLOWED prevents SetSecurityInfo from propagating ACLs to unchecked children.
// https://learn.microsoft.com/windows/win32/api/aclapi/nf-aclapi-setsecurityinfo
const privateHandleAccess = windows.MAXIMUM_ALLOWED

func currentUserDACL(inherit bool) (*windows.ACL, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("get current Windows user: %w", err)
	}
	inheritance := ""
	if inherit {
		inheritance = "OICI"
	}
	sd, err := windows.SecurityDescriptorFromString(fmt.Sprintf("D:P(A;%s;FA;;;%s)", inheritance, user.User.Sid.String()))
	if err != nil {
		return nil, fmt.Errorf("create private ACL: %w", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return nil, fmt.Errorf("read private ACL: %w", err)
	}
	return dacl, nil
}

func openPrivateRoot(path string) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(extendedLengthPath(path))
	if err != nil {
		return windows.InvalidHandle, err
	}
	return windows.CreateFile(name, privateHandleAccess,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
}

type privateDACLs struct {
	file, directory *windows.ACL
}

func securePrivateTree(dir *os.File, path string, acls privateDACLs) error {
	handle := windows.Handle(dir.Fd())
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return fmt.Errorf("inspect private state path %q: %w", path, err)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("private state contains a reparse point at %q", path)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return fmt.Errorf("private state path is not a directory: %q", path)
	}
	if err := setPrivateDACL(handle, acls.file); err != nil {
		return fmt.Errorf("restrict private state directory %q: %w", path, err)
	}
	if err := securePrivateChildren(dir, path, acls); err != nil {
		return err
	}
	return setPrivateDACL(handle, acls.directory)
}

func securePrivateChildren(dir *os.File, path string, acls privateDACLs) error {
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return fmt.Errorf("read private state directory %q: %w", path, err)
	}
	for _, entry := range entries {
		childPath := filepath.Join(path, entry.Name())
		if err := securePrivateEntry(windows.Handle(dir.Fd()), entry.Name(), childPath, acls); err != nil {
			return err
		}
	}
	return nil
}

func extendedLengthPath(path string) string {
	if strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\\.\`) {
		return path
	}
	if strings.HasPrefix(path, `\\`) {
		return `\\?\UNC\` + strings.TrimPrefix(path, `\\`)
	}
	return `\\?\` + path
}

func securePrivateEntry(parent windows.Handle, name, path string, acls privateDACLs) error {
	handle, err := openPrivateEntry(parent, name)
	if err != nil {
		return fmt.Errorf("open private state entry %q: %w", path, err)
	}
	defer windows.CloseHandle(handle)
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return fmt.Errorf("inspect private state entry %q: %w", path, err)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("private state contains a reparse point at %q", path)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		if info.NumberOfLinks > 1 {
			return fmt.Errorf("private state file has %d hard links; refusing to change ACL at %q", info.NumberOfLinks, path)
		}
		if err := setPrivateDACL(handle, acls.file); err != nil {
			return fmt.Errorf("restrict private state file %q: %w", path, err)
		}
		return nil
	}
	if err := setPrivateDACL(handle, acls.file); err != nil {
		return fmt.Errorf("restrict private state directory %q: %w", path, err)
	}
	var listHandle windows.Handle
	process := windows.CurrentProcess()
	if err := windows.DuplicateHandle(process, handle, process, &listHandle, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return fmt.Errorf("duplicate private state directory handle %q: %w", path, err)
	}
	child := os.NewFile(uintptr(listHandle), path)
	if child == nil {
		_ = windows.CloseHandle(listHandle)
		return fmt.Errorf("open private state directory %q: invalid handle", path)
	}
	defer child.Close()
	if err := securePrivateChildren(child, path, acls); err != nil {
		return err
	}
	return setPrivateDACL(handle, acls.directory)
}

func openPrivateEntry(parent windows.Handle, name string) (windows.Handle, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return windows.InvalidHandle, err
	}
	attributes := windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: parent,
		ObjectName:    objectName,
		Attributes:    windows.OBJ_CASE_INSENSITIVE,
	}
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&handle, privateHandleAccess|windows.SYNCHRONIZE, &attributes, &status, nil, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, windows.FILE_OPEN,
		windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	if err != nil {
		return windows.InvalidHandle, err
	}
	return handle, nil
}

func setPrivateDACL(handle windows.Handle, dacl *windows.ACL) error {
	return windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil)
}

// SyncDir is a no-op on Windows. Callers must flush file data before rename.
func SyncDir(string) error { return nil }
