package ui

// Reopening is an explicit user action, never a side effect of publication or
// connection loss. The lifecycle service remains the authority for busy refusal.
import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"cercano/source/server/pkg/proto"
	tea "charm.land/bubbletea/v2"
	protobuf "google.golang.org/protobuf/proto"
)

func reopenDevHost(ctx context.Context, destination DevHostSelection, conversation string) (DevHostSelection, error) {
	dc, err := destination.controls(ctx)
	if err != nil {
		return destination, err
	}
	defer dc.Close()
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(destination.Identity.Store), "hosts", "*.json"))
	if err != nil {
		return destination, err
	}
	for _, path := range paths {
		st, err := os.Lstat(path)
		if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
			return destination, fmt.Errorf("cannot read private host record")
		}
		source, err := ReadDevHostSelection(path)
		if err != nil {
			return destination, err
		}
		if source.Identity.Store != destination.Identity.Store || source.Identity.Contract != destination.Identity.Contract || source.Identity.Schema != destination.Identity.Schema {
			return destination, fmt.Errorf("incompatible host record")
		}
		sc, err := source.controls(ctx)
		if err != nil {
			return destination, err
		}
		listing, err := sc.LifecycleList(ctx, &proto.LifecycleControlRequest{Target: source.Identity, Management: true})
		if err != nil {
			sc.Close()
			return destination, err
		}
		if !protobuf.Equal(listing.Identity, source.Identity) {
			sc.Close()
			return destination, fmt.Errorf("host identity mismatch")
		}
		for _, owner := range listing.Retained {
			if owner.Conversation != conversation {
				continue
			}
			if protobuf.Equal(source.Identity, destination.Identity) {
				sc.Close()
				destination.Conversation, destination.Epoch = conversation, owner.Epoch
				return destination, nil
			}
			var nonce [16]byte
			if _, err = rand.Read(nonce[:]); err != nil {
				sc.Close()
				return destination, err
			}
			result, err := sc.Transfer(ctx, dc, &proto.LifecycleControlRequest{Target: source.Identity, Source: source.Identity, Destination: destination.Identity, DestinationEndpoint: destination.Endpoint, Owner: owner, Management: true, HandoverId: hex.EncodeToString(nonce[:])})
			sc.Close()
			if err != nil || result.GetPending() || result.GetOwner() == nil {
				if result.GetReceipt() != "" {
					return destination, fmt.Errorf("reopen was not completed; handover needs explicit recovery; no retry scheduled: %v", err)
				}
				return destination, fmt.Errorf("conversation remains on its original host; outstanding work or attached clients must finish first. Work is preserved; no transfer is scheduled: %v", err)
			}
			destination.Conversation, destination.Epoch = conversation, result.Owner.Epoch
			return destination, nil
		}
		sc.Close()
	}
	result, err := dc.LifecycleAcquire(ctx, &proto.LifecycleControlRequest{Target: destination.Identity, Owner: &proto.LifecycleOwner{Conversation: conversation}})
	if err != nil {
		return destination, err
	}
	if result.GetOwner().GetEpoch() == "" {
		return destination, fmt.Errorf("reopen returned no ownership")
	}
	destination.Conversation, destination.Epoch = conversation, result.Owner.Epoch
	return destination, nil
}

func (m Model) beginDevHostReopen(conversation string) (Model, tea.Cmd) {
	if m.devHost.busy || m.devHost.detached {
		m.errMsg = "Reopen unavailable while disconnected or another reopen is running"
		return m, nil
	}
	m.devHost.busy = true
	selection := m.devHost.selection
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		s, err := reopenDevHost(ctx, selection, conversation)
		r := devHostResult{action: "reopen", err: err}
		if err == nil {
			r.selection = &s
			r.client, r.err = s.Dial(ctx)
		}
		return r
	}
}
