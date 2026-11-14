package conversation

import (
	"os"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// windowsUnsafeTestDirectory creates only a new fixture object; never changes
// an existing directory's ACL. Keep the owner in control for test cleanup.
func windowsUnsafeTestDirectory(t *testing.T, path string) {
	t.Helper()
	sid := windowsTestUser(t)
	sd, err := windows.SecurityDescriptorFromString("O:" + sid.String() + "D:P(A;OICI;FA;;;" + sid.String() + ")(A;OICI;FR;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.CreateDirectory(p, &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}); err != nil {
		t.Fatal(err)
	}
}

func windowsDevelopmentTestDescriptor(t *testing.T, path string) string {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	return sd.String()
}

func makeNonprivateDevelopmentTestDirectory(t *testing.T, path string) func() {
	t.Helper()
	windowsUnsafeTestDirectory(t, path)
	before := windowsDevelopmentTestDescriptor(t, path)
	return func() {
		t.Helper()
		if after := windowsDevelopmentTestDescriptor(t, path); after != before {
			t.Errorf("refused parent descriptor mutated: %q -> %q", before, after)
		}
	}
}

// Probe the original fixture's faulty premise: chmod(0755) does not make a
// private Windows DACL public. Both 0700 and 0755 only clear read-only.
func TestWindowsDevelopmentChmodPreservesPrivateDescriptor(t *testing.T) {
	path := windowsSecureTestDirectory(t)
	before := windowsDevelopmentTestDescriptor(t, path)
	for _, mode := range []os.FileMode{0700, 0755} {
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		if after := windowsDevelopmentTestDescriptor(t, path); after != before {
			t.Fatalf("chmod(%o) changed descriptor: %q -> %q", mode, before, after)
		}
		if err := validatePrivateDirectory(path); err != nil {
			t.Fatal(err)
		}
	}
}
