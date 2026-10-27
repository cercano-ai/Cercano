package main

import (
	"encoding/json"
	"testing"
)

func TestDevHostCanonicalPublisherManifest(t *testing.T) {
	var m devHostManifest
	// Publisher manifests contain other provenance fields; the host consumes
	// exactly the build identity and its own executable hash.
	if err := json.Unmarshal([]byte(`{"build_id":"build-A","generation":"gen-A","binaries":{"cercano":{"sha256":"digest-A"},"cercano-cli":{"sha256":"digest-cli"}}}`), &m); err != nil {
		t.Fatal(err)
	}
	if m.Build != "build-A" || m.Binaries["cercano"].SHA256 != "digest-A" {
		t.Fatalf("canonical manifest not consumed: %+v", m)
	}
}

func TestDevHostLegacyFixtureManifestIsNotCanonical(t *testing.T) {
	var m devHostManifest
	if err := json.Unmarshal([]byte(`{"Build":"legacy","SHA256":"digest"}`), &m); err != nil {
		t.Fatal(err)
	}
	if m.Build != "" || m.Binaries["cercano"].SHA256 != "" {
		t.Fatalf("legacy fixture shape was accepted: %+v", m)
	}
}
