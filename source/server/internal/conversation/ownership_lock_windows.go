package conversation

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func lockOwnershipFile(f *os.File) error {
	// The capability retains this synchronous handle for its entire lifetime.
	// Closing the handle releases the byte-range lock, including process exit.
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
}
func ownershipLockContended(err error) bool { return errors.Is(err, windows.ERROR_LOCK_VIOLATION) }

// Only canonical local drive paths are supported. Reject rather than normalize
// aliases: every admitted spelling must address the same permanent lock path.
// As on Unix, application-owned directories/files must not be renamed/replaced
// by the owner while a store is in use. No security descriptor is ever repaired.
func validateWindowsPathSyntax(absolute string) error {
	if _, err := coexistenceFileURL(absolute, true); err != nil {
		return err
	}
	// Alternate data streams (including :$DATA), trailing-dot/space aliases,
	// and Win32 wildcard/device spellings must not acquire separate lock names.
	for _, part := range strings.Split(strings.ReplaceAll(absolute[3:], "/", `\`), `\`) {
		if strings.ContainsAny(part, `:<>"|?*`) || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return fmt.Errorf("unsupported coexistence Windows path component")
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || strings.HasPrefix(base, "COM") && len(base) == 4 || strings.HasPrefix(base, "LPT") && len(base) == 4 {
			return fmt.Errorf("unsupported coexistence Windows device name")
		}
	}
	return nil
}

func windowsPrivateHandle(path string, directory bool) (windows.Handle, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return 0, err
	}
	if err = validateWindowsPathSyntax(absolute); err != nil {
		return 0, err
	}
	// Validate from the volume root downward: each accepted ancestor protects
	// the next name from untrusted replacement, including FILE_DELETE_CHILD.
	// Trusted owners/admins must not rename objects or change ACLs during use.
	if err := rejectWindowsImpersonation(); err != nil {
		return 0, err
	}
	var ancestors []string
	for part := filepath.Dir(absolute); ; part = filepath.Dir(part) {
		ancestors = append(ancestors, part)
		if filepath.Dir(part) == part {
			break
		}
	}
	for i := len(ancestors) - 1; i >= 0; i-- {
		part := ancestors[i]
		p, e := windows.UTF16PtrFromString(part)
		if e != nil {
			return 0, e
		}
		h, e := windows.CreateFile(p, windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if e != nil {
			return 0, e
		}
		e = validateWindowsHandlePolicy(h, part, true, true)
		windows.CloseHandle(h)
		if e != nil {
			return 0, fmt.Errorf("unsafe coexistence ancestor %q: %w", part, e)
		}
	}
	p, err := windows.UTF16PtrFromString(absolute)
	if err != nil {
		return 0, err
	}
	h, err := windows.CreateFile(p, windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return 0, err
	}
	if err = validateWindowsHandle(h, absolute, directory); err != nil {
		windows.CloseHandle(h)
		return 0, err
	}
	return h, nil
}

func validateWindowsHandle(h windows.Handle, path string, directory bool) error {
	return validateWindowsHandlePolicy(h, path, directory, false)
}
func validateWindowsHandlePolicy(h windows.Handle, path string, directory, ancestor bool) error {
	if err := rejectWindowsImpersonation(); err != nil {
		return err
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory {
		return fmt.Errorf("unsupported coexistence object type")
	}
	if !directory && info.NumberOfLinks != 1 {
		return fmt.Errorf("coexistence hard-linked file paths unsupported")
	}
	buf := make([]uint16, 32768)
	n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
	if err != nil {
		return err
	}
	if n == 0 || n >= uint32(len(buf)) {
		return fmt.Errorf("unsupported coexistence canonical path length")
	}
	canonical := strings.TrimPrefix(windows.UTF16ToString(buf[:n]), `\\?\`)
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if canonical != absolute {
		return fmt.Errorf("coexistence path alias unsupported: %q (canonical %q)", absolute, canonical)
	}
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	return validateWindowsDescriptorPolicy(sd, user.User.Sid, directory, ancestor)
}

// SYSTEM and builtin Administrators are the only additional allowed DACL trustees.
// Ancestor ownership separately permits the exact TrustedInstaller service SID.
// This does not add grants, enable privileges, or treat group membership as
// ownership. Unknown/deny/object/callback ACEs and inheritance-only ACEs are
// deliberately refused rather than attempting general Windows access checking.
func validateWindowsDescriptor(sd *windows.SECURITY_DESCRIPTOR, current *windows.SID, directory bool) error {
	return validateWindowsDescriptorPolicy(sd, current, directory, false)
}
func validateWindowsDescriptorPolicy(sd *windows.SECURITY_DESCRIPTOR, current *windows.SID, directory, ancestor bool) error {
	if !sd.IsValid() {
		return fmt.Errorf("invalid security descriptor")
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	if owner == nil || (!ancestor && !owner.Equals(current)) {
		return fmt.Errorf("coexistence object must be owned by current user")
	}
	control, _, err := sd.Control()
	if err != nil {
		return err
	}
	if control&windows.SE_DACL_PRESENT == 0 {
		return fmt.Errorf("coexistence requires a present DACL")
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if acl == nil {
		return fmt.Errorf("coexistence refuses null DACL")
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return err
	}
	admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return err
	}
	if ancestor && !windowsAncestorOwnerTrusted(owner.String(), current.String()) {
		return fmt.Errorf("untrusted coexistence ancestor owner")
	}
	// GetSecurityInfo/SDDL return an OS-allocated self-relative descriptor. Bound
	// the ACL within that descriptor before constructing any unsafe byte slice.
	base, ap := uintptr(unsafe.Pointer(sd)), uintptr(unsafe.Pointer(acl))
	length := uintptr(sd.Length())
	if ap < base || ap-base > length || length-(ap-base) < 8 {
		return fmt.Errorf("DACL outside security descriptor")
	}
	raw := unsafe.Slice((*byte)(unsafe.Pointer(acl)), int(length-(ap-base)))
	aces, err := parseWindowsACL(raw)
	if err != nil {
		return err
	}
	inheritableUser := false
	for _, ace := range aces {
		flags := ace[1]
		allowedFlags := byte(windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE | windows.INHERITED_ACE)
		if ancestor {
			allowedFlags |= windows.INHERIT_ONLY_ACE | windows.NO_PROPAGATE_INHERIT_ACE
		}
		if ace[0] != windows.ACCESS_ALLOWED_ACE_TYPE || flags & ^allowedFlags != 0 {
			return fmt.Errorf("unsupported coexistence ACE type/flags")
		}
		// parseWindowsACL proved the complete SID is within this ACE before any
		// native SID operation (IsValid/Equals/Len) can read it.
		sid := (*windows.SID)(unsafe.Pointer(&ace[8]))
		if !sid.IsValid() {
			return fmt.Errorf("invalid coexistence ACE SID")
		}
		trusted := sid.Equals(current) || sid.Equals(system) || sid.Equals(admins)
		mask := binary.LittleEndian.Uint32(ace[4:8])
		if !trusted {
			// Ancestor read/list/traverse and sibling creation are benign. Other rights, including
			// generic write/all, delete-child, DELETE, WRITE_DAC/OWNER, fails closed.
			if !ancestor || flags&windows.INHERIT_ONLY_ACE == 0 && !windowsAncestorUnrelatedRightsSafe(mask) {
				return fmt.Errorf("coexistence DACL grants an unrelated trustee unsafe rights")
			}
		}
		if sid.Equals(current) && flags&(windows.OBJECT_INHERIT_ACE|windows.CONTAINER_INHERIT_ACE) == windows.OBJECT_INHERIT_ACE|windows.CONTAINER_INHERIT_ACE && mask&0x001f01ff == 0x001f01ff {
			inheritableUser = true
		}
	}
	runtime.KeepAlive(sd)
	// SQLite and os.OpenFile create sidecars/locks with inherited descriptors.
	// Require a full-control user grant propagated to both files and directories;
	// every propagating grant above is restricted to the same explicit allowlist.
	if directory && !ancestor && !inheritableUser {
		return fmt.Errorf("coexistence directory lacks safe child inheritance")
	}
	return nil
}

// Default owner is part of safe inherited creation: an elevated or impersonated
// token may otherwise create objects owned by a group instead of the user. Refuse
// that environment; never adjust the token or repair the resulting object's ACL.
func validateWindowsCreationOwner() error {
	if err := rejectWindowsImpersonation(); err != nil {
		return err
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	token := windows.GetCurrentThreadEffectiveToken()
	var size uint32
	err = windows.GetTokenInformation(token, windows.TokenOwner, nil, 0, &size)
	if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) || size == 0 {
		return fmt.Errorf("query creation owner: %v", err)
	}
	buf := make([]byte, size)
	if err = windows.GetTokenInformation(token, windows.TokenOwner, &buf[0], size, &size); err != nil {
		return err
	}
	owner := *(**windows.SID)(unsafe.Pointer(&buf[0]))
	if owner == nil || !owner.IsValid() || !owner.Equals(user.User.Sid) {
		return fmt.Errorf("coexistence creation token must default to current-user ownership")
	}
	return nil
}

func validatePrivateDirectory(path string) error {
	if err := validateWindowsCreationOwner(); err != nil {
		return err
	}
	h, err := windowsPrivateHandle(path, true)
	if err != nil {
		return err
	}
	return windows.CloseHandle(h)
}
func validateLockDirectory(path string) error { return validatePrivateDirectory(path) }
func validateLockHandle(f *os.File) error {
	return validateWindowsHandle(windows.Handle(f.Fd()), f.Name(), false)
}
func validateOwnershipPath(path string) error {
	h, err := windowsPrivateHandle(path, false)
	if err != nil {
		return err
	}
	return windows.CloseHandle(h)
}
func validateCoexistencePrivacy(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err = validateWindowsPathSyntax(absolute); err != nil {
		return err
	}
	if err = validatePrivateDirectory(filepath.Dir(absolute)); err != nil {
		return err
	}
	for _, suffix := range []string{"", "-wal", "-shm", "-journal", ".ownership-locks"} {
		candidate := absolute + suffix
		_, err := os.Lstat(candidate)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		h, err := windowsPrivateHandle(candidate, suffix == ".ownership-locks")
		if err != nil {
			return err
		}
		if err = windows.CloseHandle(h); err != nil {
			return err
		}
	}
	return nil
}

// No impersonation semantics are supported. Pin the query to one OS thread;
// application code must not change the security context during store use.
func rejectWindowsImpersonation() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var token windows.Token
	err := windows.OpenThreadToken(windows.CurrentThread(), windows.TOKEN_QUERY, true, &token)
	if errors.Is(err, windows.ERROR_NO_TOKEN) {
		return nil
	}
	if err == nil {
		token.Close()
	}
	return fmt.Errorf("coexistence thread impersonation unsupported (token query: %v)", err)
}

// Pure bounded parser: never pass malformed ACL/ACE/SID pointers to Win32.
// The caller supplies bytes within the security descriptor allocation.
func parseWindowsACL(raw []byte) ([][]byte, error) {
	invalid := func() ([][]byte, error) { return nil, fmt.Errorf("malformed coexistence ACL/ACE/SID") }
	if len(raw) < 8 || (raw[0] != 2 && raw[0] != 4) {
		return invalid()
	}
	size := int(binary.LittleEndian.Uint16(raw[2:4]))
	count := int(binary.LittleEndian.Uint16(raw[4:6]))
	if size < 8 || size > len(raw) {
		return invalid()
	}
	raw = raw[:size]
	var aces [][]byte
	offset := 8
	for i := 0; i < count; i++ {
		if len(raw)-offset < 4 {
			return invalid()
		}
		n := int(binary.LittleEndian.Uint16(raw[offset+2 : offset+4]))
		if n < 16 || n%4 != 0 || n > len(raw)-offset {
			return invalid()
		}
		ace := raw[offset : offset+n]
		// Only simple allow ACEs have the SID layout supported here.
		if ace[0] != windows.ACCESS_ALLOWED_ACE_TYPE {
			return invalid()
		}
		if ace[8] != 1 || ace[9] > 15 || 16+4*int(ace[9]) != n {
			return invalid()
		}
		aces = append(aces, ace)
		offset += n
	}
	return aces, nil
}
