package server

import (
	"cercano/source/server/internal/credentialtest"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
	"reflect"
	"testing"
)

func TestProfileSnapshotRejectedSave(t *testing.T) {
	credentialtest.Enable(t)
	s := routingUpdateServer(t, "open_only")
	before := s.cfgSvc.Get()
	other, err := config.Load(s.cfgSvc.Path())
	if err != nil {
		t.Fatal(err)
	}
	other.Port = "12345"
	if _, err := config.SaveSnapshot(other, s.cfgSvc.Path(), other.Snapshot()); err != nil {
		t.Fatal(err)
	}
	r, err := s.UpsertCloudProfile(t.Context(), &proto.UpsertCloudProfileRequest{Name: "new", Flavor: "messages"})
	if err == nil && r.Ok {
		t.Error("stale profile save reported success")
	}
	if !reflect.DeepEqual(before, s.cfgSvc.Get()) {
		t.Error("rejected profile applied live")
	}
}

func TestProfileSnapshotRejectedSelectionAndRemoval(t *testing.T) {
	for _, operation := range []string{"select", "remove"} {
		t.Run(operation, func(t *testing.T) {
			credentialtest.Enable(t)
			s := routingUpdateServer(t, "open_only")
			before := s.cfgSvc.Get()
			key, _ := s.cfgSvc.Secrets().Get("messages-one")
			other, err := config.Load(s.cfgSvc.Path())
			if err != nil {
				t.Fatal(err)
			}
			other.Port = "12345"
			if _, err := config.SaveSnapshot(other, s.cfgSvc.Path(), other.Snapshot()); err != nil {
				t.Fatal(err)
			}
			if operation == "select" {
				r, err := s.SetActiveCloudProfile(t.Context(), &proto.SetActiveCloudProfileRequest{Name: "messages-one"})
				if err == nil && r.Ok {
					t.Fatal("stale selection accepted")
				}
			} else {
				r, err := s.RemoveCloudProfile(t.Context(), &proto.RemoveCloudProfileRequest{Name: "messages-one"})
				if err == nil && r.Ok {
					t.Fatal("stale removal accepted")
				}
			}
			if !reflect.DeepEqual(before, s.cfgSvc.Get()) {
				t.Fatal("rejected draft applied")
			}
			after, err := s.cfgSvc.Secrets().Get("messages-one")
			if err != nil || after != key {
				t.Fatal("rejected draft changed credential")
			}
		})
	}
}
