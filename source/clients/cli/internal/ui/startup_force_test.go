package ui

import (
	"reflect"
	"runtime"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Inspect the actual startup batch without executing unrelated network/event
// commands. Legacy force commands remain callable explicitly by their tests.
func TestInitDoesNotStartForceDiscoveryOrPolling(t *testing.T) {
	m := New(nil, false)
	batch, ok := m.Init()().(tea.BatchMsg)
	if !ok || len(batch) == 0 {
		t.Fatal("missing startup batch")
	}
	for _, cmd := range batch {
		name := runtime.FuncForPC(reflect.ValueOf(cmd).Pointer()).Name()
		if strings.Contains(strings.ToLower(name), "force") {
			t.Fatalf("startup schedules retired force control: %s", name)
		}
	}
}
