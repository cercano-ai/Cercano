package conversation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func windowsTestUser(t *testing.T) *windows.SID {
	t.Helper()
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	return u.User.Sid
}
func windowsSecureTestDirectory(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Private")
	sid := windowsTestUser(t)
	sd, err := windows.SecurityDescriptorFromString("O:" + sid.String() + "D:P(A;OICI;FA;;;" + sid.String() + ")")
	if err != nil {
		t.Fatal(err)
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	// Explicit descriptor only at creation of a new application-owned test object.
	if err = windows.CreateDirectory(p, &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}); err != nil {
		t.Fatal(err)
	}
	if err = validatePrivateDirectory(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestWindowsOwnershipContentionClassification(t *testing.T) {
	for _, e := range []error{nil, windows.ERROR_ACCESS_DENIED, windows.ERROR_SHARING_VIOLATION, windows.ERROR_INVALID_HANDLE, windows.ERROR_IO_PENDING} {
		if ownershipLockContended(e) {
			t.Fatalf("misclassified %v", e)
		}
	}
	if !ownershipLockContended(fmt.Errorf("wrapped: %w", windows.ERROR_LOCK_VIOLATION)) {
		t.Fatal("lost wrapped lock violation")
	}
}

func TestWindowsOwnershipDescriptorPolicy(t *testing.T) {
	user := windowsTestUser(t)
	prefix := "O:" + user.String()
	grant := "(A;OICI;FA;;;" + user.String() + ")"
	for _, tc := range []struct {
		name, sddl string
		ok         bool
	}{
		{"user", prefix + "D:P" + grant, true},
		{"system-admin", prefix + "D:P" + grant + "(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)", true},
		{"world", prefix + "D:P" + grant + "(A;OICI;FR;;;WD)", false},
		{"users", prefix + "D:P" + grant + "(A;;FR;;;BU)", false},
		{"unrelated", prefix + "D:P" + grant + "(A;;FR;;;S-1-5-21-1-2-3-1001)", false},
		{"wrong-owner", "O:SYD:P" + grant, false},
		{"null", prefix + "D:NO_ACCESS_CONTROL", false},
		{"absent", prefix, false},
		{"empty", prefix + "D:P", false},
		{"deny", prefix + "D:P(D;;FW;;;WD)" + grant, false},
		{"inherit-only", prefix + "D:P" + grant + "(A;OICIIO;FA;;;SY)", false},
		{"no-propagate", prefix + "D:P(A;OICINP;FA;;;" + user.String() + ")", false},
		{"no-inheritance", prefix + "D:P(A;;FA;;;" + user.String() + ")", false},
		{"creator-owner", prefix + "D:P" + grant + "(A;OICIIO;FA;;;CO)", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sd, err := windows.SecurityDescriptorFromString(tc.sddl)
			if err != nil {
				t.Fatal(err)
			}
			before := sd.String()
			err = validateWindowsDescriptor(sd, user, true)
			if (err == nil) != tc.ok {
				t.Fatalf("accept=%v: %v", tc.ok, err)
			}
			if after := sd.String(); after != before {
				t.Fatalf("descriptor mutated: %q -> %q", before, after)
			}
		})
	}
}

func TestWindowsOwnershipPrivacyAndAliases(t *testing.T) {
	dir := windowsSecureTestDirectory(t)
	path := filepath.Join(dir, "LongDatabaseName.db")
	if err := PrepareCoexistence(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	if err := validateCoexistencePrivacy(path); err != nil {
		t.Fatal(err)
	}
	// The database, sidecars and new lock directory/file must inherit safely.
	f, err := openOwnershipLock(path, "id", true)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	h, err := windowsPrivateHandle(path, false)
	if err != nil {
		t.Fatal(err)
	}
	before, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	windows.CloseHandle(h)
	alias := filepath.Join(dir, "alias.db")
	if err := os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	if err = validateOwnershipPath(path); err == nil {
		t.Fatal("accepted hardlink")
	}
	if err = validateOwnershipPath(alias); err == nil {
		t.Fatal("accepted hardlink alias")
	}
	if err = os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err = validateOwnershipPath(strings.Replace(path, "LongDatabaseName", "longdatabasename", 1)); err == nil {
		t.Fatal("accepted case alias")
	}
	h, err = windowsPrivateHandle(path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	after, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	if before.String() != after.String() {
		t.Fatal("file security descriptor mutated")
	}
	// An unrelated inheritable grant must be refused before creating any database.
	bad := filepath.Join(dir, "Unsafe")
	windowsUnsafeTestDirectory(t, bad)
	rejected := filepath.Join(bad, "must-not-exist.db")
	if err = PrepareCoexistence(context.Background(), rejected); err == nil {
		t.Fatal("accepted unsafe parent")
	}
	if _, err = os.Stat(rejected); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("created rejected database: %v", err)
	}
}

func TestWindowsOwnershipUnsupportedPaths(t *testing.T) {
	for _, p := range []string{`C:\private\db:stream`, `C:\private\db::$DATA`, `C:\private\db.`, `C:\private\db `, `C:\private\NUL.db`, `\\server\share\db`, `\\?\C:\private\db`} {
		if err := validateWindowsPathSyntax(p); err == nil {
			t.Fatalf("accepted %q", p)
		}
	}
	dir := windowsSecureTestDirectory(t)
	path := filepath.Join(dir, "LongDatabaseName.db")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	p, _ := windows.UTF16PtrFromString(path)
	buf := make([]uint16, 32768)
	n, err := windows.GetShortPathName(p, &buf[0], uint32(len(buf)))
	if err != nil {
		t.Fatal(err)
	}
	short := windows.UTF16ToString(buf[:n])
	if short != path {
		if err := validateOwnershipPath(short); err == nil {
			t.Fatal("accepted short-name alias")
		}
	} else {
		t.Log("filesystem did not allocate an 8.3 alias")
	}
	link := filepath.Join(dir, "reparse.db")
	if err := os.Symlink(path, link); err != nil {
		t.Logf("symlink creation unavailable without changing privileges: %v", err)
	} else {
		if err := validateOwnershipPath(link); err == nil {
			t.Fatal("accepted reparse alias")
		}
	}
}

func TestWindowsOwnershipDurableTransitions(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(windowsSecureTestDirectory(t), "durable.db")
	mustCoexist(t, PrepareCoexistence(ctx, path))
	s, err := OpenCoexistence(ctx, path, CurrentCoexistenceDeclaration())
	mustCoexist(t, err)
	defer s.Close()
	a, err := AcquireOwnership(ctx, s, "conversation", "A")
	mustCoexist(t, err)
	defer a.Close()
	mustCoexist(t, s.EnsureConversation(WithOwnership(ctx, a), "conversation", "", ""))
	_, err = AcquireOwnership(ctx, s, "conversation", "B")
	var conflict *OwnershipConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("contention: %v", err)
	}
	handover, err := a.Handover(ctx, "B")
	mustCoexist(t, err)
	b, err := ClaimHandover(ctx, s, "conversation", "B", handover)
	mustCoexist(t, err)
	epoch := b.Epoch()
	mustCoexist(t, b.Close())
	_, err = AcquireOwnership(ctx, s, "conversation", "C")
	if !errors.As(err, &conflict) {
		t.Fatalf("Close must not release durable ownership: %v", err)
	}
	restored, err := RestoreClaimedHandover(ctx, s, "conversation", "B", handover)
	mustCoexist(t, err)
	defer restored.Close()
	mustCoexist(t, restored.Release(ctx))
	c, err := AcquireOwnership(ctx, s, "conversation", "C")
	mustCoexist(t, err)
	defer c.Close()
	if c.Epoch() == epoch {
		t.Fatal("epoch reused")
	}
	mustCoexist(t, c.Release(ctx))
}

func TestWindowsOwnershipUnknownACE(t *testing.T) {
	user := windowsTestUser(t)
	for _, kind := range []byte{windows.ACCESS_DENIED_ACE_TYPE, 5 /* ACCESS_ALLOWED_OBJECT */, 9 /* ACCESS_ALLOWED_CALLBACK */, 255} {
		sd, err := windows.SecurityDescriptorFromString("O:" + user.String() + "D:P(A;OICI;FA;;;" + user.String() + ")")
		if err != nil {
			t.Fatal(err)
		}
		acl, _, err := sd.DACL()
		if err != nil {
			t.Fatal(err)
		}
		var ace *windows.ACCESS_ALLOWED_ACE
		if err = windows.GetAce(acl, 0, &ace); err != nil {
			t.Fatal(err)
		}
		ace.Header.AceType = kind
		if err = validateWindowsDescriptor(sd, user, true); err == nil {
			t.Fatalf("accepted ACE type %d", kind)
		}
		if ace.Header.AceType != kind {
			t.Fatal("validator mutated ACE")
		}
	}
}

func TestWindowsOwnershipUnsafeSidecarNonmutation(t *testing.T) {
	dir := windowsSecureTestDirectory(t)
	path := filepath.Join(dir, "sidecar.db")
	mustCoexist(t, PrepareCoexistence(context.Background(), path))
	user := windowsTestUser(t)
	sd, err := windows.SecurityDescriptorFromString("O:" + user.String() + "D:P(A;;FA;;;" + user.String() + ")(A;;FR;;;WD)")
	mustCoexist(t, err)
	p, _ := windows.UTF16PtrFromString(path + "-wal")
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	mustCoexist(t, err)
	defer windows.CloseHandle(h)
	before, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	mustCoexist(t, err)
	if err := validateCoexistencePrivacy(path); err == nil {
		t.Fatal("accepted unsafe existing WAL")
	}
	after, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	mustCoexist(t, err)
	if before.String() != after.String() {
		t.Fatal("rejected WAL descriptor mutated")
	}
}

func TestWindowsOwnershipShortAndReparseAliases(t *testing.T) {
	dir := windowsSecureTestDirectory(t)
	path := filepath.Join(dir, "LongDatabaseNameForAlias.db")
	mustCoexist(t, PrepareCoexistence(context.Background(), path))
	t.Run("short-name", func(t *testing.T) {
		p, _ := windows.UTF16PtrFromString(path)
		buf := make([]uint16, 32768)
		n, err := windows.GetShortPathName(p, &buf[0], uint32(len(buf)))
		mustCoexist(t, err)
		if n == 0 || n >= uint32(len(buf)) {
			t.Fatal("invalid short path result")
		}
		alias := windows.UTF16ToString(buf[:n])
		if alias == path {
			t.Skip("filesystem does not supply a distinct short name")
		}
		if err := validateOwnershipPath(alias); err == nil {
			t.Fatal("accepted short-name alias")
		}
	})
	t.Run("reparse", func(t *testing.T) {
		alias := filepath.Join(dir, "link.db")
		if err := os.Symlink(path, alias); err != nil {
			t.Skipf("symlink unavailable without changing privileges: %v", err)
		}
		if err := validateOwnershipPath(alias); err == nil {
			t.Fatal("accepted reparse alias")
		}
	})
}

// A create-sibling grant on an ancestor must not prevent using an existing
// private child, including creation of the database and durable ownership lock.
func TestWindowsSecureChildSiblingCreateAncestorDurableAcquisition(t *testing.T) {
	user := windowsTestUser(t)
	root := windowsSecureTestDirectory(t)
	makeDir := func(path, grants string) {
		sd, err := windows.SecurityDescriptorFromString("O:" + user.String() + "D:P(A;OICI;FA;;;" + user.String() + ")" + grants)
		mustCoexist(t, err)
		p, err := windows.UTF16PtrFromString(path)
		mustCoexist(t, err)
		mustCoexist(t, windows.CreateDirectory(p, &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}))
	}
	ancestor := filepath.Join(root, "SiblingCreateAncestor")
	makeDir(ancestor, "(A;;0x2;;;WD)")
	child := filepath.Join(ancestor, "PrivateChild")
	makeDir(child, "")
	snapshot := func(path string) string {
		sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
		mustCoexist(t, err)
		return sd.String()
	}
	beforeAncestor, beforeChild := snapshot(ancestor), snapshot(child)
	mustCoexist(t, validatePrivateDirectory(child))
	path := filepath.Join(child, "durable.db")
	for _, absent := range []string{path, path + ".ownership-locks"} {
		if _, err := os.Lstat(absent); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("fixture must be absent %q: %v", absent, err)
		}
	}
	ctx := context.Background()
	mustCoexist(t, PrepareCoexistence(ctx, path))
	s, err := OpenCoexistence(ctx, path, CurrentCoexistenceDeclaration())
	mustCoexist(t, err)
	defer s.Close()
	owner, err := AcquireOwnership(ctx, s, "conversation", "A")
	mustCoexist(t, err)
	defer owner.Close()
	mustCoexist(t, s.EnsureConversation(WithOwnership(ctx, owner), "conversation", "", ""))
	mustCoexist(t, validateCoexistencePrivacy(path))
	entries, err := os.ReadDir(path + ".ownership-locks")
	mustCoexist(t, err)
	if len(entries) != 1 {
		t.Fatalf("expected newly created ownership lock: %v", entries)
	}
	mustCoexist(t, owner.Close())
	other, err := AcquireOwnership(ctx, s, "conversation", "B")
	if other != nil {
		defer other.Close()
	}
	var conflict *OwnershipConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("Close must retain durable ownership: %v", err)
	}
	if snapshot(ancestor) != beforeAncestor || snapshot(child) != beforeChild {
		t.Fatal("acquisition mutated existing directory ACLs")
	}
}

