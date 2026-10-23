//go:build !unix

package installcontext

import "cercano/source/server/internal/updatecoord/privdir"

func verifyBootImage(path string) error {
	if _, err := regularBootImage(path); err != nil {
		return err
	}
	f, err := privdir.OpenPrivateStateFile(path)
	if err != nil {
		return err
	}
	return f.Close()
}
