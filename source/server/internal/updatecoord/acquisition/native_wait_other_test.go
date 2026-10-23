//go:build !darwin

package acquisition

import (
	"cercano/source/server/internal/updatecoord/nativeprocess"
	"context"
	"testing"
)

func nativePinUpdater(t *testing.T, _ nativeprocess.Identity) func(context.Context) {
	t.Fatal("native integration requires macOS kernel exit watch")
	return nil
}