// Regression reproducer: a private immediate parent does not protect its name
// against FILE_DELETE_CHILD granted on an ancestor. Native execution required.
func TestWindowsSecureChildUnsafeAncestorNonmutation(t *testing.T) {
	user := windowsTestUser(t)
	root := t.TempDir()
	makeDir := func(path, grants string) {
		sd, err := windows.SecurityDescriptorFromString("O:" + user.String() + "D:P(A;OICI;FA;;;" + user.String() + ")" + grants)
		mustCoexist(t, err)
		p, _ := windows.UTF16PtrFromString(path)
		mustCoexist(t, windows.CreateDirectory(p, &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}))
	}
	ancestor := filepath.Join(root, "UnsafeAncestor")
	// SDDL DC is mask 0x2 (create sibling), not FILE_DELETE_CHILD.
	makeDir(ancestor, "(A;;0x40;;;WD)")
	child := filepath.Join(ancestor, "PrivateChild")
	makeDir(child, "")
	snapshot := func(path string) string {
		sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
		mustCoexist(t, err)
		return sd.String()
	}
	beforeAncestor, beforeChild := snapshot(ancestor), snapshot(child)
	// Isolate the defect even if the runner's volume root is also unsafe.
	ancestorSD, err := windows.SecurityDescriptorFromString(beforeAncestor)
	mustCoexist(t, err)
	if err := validateWindowsDescriptorPolicy(ancestorSD, user, true, true); err == nil {
		t.Error("accepted ancestor FILE_DELETE_CHILD grant")
	}
	childSD, err := windows.SecurityDescriptorFromString(beforeChild)
	mustCoexist(t, err)
	mustCoexist(t, validateWindowsDescriptor(childSD, user, true))
	database := filepath.Join(child, "must-not-exist.db")
	if err := PrepareCoexistence(context.Background(), database); err == nil {
		t.Error("accepted private child beneath replaceable ancestor")
	}
	if snapshot(ancestor) != beforeAncestor || snapshot(child) != beforeChild {
		t.Fatal("refusal mutated ACL")
	}
	entries, err := os.ReadDir(child)
	mustCoexist(t, err)
	if len(entries) != 0 {
		t.Fatal("refusal created database or lock artifacts")
	}
}

