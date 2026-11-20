package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"
)

// UpgradeInventory is a Windows-only, read-only retained-host inventory capability.
// Its permanent launch-exclusion lock must remain held through offline migration.
// It grants no general filesystem or lifecycle authority.
type UpgradeInventory struct {
	state string
	lock  *os.File
	once  sync.Once
}

func OpenUpgradeInventory(ctx context.Context, state string) (*UpgradeInventory, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	h, err := windowsPrivateHandle(state, true)
	if err != nil {
		return nil, err
	}
	windows.CloseHandle(h)
	if err := validateWindowsCreationOwner(); err != nil {
		return nil, err
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	sd, err := windows.SecurityDescriptorFromString("O:" + user.User.Sid.String() + "D:P(A;;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return nil, err
	}
	sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	path := filepath.Join(state, "current-target.lock")
	h, err = windowsPrivateHandleAccess(path, false, windows.GENERIC_READ|windows.GENERIC_WRITE, sa, windows.OPEN_ALWAYS)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(h), path)
	for {
		if err = ctx.Err(); err != nil {
			f.Close()
			return nil, err
		}
		err = lockOwnershipFile(f)
		if err == nil {
			return &UpgradeInventory{state: state, lock: f}, nil
		}
		if !ownershipLockContended(err) {
			f.Close()
			return nil, err
		}
		timer := time.NewTimer(20 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			f.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
func (i *UpgradeInventory) Close() {
	i.once.Do(func() {
		windows.UnlockFileEx(windows.Handle(i.lock.Fd()), 0, 1, 0, &windows.Overlapped{})
		i.lock.Close()
	})
}
func (i *UpgradeInventory) Entries() ([]os.DirEntry, error) {
	path := filepath.Join(i.state, "hosts")
	h, err := windowsPrivateHandleAccess(path, true, windows.GENERIC_READ, nil, windows.OPEN_EXISTING)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(h), path)
	defer f.Close()
	return f.ReadDir(-1)
}
func (i *UpgradeInventory) Read(path string, target any) error {
	if filepath.Dir(path) != filepath.Join(i.state, "hosts") || !(strings.HasSuffix(path, ".json") || strings.HasSuffix(path, ".starting")) {
		return fmt.Errorf("outside upgrade inventory")
	}
	h, err := windowsPrivateHandleAccess(path, false, windows.GENERIC_READ, nil, windows.OPEN_EXISTING)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(h), path)
	defer f.Close()
	// Match launchRead's whole-document JSON validation (including trailing data).
	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
