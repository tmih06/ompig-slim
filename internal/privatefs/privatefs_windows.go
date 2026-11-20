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
	defer root.Close()
	info, err := inspectPrivateEntry(handle, path)
	if err != nil {
		return err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return fmt.Errorf("private state path is not a directory: %q", path)
	}
	return securePrivateDir(root, path, acls)
}

// RestrictFile limits an opened file to the current Windows user.
func RestrictFile(file *os.File) error {
	dacl, err := currentUserDACL(false)
	if err != nil {
		return err
	}
	// Go's file handles lack WRITE_DAC. ReOpenFile adds it without resolving a path.
	handle, _, callErr := reOpenFile.Call(uintptr(file.Fd()), privateFileACLAccess,
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

const privateFileACLAccess = windows.READ_CONTROL | windows.WRITE_DAC | windows.FILE_READ_ATTRIBUTES

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

func extendedLengthPath(path string) string {
	if strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\\.\`) {
		return path
	}
	if strings.HasPrefix(path, `\\`) {
		return `\\?\UNC\` + strings.TrimPrefix(path, `\\`)
	}
	return `\\?\` + path
}

type privateDACLs struct {
	file, directory *windows.ACL
}

func securePrivateDir(dir *os.File, path string, acls privateDACLs) error {
	handle := windows.Handle(dir.Fd())
	if err := setPrivateDACL(handle, acls.file); err != nil {
		return fmt.Errorf("restrict private state directory %q: %w", path, err)
	}
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return fmt.Errorf("read private state directory %q: %w", path, err)
	}
	for _, entry := range entries {
		if err := securePrivateEntry(handle, entry.Name(), filepath.Join(path, entry.Name()), acls); err != nil {
			return err
		}
	}
	return setPrivateDACL(handle, acls.directory)
}

func securePrivateEntry(parent windows.Handle, name, path string, acls privateDACLs) error {
	handle, err := openPrivateEntry(parent, name)
	if err != nil {
		return fmt.Errorf("open private state entry %q: %w", path, err)
	}
	entry := os.NewFile(uintptr(handle), path)
	defer entry.Close()
	info, err := inspectPrivateEntry(handle, path)
	if err != nil {
		return err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return securePrivateDir(entry, path, acls)
	}
	if info.NumberOfLinks > 1 {
		return fmt.Errorf("private state file has %d hard links; refusing to change ACL at %q", info.NumberOfLinks, path)
	}
	if err := setPrivateDACL(handle, acls.file); err != nil {
		return fmt.Errorf("restrict private state file %q: %w", path, err)
	}
	return nil
}

func inspectPrivateEntry(handle windows.Handle, path string) (windows.ByHandleFileInformation, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return info, fmt.Errorf("inspect private state entry %q: %w", path, err)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return info, fmt.Errorf("private state contains a reparse point at %q", path)
	}
	return info, nil
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
	const share = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE
	const options = windows.FILE_OPEN_REPARSE_POINT | windows.FILE_SYNCHRONOUS_IO_NONALERT
	err = windows.NtCreateFile(&handle, privateHandleAccess|windows.SYNCHRONIZE, &attributes, &status, nil, 0,
		share, windows.FILE_OPEN, options, 0, 0)
	if errors.Is(err, windows.STATUS_SHARING_VIOLATION) {
		// Go file handles do not share DELETE. Keep directories on MAXIMUM_ALLOWED
		// to prevent ACL propagation, and request data access to enforce the share mode.
		err = windows.NtCreateFile(&handle, privateFileACLAccess|windows.FILE_READ_DATA|windows.SYNCHRONIZE,
			&attributes, &status, nil, 0, share, windows.FILE_OPEN, options|windows.FILE_NON_DIRECTORY_FILE, 0, 0)
	}
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
