package main

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"cercano/source/server/internal/crashlog"
	"cercano/source/server/internal/localruntime"
	"cercano/source/server/pkg/config"
)

// An embedded nil Manager traps any unintended runtime operation; no native
// manager, process, store, configuration loader or transport is constructed.
type compositionRuntimeFake struct {
	localruntime.Manager
	calls  *[]string
	closed int
}

func (f *compositionRuntimeFake) SetEndpoints([]localruntime.EndpointRecord) {
	*f.calls = append(*f.calls, "endpoints")
}
func (f *compositionRuntimeFake) Close() error { f.closed++; return nil }

func runtimeFixture(t *testing.T) (productionRuntimeDependencies, *compositionRuntimeFake, *[]string) {
	t.Helper()
	calls := []string{}
	f := &compositionRuntimeFake{calls: &calls}
	return productionRuntimeDependencies{
		probe:  func(string) error { calls = append(calls, "probe"); return errors.New("offline") },
		mode:   func() string { return "" },
		remote: func(context.Context) (productionRuntimeRemote, error) { calls = append(calls, "remote"); return f, nil },
		local:  func(config.Config, *crashlog.Writer) localruntime.Manager { calls = append(calls, "local"); return f },
		warm: func(m localruntime.Manager, _ config.Config) {
			if m != f {
				t.Fatal("wrong manager")
			}
			calls = append(calls, "warm")
		},
	}, f, &calls
}

func TestProductionRuntimeIndependentCleanup(t *testing.T) {
	a, fa, ca := runtimeFixture(t)
	b, fb, cb := runtimeFixture(t)
	if warning := ollamaStartupWarning(a.probe, "fixture"); warning == "" {
		t.Fatal("probe warning lost")
	}
	ma, closeA, err := a.connect(context.Background(), config.Defaults(), nil)
	if err != nil || ma != fa {
		t.Fatalf("A connect: %v", err)
	}
	mb, closeB, err := b.connect(context.Background(), config.Defaults(), nil)
	if err != nil || mb != fb {
		t.Fatalf("B connect: %v", err)
	}
	if !reflect.DeepEqual(*ca, []string{"probe", "remote", "endpoints", "warm"}) || !reflect.DeepEqual(*cb, []string{"remote", "endpoints", "warm"}) {
		t.Fatalf("ordering: %v %v", *ca, *cb)
	}
	var cleanup sync.WaitGroup
	for i := 0; i < 8; i++ {
		cleanup.Add(1)
		go func() { defer cleanup.Done(); closeA() }()
	}
	cleanup.Wait()
	if fa.closed != 1 || fb.closed != 0 {
		t.Fatal("A cleanup affected B or repeated")
	}
	b.warm(mb, config.Defaults())
	closeB()
	closeB()
	if fb.closed != 1 {
		t.Fatal("B cleanup not independent")
	}
}

func TestProductionRuntimeFallbackAndCancellation(t *testing.T) {
	for _, mode := range []string{"inprocess", "remote-error", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			d, f, calls := runtimeFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "inprocess" {
				d.mode = func() string { return " InProcess " }
			}
			d.remote = func(context.Context) (productionRuntimeRemote, error) {
				*calls = append(*calls, "remote")
				return nil, errors.New("fixture unavailable")
			}
			if mode == "cancelled" {
				cancel()
			}
			m, closeFn, err := d.connect(ctx, config.Defaults(), nil)
			if mode == "cancelled" {
				if !errors.Is(err, context.Canceled) || m != nil || closeFn != nil || !reflect.DeepEqual(*calls, []string{"remote"}) {
					t.Fatalf("cancellation fell back: %v %v", err, *calls)
				}
				return
			}
			if err != nil || m != f || closeFn == nil {
				t.Fatalf("fallback: %v", err)
			}
			closeFn()
			want := []string{"local"}
			if mode == "remote-error" {
				want = []string{"remote", "local"}
			}
			if !reflect.DeepEqual(*calls, want) || f.closed != 0 {
				t.Fatalf("fallback ordering/cleanup: %v", *calls)
			}
		})
	}
}

func TestProductionRuntimeAdmissionBeforeOpen(t *testing.T) {
	for _, a := range []productionStartupAdmission{
		{},
		{paths: productionAuthorityPaths{config: "never-open-config", conversations: "never-open-store"}},
	} {
		d, _, calls := runtimeFixture(t)
		_, cleanup, err := startGRPCServerWithRuntime(config.Config{}, "", nil, productionCredentialDependencies{}, a, d)
		if err == nil || cleanup != nil || len(*calls) != 0 {
			t.Fatalf("rejected admission used dependencies: %v %v", err, *calls)
		}
	}
	// A malformed injection also fails before enterprise/credentials/probes.
	a := productionStartupAdmission{legacy: true, paths: productionAuthorityPaths{config: "never-open-config", conversations: "never-open-store"}}
	if _, _, err := startGRPCServerWithRuntime(config.Config{}, "", nil, productionCredentialDependencies{}, a, productionRuntimeDependencies{}); err == nil {
		t.Fatal("missing dependencies accepted")
	}
}
