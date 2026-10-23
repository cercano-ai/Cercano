package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cercano/source/server/internal/testenv"
	"cercano/source/server/pkg/paths"
	"cercano/source/server/pkg/proto"
)

// Servers built here save config (machine.yaml included) and write profile
// logs through the default paths. TestMain must keep all of that under a
// temporary home, never the developer's real ~/.config/cercano.
func TestServerTestsWriteUnderTemporaryHome(t *testing.T) {
	home := testenv.Home()
	if home == "" {
		t.Fatal("TestMain does not call testenv.IsolateHome: this package's tests write into the real home")
	}
	if root := paths.ConfigRoot(); !strings.HasPrefix(root, home+string(filepath.Separator)) {
		t.Fatalf("paths.ConfigRoot() = %q, want it under the temporary home %q", root, home)
	}
	s := routingUpdateServer(t, "open_only")
	if reply, err := s.UpdateConfig(t.Context(), &proto.UpdateConfigRequest{LocusMode: "cloud_only"}); err != nil || !reply.Success {
		t.Fatalf("UpdateConfig: %v %+v", err, reply)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "cercano", "machine.yaml")); err != nil {
		t.Fatalf("saving config did not write machine.yaml under the temporary home: %v", err)
	}
}
