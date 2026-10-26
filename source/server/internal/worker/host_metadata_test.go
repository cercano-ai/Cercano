package worker

import (
	"context"
	"reflect"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"
)

func TestInitialProfileMetadataReplacesRetainedFence(t *testing.T) {
	const key = "cercano-initial-profile"
	const authoritative = `{"Name":"plan","AllowedTiers":{"R":true}}`
	type ownerKey struct{}
	owner := new(int) // opaque exact-owner context authority must retain identity
	for _, inherited := range [][]string{nil, {`{}`}, {`null`}, {`malformed`}, {`{"Name":"old"}`, `{"Name":"old"}`}} {
		t.Run("inherited="+stringJoin(inherited), func(t *testing.T) {
			base, cancel := context.WithTimeout(context.WithValue(context.Background(), ownerKey{}, owner), time.Minute)
			defer cancel()
			original := metadata.Pairs("unrelated", "keep")
			original.Set(key, inherited...)
			parent := metadata.NewOutgoingContext(base, original)
			// Exercise appended metadata as well as the original outgoing map.
			parent = metadata.AppendToOutgoingContext(parent, "unrelated", "also-keep")
			before, _ := metadata.FromOutgoingContext(parent)
			got := withInitialProfileMetadata(parent, authoritative)
			got = withInitialProfileMetadata(got, authoritative) // automatic completion/reuse
			md, _ := metadata.FromOutgoingContext(got)
			if !reflect.DeepEqual(md.Get(key), []string{authoritative}) {
				t.Fatalf("profile = %q", md.Get(key))
			}
			if !reflect.DeepEqual(md.Get("unrelated"), []string{"keep", "also-keep"}) {
				t.Fatalf("unrelated metadata lost: %v", md)
			}
			after, _ := metadata.FromOutgoingContext(parent)
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("mutated retained context: %v -> %v", before, after)
			}
			if got.Value(ownerKey{}) != owner {
				t.Fatal("lost exact owner")
			}
			deadline, _ := parent.Deadline()
			if d, ok := got.Deadline(); !ok || d != deadline {
				t.Fatal("lost deadline")
			}
			cancel()
			if got.Err() != context.Canceled {
				t.Fatal("lost cancellation")
			}
		})
	}
}

func stringJoin(values []string) string {
	if len(values) == 0 {
		return "absent"
	}
	return values[0]
}
