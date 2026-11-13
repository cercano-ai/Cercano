//go:build windows

package config

import (
	"errors"
	"os"
	"syscall"
)

// configLinkCount asks Windows for the file's hard-link count. os.Lstat on
// Windows returns Win32FileAttributeData, which has no link count, so the
// Unix Nlink path refused every existing configuration.
func configLinkCount(path string, info os.FileInfo) (uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return 0, err
	}
	// The handle must be the file that was checked, not a replacement.
	if !os.SameFile(info, opened) {
		return 0, errors.New("configuration authority changed during verification")
	}
	var data syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &data); err != nil {
		return 0, err
	}
	return uint64(data.NumberOfLinks), nil
}