func TestWindowsAncestorDescriptorPolicy(t *testing.T) {
	user := windowsTestUser(t)
	for _, tc := range []struct {
		rights string
		ok     bool
	}{
		{"FRFX", true}, {"0x2", true}, {"0x4", true}, {"0x1200af", true}, {"0x46", false}, {"0x1301ff", false}, {"0x40", false}, {"SD", false}, {"WD", false}, {"WO", false}, {"FW", false}, {"GA", false},
	} {
		t.Run(tc.rights, func(t *testing.T) {
			sd, err := windows.SecurityDescriptorFromString("O:SYD:P(A;;FA;;;SY)(A;;" + tc.rights + ";;;WD)")
			mustCoexist(t, err)
			before := sd.String()
			err = validateWindowsDescriptorPolicy(sd, user, true, true)
			if (err == nil) != tc.ok {
				t.Fatalf("accept=%v: %v", tc.ok, err)
			}
			if sd.String() != before {
				t.Fatal("mutated descriptor")
			}
		})
	}
}

func TestWindowsMalformedACLBounds(t *testing.T) {
	// ACL header, one allow ACE, one revision-1 SID with zero subauthorities.
	valid := []byte{2, 0, 24, 0, 1, 0, 0, 0, 0, 0, 16, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 5}
	if _, err := parseWindowsACL(valid); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		offset int
		value  byte
	}{
		{"revision", 0, 0}, {"acl-too-small", 2, 7}, {"acl-overrun", 2, 25},
		{"ace-count-overrun", 4, 2}, {"ace-zero", 10, 0}, {"ace-overrun", 10, 20},
		{"ace-unaligned", 10, 17}, {"sid-revision", 16, 2},
		{"sid-truncated", 17, 1}, {"sid-count-overflow", 17, 255},
		{"unsupported-ace", 8, 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := append([]byte(nil), valid...)
			raw[tc.offset] = tc.value
			before := string(raw)
			if _, err := parseWindowsACL(raw); err == nil {
				t.Fatal("accepted malformed structure")
			}
			if string(raw) != before {
				t.Fatal("mutated structure")
			}
		})
	}
	for n := 0; n < len(valid); n++ {
		if _, err := parseWindowsACL(valid[:n]); err == nil {
			t.Fatalf("accepted truncation %d", n)
		}
	}
}

