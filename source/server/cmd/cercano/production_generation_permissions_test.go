package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"cercano/source/server/internal/agent"
	"cercano/source/server/pkg/proto"
)

func TestProductionGenerationPermissionsHost(t *testing.T) {
	for _, supplied := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing-strict", true: "shared-policy"}[supplied], func(t *testing.T) {
			o, auth := developmentProductionFixture(t)
			path := filepath.Join(o.State, "permissions.yaml")
			if supplied {
				if err := os.WriteFile(path, []byte("mode: strict\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			d := enabledDevelopmentFakes(t, developmentProductionDependencies{})
			srv, cleanup, err := newProductionDevelopmentHostWithDependencies(context.Background(), o, auth, d)
			if cleanup != nil {
				defer cleanup.Wait(context.Background())
			}
			if err != nil {
				t.Fatal(err)
			}
			check := func(want string) {
				t.Helper()
				got, err := srv.GetPermissionMode(context.Background(), &proto.GetPermissionModeRequest{})
				if err != nil || got.GetMode() != want {
					t.Fatalf("permission = %v, %v; want %s", got, err, want)
				}
			}
			check("strict")
			if supplied {
				peer, err := agent.LoadPermissionStore(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := peer.SetMode(agent.ModePermissive); err != nil {
					t.Fatal(err)
				}
				check("permissive")
				if err := os.WriteFile(path, []byte("mode: invalid\n"), 0600); err != nil {
					t.Fatal(err)
				}
				check("permissive") // ordinary last-known validated policy
			}
			// Persistent administrative mutations still require their ownership
			// integration; installing the real store does not remove that guard.
			if _, err := srv.SetPermissionMode(context.Background(), &proto.SetPermissionModeRequest{Mode: "bypass"}); err == nil {
				t.Fatal("coexistence mutation guard removed")
			}
			if err := srv.RetireCoexistence(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
