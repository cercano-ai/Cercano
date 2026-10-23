package installcontext

import (
	"errors"
	"fmt"
	"io"
	"os"

	"cercano/source/server/internal/updatecoord/activation"
	"cercano/source/server/internal/updatecoord/privdir"
)

// readBootSelection reads only the handle whose owner/access/link policy was
// verified by privdir. Never replace this with a policy check followed by a
// pathname-based activation.ReadSelection: that would read an unbound object.
// The caller holds the directory-bound launch lease throughout both reads.
func readBootSelection(path string) (activation.Selection, error) {
	var zero activation.Selection
	unreadable := func(err error) (activation.Selection, error) {
		return zero, fmt.Errorf("%w: %w", activation.ErrSelectionUnreadable, err)
	}
	before, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return zero, activation.ErrSelectionAbsent
		}
		return unreadable(err)
	}
	if !before.Mode().IsRegular() {
		return unreadable(privdir.ErrUnsafePath)
	}
	f, err := privdir.OpenPrivateStateFile(path)
	if err != nil {
		return unreadable(err)
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return unreadable(err)
	}
	if !os.SameFile(before, opened) {
		return unreadable(privdir.ErrUnsafePath)
	}
	// Mirrors activation's decoder bound; ParseSelection remains authoritative.
	data, err := io.ReadAll(io.LimitReader(f, 4*1024+1))
	if err != nil {
		return unreadable(err)
	}
	after, err := os.Lstat(path)
	if err != nil {
		return unreadable(err)
	}
	if !after.Mode().IsRegular() || !os.SameFile(opened, after) {
		return unreadable(privdir.ErrUnsafePath)
	}
	// Re-prove policy after reading, binding the recheck to the SAME object.
	// Contents above come exclusively from f, never from this second handle.
	checked, err := privdir.OpenPrivateStateFile(path)
	if err != nil {
		return unreadable(err)
	}
	defer checked.Close()
	info, err := checked.Stat()
	if err != nil {
		return unreadable(err)
	}
	if !os.SameFile(opened, info) {
		return unreadable(privdir.ErrUnsafePath)
	}
	selected, err := activation.ParseSelection(data)
	if err != nil {
		return zero, fmt.Errorf("%w: %w", activation.ErrSelectionMalformed, err)
	}
	return selected, nil
}
