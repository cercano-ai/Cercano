package main

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
	"unsafe"
)

func windowsUpgradeImage(name string) bool {
	name = strings.ToLower(filepath.Base(name))
	return name == "cercano.exe" || name == "cercano-agent.exe"
}

// Toolhelp supplies the bounded image-name candidate set. Each candidate's
// retained process handle binds token, image and liveness checks to one process,
// not a reusable PID. A failed open/query is uncertainty, never proof of exit.
func knownUpgradeAgents(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return fmt.Errorf("cannot enumerate known Cercano agents: %w", err)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	err = windows.Process32First(snapshot, &entry)
	for err == nil {
		if e := ctx.Err(); e != nil {
			return e
		}
		if entry.ProcessID != uint32(os.Getpid()) && windowsUpgradeImage(windows.UTF16ToString(entry.ExeFile[:])) {
			if e := checkWindowsUpgradeProcess(entry.ProcessID, user.User.Sid); e != nil {
				return e
			}
		}
		err = windows.Process32Next(snapshot, &entry)
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return fmt.Errorf("incomplete process enumeration: %w", err)
	}
	return nil
}

func checkWindowsUpgradeProcess(pid uint32, current *windows.SID) error {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return fmt.Errorf("known Cercano pid=%d cannot be inspected; stop writers and retry: %w", pid, err)
	}
	defer windows.CloseHandle(h)
	return checkWindowsUpgradeProcessHandle(h, pid, current)
}

func checkWindowsUpgradeProcessHandle(h windows.Handle, pid uint32, current *windows.SID) error {
	exited := func() (bool, error) {
		status, e := windows.WaitForSingleObject(h, 0)
		if e != nil {
			return false, e
		}
		if status == windows.WAIT_OBJECT_0 {
			return true, nil
		}
		if status != uint32(windows.WAIT_TIMEOUT) {
			return false, fmt.Errorf("uncertain process wait status %d", status)
		}
		return false, nil
	}
	if done, e := exited(); e != nil {
		return e
	} else if done {
		return nil
	}
	var token windows.Token
	err := windows.OpenProcessToken(h, windows.TOKEN_QUERY, &token)
	if err == nil {
		var owner *windows.Tokenuser
		owner, err = token.GetTokenUser()
		token.Close()
		if err == nil && !owner.User.Sid.Equals(current) {
			return nil
		}
	}
	var image [32768]uint16
	size := uint32(len(image))
	if err == nil {
		err = windows.QueryFullProcessImageName(h, 0, &image[0], &size)
	}
	if done, e := exited(); e != nil {
		return e
	} else if done {
		return nil
	}
	if err != nil {
		return fmt.Errorf("known Cercano pid=%d identity uncertain: %w", pid, err)
	}
	// A snapshot/image discrepancy is also uncertainty, not a nonwriter exemption.
	return fmt.Errorf("offline upgrade refused: same-user known Cercano process pid=%d image=%s is active; stop ALL legacy Cercano agents and retry", pid, windows.UTF16ToString(image[:size]))
}
