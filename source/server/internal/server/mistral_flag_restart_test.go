package server

import (
	"context"
	"sync"
	"testing"

	"cercano/source/server/internal/localruntime"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
)

// lifecycleRecorder records the lifecycle calls made on a runtime manager.
type lifecycleRecorder struct {
	*localruntime.InMemoryManager
	mu       sync.Mutex
	stops    []string
	starts   []localruntime.StartRequest
	restarts []localruntime.RestartRequest
}

func (m *lifecycleRecorder) Stop(_ context.Context, req localruntime.StopRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stops = append(m.stops, req.InstanceID)
	return nil
}

func (m *lifecycleRecorder) Start(_ context.Context, req localruntime.StartRequest) (*localruntime.InstanceRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.starts = append(m.starts, req)
	return &localruntime.InstanceRecord{ID: "started", Runtime: req.Runtime, ModelID: req.ModelID, State: localruntime.InstanceRunning}, nil
}

func (m *lifecycleRecorder) Restart(_ context.Context, req localruntime.RestartRequest) (*localruntime.InstanceRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restarts = append(m.restarts, req)
	return &localruntime.InstanceRecord{ID: "restarted", Runtime: "mistralrs", ModelID: req.ModelID, State: localruntime.InstanceRunning}, nil
}

// TestUpdateConfig_MistralFlagChangeRestartsSidecar: changing a mistral.rs
// launch flag restarts the running sidecar through Restart, which really
// relaunches it (and moves holds) even when other agents share it on the
// model host; a Stop+Start there would only release and re-take this agent's
// hold on the unchanged process.
func TestUpdateConfig_MistralFlagChangeRestartsSidecar(t *testing.T) {
	s, _ := newTestServer()
	s.cfgSvc.Mutate(func(c *config.Config) {
		c.OpenRuntime = "mistralrs"
		c.MistralRS.DefaultModel = "new-model"
	})
	mgr := &lifecycleRecorder{InMemoryManager: localruntime.NewManager()}
	mgr.UpdateInstance(localruntime.InstanceRecord{ID: "mistralrs:old:1", Runtime: "mistralrs", ModelID: "old-model", State: localruntime.InstanceRunning})
	s.SetRuntimeManager(mgr)

	resp, err := s.UpdateConfig(t.Context(), &proto.UpdateConfigRequest{MistralrsIsq: "Q4K"})
	if err != nil || !resp.Success {
		t.Fatalf("UpdateConfig: err=%v resp=%+v", err, resp)
	}
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	if len(mgr.stops) != 0 || len(mgr.starts) != 0 {
		t.Fatalf("flag restart used Stop %v / Start %v, want Restart", mgr.stops, mgr.starts)
	}
	if len(mgr.restarts) != 1 {
		t.Fatalf("Restart calls = %d, want 1", len(mgr.restarts))
	}
	if got := mgr.restarts[0]; got.InstanceID != "mistralrs:old:1" || got.ModelID != "new-model" {
		t.Fatalf("Restart request = %+v, want the running instance on the configured model", got)
	}
}
