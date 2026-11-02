package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/engine"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/loopcompact"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/internal/server"
	"cercano/source/server/pkg/config"
	"fmt"
	"strings"
	"sync"
)

// Exercise the same constructor binding used by the generation production host,
// not a direct generator invocation. No credential policy override is needed.
func TestProductionPreparedSchedulingOwnership(t *testing.T) {
	for _, producer := range []string{"recap", "compaction"} {
		for _, active := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/active=%v", producer, active), func(t *testing.T) { testProductionPreparedSchedulingOwnership(t, producer, active) })
		}
	}
}
func testProductionPreparedSchedulingOwnership(t *testing.T, producer string, active bool) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	ctx := context.Background()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "history.db")
	if err := conversation.PrepareDevelopmentStore(ctx, path); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Compaction.Enabled = true
	cfg.OpenRuntime = "synthetic"
	routing := composeProductionRouting(productionRoutingDependencies{open: agent.AbsentCloud("fake"), cloud: agent.AbsentCloud("fake"), classificationDisabled: true})
	var agents []*agent.Agent
	var hosts []*server.Server
	var starts []chan struct{}
	var unblocks []func()
	debounce := time.Hour
	if active {
		debounce = time.Millisecond
	}
	cfg.Compaction.ActivationFloorTokens = 1000
	cfg.Compaction.SegmentTokens = 4000
	cfg.Compaction.VerbatimRecent = 2
	cfg.LocusMode = "open_only"
	cfg.TaskAssignments = map[config.Task]config.TaskAssignment{config.TaskCompaction: {Destination: config.DestinationLocal, Quality: config.CostEconomy}}
	for _, host := range []string{"A", "B"} {
		var a *agent.Agent
		entered, resume := make(chan struct{}), make(chan struct{})
		var once, closeOnce sync.Once
		unblock := func() { closeOnce.Do(func() { close(resume) }) }
		t.Cleanup(unblock)
		starts = append(starts, entered)
		unblocks = append(unblocks, unblock)
		call := func(c context.Context) error { once.Do(func() { close(entered) }); <-resume; return c.Err() }
		s, err := newProductionPreparedServer(ctx, server.PreparedServerOptions{
			Config: cfg, Secrets: secrets.NewMemory(), ConfigPath: filepath.Join(dir, host+".yaml"), RoutingPath: filepath.Join(dir, host+".routing"), FailurePath: filepath.Join(dir, host+".failures"),
			Coexistence: server.CoexistenceOptions{StorePath: path, Host: host, Build: "fixture", CloseParticipation: func(context.Context) error { return nil }, Contract: conversation.DevelopmentCoexistenceDeclaration()},
			Agent: func(st conversation.Store) (*agent.Agent, error) {
				var err error
				a, _, err = composeProductionAgent(routing, st, cfg, productionScheduling{})
				return a, err
			},
		}, routing, nil, engine.NewEngineRegistry(), productionSchedulingDependencies{loop: loopcompact.WiringDeps{Cfg: cfg, ChatModel: "fixture", Candidates: func() inference.Tiers { return inference.Tiers{Open: &schedulingProvider{call: call}} }}}, productionSchedulingOptions{recapDebounce: debounce, compactionDebounce: debounce})
		if err != nil {
			t.Fatal(err)
		}
		hosts = append(hosts, s)
		agents = append(agents, a)
		t.Cleanup(func() { unblock(); s.Shutdown(); s.CloseCoexistenceStore() })
	}
	for i, s := range hosts {
		id := []string{"a", "b"}[i]
		owner, err := s.Retain(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		owned, ticket, err := s.BeginRetainedWork(ctx, owner, broker.ForegroundWork)
		if err != nil {
			t.Fatal(err)
		}
		a := agents[i]
		if active {
			for j := 0; j < 12; j++ {
				if err := a.PersistentStore().Append(owned, conversation.Turn{ID: fmt.Sprintf("%s-%d", id, j), ConversationID: id, Role: "user", Content: strings.Repeat("fixture text ", 1000), CreatedAt: time.Now().Add(time.Duration(j-12) * time.Second)}); err != nil {
					t.Fatal(err)
				}
			}
		}
		schedule := a.ScheduleRecapContext
		crossSchedule := agents[1-i].ScheduleRecapContext
		if producer == "compaction" {
			schedule = a.ScheduleCompactionContext
			crossSchedule = agents[1-i].ScheduleCompactionContext
		}
		{
			if err := schedule(ctx, id); !errors.Is(err, broker.ErrAdmissionStale) {
				t.Fatalf("ambient: %v", err)
			}
			if err := schedule(owned, "sibling"); !errors.Is(err, broker.ErrAdmissionStale) {
				t.Fatalf("cross-owner: %v", err)
			}
			if err := schedule(owned, id); err != nil {
				t.Fatal(err)
			}
		}
		if err := crossSchedule(owned, id); !errors.Is(err, broker.ErrAdmissionStale) {
			t.Fatalf("cross-host: %v", err)
		}
		if active {
			select {
			case <-starts[i]:
			case <-time.After(5 * time.Second):
				t.Fatal("producer did not run")
			}
		}
		if err := ticket.Complete(); err != nil {
			t.Fatal(err)
		}
		if err := s.ReleaseRetained(ctx, owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
			t.Fatalf("pending producers not counted: %v", err)
		}
		s.FenceBackgroundWork()
		if err := schedule(owned, id); err == nil {
			t.Fatal("fenced scheduling accepted")
		}
		joined := make(chan struct{})
		go func() { s.JoinBackgroundWork(); close(joined) }()
		if active {
			select {
			case <-joined:
				t.Fatal("join returned before provider completion")
			case <-time.After(20 * time.Millisecond):
			}
			unblocks[i]()
		}
		select {
		case <-joined:
		case <-time.After(5 * time.Second):
			t.Fatal("join did not drain")
		}
		err = s.ReleaseRetained(ctx, owner, nil)
		if active && producer != "recap" {
			// Failed/cancelled production execution remains unresolved; joining
			// callbacks is not permission to erase its persistence obligation.
			if !errors.Is(err, broker.ErrAdmissionBusy) {
				t.Fatalf("cancelled work lost closure protection: %v", err)
			}
		} else if err != nil {
			t.Fatalf("pending timer did not drain: %v", err)
		}
	}
}
