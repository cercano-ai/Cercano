package ui

import (
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"path/filepath"
	"testing"
)

func TestDevHostOrdinaryReopenStartsCommand(t *testing.T) {
	t.Setenv("CERCANO_UI_CONFIG", filepath.Join(t.TempDir(), "ui.yaml"))
	m := New(&agentclient.Client{}, false).WithDevHost(DevHostSelection{Conversation: "new"}, "test")
	_, cmd := m.applyResume("original")
	if cmd == nil {
		t.Fatal("ordinary reopen rejected instead of starting bounded lifecycle command")
	}
}

func TestFindReopenOwnerFakeControls(t *testing.T) {
	for _, tc := range []struct {
		name  string
		paths []string
		want  string
		fail  bool
	}{
		{"stale before actual owner", []string{"stale", "A"}, "A", false},
		{"stale after current owner", []string{"B", "stale"}, "B", false},
		{"unreachable possible actual owner", []string{"empty", "stale"}, "", true},
		{"conflicting live owners", []string{"A", "B"}, "", true},
		{"known unowned", []string{"empty"}, "", false},
		{"invalid authentication is fatal", []string{"auth", "A"}, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source, owner, err := findReopenOwner(tc.paths, "original", func(host string) (DevHostSelection, *proto.LifecycleControlResponse, error) {
				s := DevHostSelection{Identity: &proto.LifecycleIdentity{Host: host}}
				if host == "auth" {
					return s, nil, status.Error(codes.Unauthenticated, "invalid fixture token")
				}
				if host == "stale" {
					return s, nil, status.Error(codes.Unavailable, "unreachable fake control")
				}
				r := &proto.LifecycleControlResponse{Identity: s.Identity}
				if host != "empty" {
					r.Retained = []*proto.LifecycleOwner{{Conversation: "original", Host: host, Epoch: "epoch"}}
				}
				return s, r, nil
			})
			if (err != nil) != tc.fail {
				t.Fatalf("owner=%v err=%v", owner, err)
			}
			if tc.want != "" && (owner == nil || source.Identity.Host != tc.want) {
				t.Fatalf("owner=%v source=%v", owner, source)
			}
		})
	}
}
