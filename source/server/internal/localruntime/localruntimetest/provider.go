// Package localruntimetest provides test doubles for the localruntime package.
package localruntimetest

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"cercano/source/server/internal/localruntime"
)

// FakeProvider is an in-process localruntime.Provider that launches nothing.
// Start returns a running instance with the deterministic ID "fake-<model>".
// Start and Stop calls are counted so tests can assert how often the manager
// reached the provider.
type FakeProvider struct {
	// ProviderName is returned by Name.
	ProviderName string
	// Models is returned by Discover.
	Models []localruntime.ModelRecord
	// UniqueIDs gives every launch a fresh instance ID ("fake-<model>-<n>"),
	// as real providers do when the port is part of the ID.
	UniqueIDs bool
	// OnStart, if set, runs at the start of every Start call, before it is
	// counted. Tests use it to block a launch.
	OnStart func(localruntime.StartRequest)
	// OnStop, if set, runs at the start of every Stop call, before it is
	// counted.
	OnStop func(id string)
	// FailAfterStart makes Start return the live instance together with an
	// error, as a provider does when a launched process is slow to become
	// ready.
	FailAfterStart bool
	// DefaultModel, if set, is launched when Start is asked for no model, as
	// a real provider launches its configured default model.
	DefaultModel string
	// Endpoint and Context, if set, are reported on every started instance,
	// so engines that need a serving endpoint and confirmed capacity can use
	// it.
	Endpoint string
	Context  localruntime.ContextCapacity

	mu         sync.Mutex
	startedID  string
	startCalls int
	stopCalls  int
	stopped    map[string]bool
}

// NewFakeProvider returns a FakeProvider with the given name and models.
func NewFakeProvider(name string, models ...localruntime.ModelRecord) *FakeProvider {
	return &FakeProvider{ProviderName: name, Models: models}
}

func (f *FakeProvider) Name() string { return f.ProviderName }

func (f *FakeProvider) Capabilities() localruntime.RuntimeCapabilities {
	return localruntime.RuntimeCapabilities{
		ManagedProcesses: true,
		CanStart:         true,
		CanStop:          true,
		CanRestart:       true,
		CanListModels:    true,
		CanStreamLogs:    true,
		SupportsChat:     true,
	}
}

func (f *FakeProvider) Discover(context.Context) ([]localruntime.ModelRecord, error) {
	return append([]localruntime.ModelRecord(nil), f.Models...), nil
}

// Start launches nothing and reports the instance. A relaunch of an instance
// previously stopped reports InstanceStarting, as a real provider would: the
// manager's state machine only allows stopped -> starting.
func (f *FakeProvider) Start(_ context.Context, req localruntime.StartRequest, sink localruntime.LogSink) (*localruntime.InstanceRecord, error) {
	if req.ModelID == "" {
		if f.DefaultModel == "" {
			return nil, errors.New("model id required")
		}
		req.ModelID = f.DefaultModel
	}
	if f.OnStart != nil {
		f.OnStart(req)
	}
	// Like real providers, report the canonical ID of the model record the
	// request resolves to, not the alias that was asked for.
	modelID := req.ModelID
	for _, m := range f.Models {
		if m.Runtime == f.ProviderName && localruntime.MatchesModel(req.ModelID, m) {
			modelID = m.ID
			break
		}
	}
	id := "fake-" + modelID
	f.mu.Lock()
	f.startCalls++
	if f.UniqueIDs {
		id = fmt.Sprintf("%s-%d", id, f.startCalls)
	}
	f.startedID = id
	state := localruntime.InstanceRunning
	if f.stopped[id] {
		state = localruntime.InstanceStarting
		delete(f.stopped, id)
	}
	f.mu.Unlock()
	sink.WriteLog(localruntime.LogEntry{Source: "fake", Message: "started " + modelID})
	rec := &localruntime.InstanceRecord{
		ID:       id,
		Runtime:  f.ProviderName,
		ModelID:  modelID,
		State:    state,
		Endpoint: f.Endpoint,
		Context:  f.Context,
	}
	if f.FailAfterStart {
		return rec, errors.New("still loading: instance left running")
	}
	return rec, nil
}

func (f *FakeProvider) Stop(_ context.Context, id string) error {
	if f.OnStop != nil {
		f.OnStop(id)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopCalls++
	if f.stopped == nil {
		f.stopped = map[string]bool{}
	}
	f.stopped[id] = true
	return nil
}

func (f *FakeProvider) Probe(context.Context, string) (*localruntime.InstanceHealth, error) {
	return &localruntime.InstanceHealth{State: localruntime.InstanceHealthy}, nil
}

// StartCalls reports how many times Start has been called.
func (f *FakeProvider) StartCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.startCalls
}

// StopCalls reports how many times Stop has been called.
func (f *FakeProvider) StopCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stopCalls
}

// StartedID reports the ID of the most recently started instance.
func (f *FakeProvider) StartedID() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.startedID
}
