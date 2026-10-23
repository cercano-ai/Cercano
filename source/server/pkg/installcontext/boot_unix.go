//go:build unix

package installcontext

import (
	"cercano/source/server/internal/updatecoord/enrollment"
	"os"
	"syscall"
)

func verifyBootImage(path string) error {
	info, err := regularBootImage(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Getuid()) || stat.Nlink != 1 || info.Mode().Perm()&0022 != 0 || info.Mode().Perm()&0100 == 0 || info.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
		return enrollment.ErrUnsafe
	}
	return nil
}
