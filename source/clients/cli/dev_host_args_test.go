package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMalformedDevHostArgs(t *testing.T) {
	for _, tc := range []struct {
		name, host, action, destination, handover, receipt string
		present                                            map[string]bool
	}{
		{name: "empty selector", present: map[string]bool{"dev-host": true}},
		{name: "orphan destination", destination: "/missing", present: map[string]bool{"dev-host-destination": true}},
		{name: "empty destination", present: map[string]bool{"dev-host-destination": true}},
		{name: "orphan handover", handover: "id", present: map[string]bool{"dev-host-handover": true}},
		{name: "orphan receipt", receipt: "receipt", present: map[string]bool{"dev-host-receipt": true}},
		{name: "unknown action", host: "/missing", action: "bogus", present: map[string]bool{"dev-host": true, "dev-host-action": true}},
		{name: "transfer missing destination", host: "/missing", action: "transfer", handover: "id", present: map[string]bool{"dev-host": true, "dev-host-action": true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateDevHostArgs(tc.present, tc.host, tc.action, tc.destination, tc.handover, tc.receipt); err == nil {
				t.Fatal("malformed arguments accepted before config/dial")
			}
		})
	}
}

func TestValidDevHostArgs(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "destination.json")
	if err := os.WriteFile(destination, []byte(`{"Identity":{"Host":"host","Build":"build","Contract":"contract","Schema":1,"Store":"store"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"", "list", "acquire", "release", "retire", "transfer", "claim"} {
		t.Run(action, func(t *testing.T) {
			present := map[string]bool{"dev-host": true}
			dest, handover, receipt := "", "", ""
			if action != "" {
				present["dev-host-action"] = true
			}
			if action == "transfer" || action == "claim" {
				dest, handover = destination, "unique-id"
				present["dev-host-destination"], present["dev-host-handover"] = true, true
			}
			if action == "claim" {
				receipt = "receipt"
				present["dev-host-receipt"] = true
			}
			if err := validateDevHostArgs(present, "/source.json", action, dest, handover, receipt); err != nil {
				t.Fatal(err)
			}
		})
	}
	if err := validateDevHostArgs(nil, "", "", "", "", ""); err != nil {
		t.Fatalf("non-dev lane: %v", err)
	}
}
