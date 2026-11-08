package broker

import (
	"reflect"
	"testing"
)

func TestAdmissionSnapshotObservational(t *testing.T) {
	a, o := fixtureAdmission(t)
	ticket, err := a.AdmitHostWork(PersistenceWork)
	mustOK(t, err)
	mustOK(t, ticket.PersistenceFailed())
	before := a.Snapshot(o)
	if !before.Retained || !before.Open || before.UnresolvedHostWork != 1 || before.HostWork[PersistenceWork] != 1 {
		t.Fatalf("snapshot: %+v", before)
	}
	for i := 0; i < 10; i++ {
		if !reflect.DeepEqual(before, a.Snapshot(o)) {
			t.Fatal("snapshot changed accounting")
		}
	}
	bad := a.Snapshot(Owner{o.Conversation, "other", o.Epoch})
	if bad.Retained || bad.Attachments != 0 || len(bad.Work) != 0 || bad.UnresolvedWork != 0 || bad.HostRetainedSessions != 1 {
		t.Fatalf("mismatch: %+v", bad)
	}
	closure, err := a.CloseOwner(o, nil)
	mustOK(t, err)
	if a.Snapshot(o).Open {
		t.Fatal("closed owner reported open")
	}
	mustOK(t, closure.Relinquished())
	mustOK(t, ticket.ResolvePersistence())
	mustOK(t, ticket.Complete())
	mustOK(t, a.Retire())
	if got := a.Snapshot(o); !got.Retired || got.Retained || got.HostRetainedSessions != 0 || len(got.HostWork) != 0 {
		t.Fatalf("retired: %+v", got)
	}
}
