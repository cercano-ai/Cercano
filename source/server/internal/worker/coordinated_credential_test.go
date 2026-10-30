package worker

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"cercano/source/server/internal/cloudfactory"
	"cercano/source/server/internal/credentialtest"
	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/secrets"
	pkgcfg "cercano/source/server/pkg/config"
)

// Entirely in-memory: even the getter is a tripwire, not a native store.
type coordinatedExportStore struct {
	secrets.Store
	gets int
}

func (s *coordinatedExportStore) IsCoordinated() bool { return true }
func (s *coordinatedExportStore) CredentialRevision(context.Context, string) (string, error) {
	return "fake-revision", nil
}
func (s *coordinatedExportStore) SetAtRevision(context.Context, string, string, string) error {
	panic("unexpected publication")
}
func (s *coordinatedExportStore) Get(string) (string, error) {
	s.gets++
	return "fake-raw-token-must-not-export", nil
}

func TestWorkerCoordinatedCredentialExportRejected(t *testing.T) {
	// Override only inside this fully fake scope. The tripwire payload is not a
	// valid subscription record, so even a regression cannot initiate refresh.
	credentialtest.Enable(t)
	profiles := []pkgcfg.CloudProfile{
		{Name: "private-profile", Provider: "anthropic", Flavor: cloudfactory.FlavorMessages},
		{Name: "private-profile", Provider: "openai", Flavor: cloudfactory.FlavorResponses},
		{Name: "private-profile", Flavor: cloudfactory.FlavorMessages, Route: cloudfactory.RouteSubscription},
		{Name: "private-profile", Flavor: cloudfactory.FlavorResponses, Route: cloudfactory.RouteChatGPT},
		{Name: "private-profile", Flavor: cloudfactory.FlavorBedrock},
		{Name: "private-profile", Flavor: "unknown", BaseURL: "https://fake.invalid"},
	}
	for _, p := range profiles {
		for _, readonly := range []bool{false, true} {
			name := p.Flavor + "/" + p.Route
			if readonly {
				name += "/readonly"
			}
			t.Run(name, func(t *testing.T) {
				raw := &coordinatedExportStore{Store: secrets.NewMemory()}
				var store secrets.Store = raw
				if readonly {
					store = secrets.Guard(store, secrets.Options{ReadOnly: true})
				}
				cfg := pkgcfg.Config{CloudProfiles: []pkgcfg.CloudProfile{p}, ActiveCloudProfile: p.Name}
				svc := cfgsvc.New("", cfg, store)
				if !secrets.IsCoordinated(svc.Secrets()) {
					t.Fatal("facade lost coordinated capability")
				}
				w := &workerRunner{cfg: svc}
				token, account, err := w.resolveCredential(context.Background(), cfg, p.Name)
				if token != "" || account != "" {
					t.Error("export returned raw credential/account")
				}
				var failure *llm.CredentialError
				if !errors.As(err, &failure) || llm.ClassOf(err) != llm.ErrCredential {
					t.Errorf("expected typed credential failure, got %v", err)
				}
				if failure != nil && (failure.Method != "worker_export" || failure.Reason != llm.CredentialSource || failure.Cause != nil) {
					t.Error("failure did not originate at the pre-source export boundary")
				}
				if err != nil && (strings.Contains(err.Error(), p.Name) || strings.Contains(err.Error(), "fake-raw-token")) {
					t.Error("failure leaked private metadata")
				}
				if llm.Failoverable(llm.ClassOf(err), err) {
					t.Error("export failure permitted failover")
				}
				if raw.gets != 0 {
					t.Errorf("getter called %d times before rejection", raw.gets)
				}
			})
		}
	}
}

func TestWorkerOrdinaryCredentialExportPreserved(t *testing.T) {
	// Explicitly fake scope only, with no native store, token refresh or transport.
	credentialtest.Enable(t)
	for _, readonly := range []bool{false, true} {
		store := secrets.NewMemory()
		if err := store.Set("fixture", "fake-fixture-key"); err != nil {
			t.Fatal(err)
		}
		var view secrets.Store = store
		if readonly {
			view = secrets.Guard(view, secrets.Options{ReadOnly: true})
		}
		cfg := pkgcfg.Config{CloudProfiles: []pkgcfg.CloudProfile{{Name: "fixture", Flavor: cloudfactory.FlavorMessages}}}
		svc := cfgsvc.New("", cfg, view)
		w := &workerRunner{cfg: svc}
		key, account, err := w.resolveCredential(context.Background(), cfg, "fixture")
		if err != nil || key != "fake-fixture-key" || account != "" {
			t.Fatalf("ordinary fixture changed: %q %q %v", key, account, err)
		}
	}
}

// Check the actual production call site without spawning a worker, connecting a
// socket, or constructing a native credential backend.
func TestWorkerProductionSnapshotHasNoRawCredential(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "host.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		fn, ok := call.Fun.(*ast.Ident)
		if !ok || fn.Name != "SnapshotConfig" {
			return true
		}
		calls++
		if len(call.Args) != 3 {
			t.Fatal("unexpected snapshot signature")
		}
		raw, ok := call.Args[1].(*ast.BasicLit)
		if !ok || raw.Kind != token.STRING || raw.Value != `""` {
			t.Error("production snapshot must pass an empty raw credential")
		}
		return true
	})
	if calls != 1 {
		t.Fatalf("expected one production snapshot call, found %d", calls)
	}
}
