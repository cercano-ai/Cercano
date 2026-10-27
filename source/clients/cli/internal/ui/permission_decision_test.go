package ui

import (
	"errors"
	"strings"
	"testing"
)

func TestPermissionDecisionResultDisplay(t *testing.T) {
	for _, tc := range []struct {
		msg       permissionDecisionMsg
		want      string
		forbidden string
	}{
		{permissionDecisionMsg{allow: true, err: errors.New("binding refused")}, "permission decision failed: binding refused", "approved"},
		{permissionDecisionMsg{allow: true, persist: true, err: errors.New("configuration frozen")}, "permission decision failed: configuration frozen", "always-allowed"},
		{permissionDecisionMsg{allow: true}, "approved — running", "failed"},
		{permissionDecisionMsg{}, "canceled.", "approved"},
	} {
		m := minimalModel()
		next, _ := m.Update(tc.msg)
		updated := next.(Model)
		entries := updated.mainChat().Entries()
		if len(entries) == 0 {
			t.Fatal("missing decision result")
		}
		text := entries[len(entries)-1].Content
		if !strings.Contains(text, tc.want) || strings.Contains(text, tc.forbidden) {
			t.Fatalf("unexpected result: %q", text)
		}
	}
}