// Synthetic descriptors model a system root without modifying any live ACL.
func TestWindowsAncestorTrustedInstallerPolicy(t *testing.T) {
	// Fixed ordinary-user SID keeps owner-negative cases meaningful under SYSTEM.
	user, err := windows.StringToSid("S-1-5-21-1-2-3-1005")
	mustCoexist(t, err)
	for _, tc := range []struct {
		name, owner, rights string
		ancestor, ok        bool
	}{
		{"system-root-siblings", windowsTrustedInstallerSID, "0x1200af", true, true},
		{"system-root-inherit-only", windowsTrustedInstallerSID, "FRFX", true, true},
		{"untrusted-owner", "S-1-5-21-1-2-3-1006", "FRFX", true, false},
		{"service-sid-near-match", "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478465", "FRFX", true, false},
		{"delete-child", windowsTrustedInstallerSID, "0x40", true, false},
		{"replace-sibling", windowsTrustedInstallerSID, "0x46", true, false},
		{"delete-self", windowsTrustedInstallerSID, "SD", true, false},
		{"write-dacl", windowsTrustedInstallerSID, "WD", true, false},
		{"write-owner", windowsTrustedInstallerSID, "WO", true, false},
		{"generic-write", windowsTrustedInstallerSID, "GW", true, false},
		{"generic-all", windowsTrustedInstallerSID, "GA", true, false},
		{"protected-object-owner", windowsTrustedInstallerSID, "FRFX", false, false},
		{"protected-object-sibling-grant", user.String(), "0x6", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sddl := "O:" + tc.owner + "D:P(A;OICI;FA;;;" + user.String() + ")(A;;FA;;;SY)(A;;FA;;;BA)(A;;" + tc.rights + ";;;WD)"
			if tc.name == "system-root-inherit-only" {
				sddl += "(A;OICIIO;GA;;;AU)"
			}
			sd, err := windows.SecurityDescriptorFromString(sddl)
			mustCoexist(t, err)
			before := sd.String()
			err = validateWindowsDescriptorPolicy(sd, user, true, tc.ancestor)
			if sd.String() != before {
				t.Fatal("descriptor mutated")
			}
			if (err == nil) != tc.ok {
				t.Fatalf("accept=%v: %v", tc.ok, err)
			}
		})
	}
	// The owner exception does not grant TrustedInstaller broad DACL rights.
	for _, ancestor := range []bool{false, true} {
		sd, err := windows.SecurityDescriptorFromString("O:" + user.String() + "D:P(A;OICI;FA;;;" + user.String() + ")(A;;FA;;;" + windowsTrustedInstallerSID + ")")
		mustCoexist(t, err)
		before := sd.String()
		if err := validateWindowsDescriptorPolicy(sd, user, true, ancestor); err == nil {
			t.Fatal("accepted TrustedInstaller DACL grant")
		}
		if sd.String() != before {
			t.Fatal("descriptor mutated")
		}
	}
}
