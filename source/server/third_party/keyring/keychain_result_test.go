//go:build darwin && cgo

package keyring

import (
	"errors"
	"testing"
	"time"

	gokeychain "github.com/99designs/go-keychain"
)

// These tests exercise the exact result decoders used by Get/GetMetadata.
// They never construct a native query, open a keychain, or read credentials.
func TestKeychainFakeQueryResults(t *testing.T) {
	denied := errors.New("fake access denied")
	interaction := gokeychain.Error(-25308) // errSecInteractionNotAllowed
	modified := time.Unix(123, 0)
	found := []gokeychain.QueryResult{{Data: []byte("fake-data"), Label: "fake-label", Description: "fake-description", ModificationDate: modified}}
	for _, tc := range []struct {
		name    string
		results []gokeychain.QueryResult
		err     error
		want    error
	}{
		{"denied_nil", nil, denied, denied},
		{"denied_empty", []gokeychain.QueryResult{}, denied, denied},
		{"interaction_nil", nil, interaction, interaction},
		{"error_with_results", found, denied, denied},
		{"native_missing", nil, gokeychain.ErrorItemNotFound, ErrKeyNotFound},
		{"native_missing_with_results", found, gokeychain.ErrorItemNotFound, ErrKeyNotFound},
		{"empty_success", []gokeychain.QueryResult{}, nil, ErrKeyNotFound},
		{"nil_success", nil, nil, ErrKeyNotFound},
		{"found", found, nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item, err := keychainGetResult("fake-key", tc.results, tc.err)
			if err != tc.want {
				t.Fatalf("Get error = %v; want %v", err, tc.want)
			}
			md, err := keychainMetadataResult("fake-key", tc.results, tc.err)
			if err != tc.want {
				t.Fatalf("GetMetadata error = %v; want %v", err, tc.want)
			}
			if tc.want != nil {
				if item.Key != "" || item.Data != nil || md.Item != nil {
					t.Fatal("failed query exposed a result")
				}
				return
			}
			if item.Key != "fake-key" || string(item.Data) != "fake-data" || item.Label != "fake-label" || item.Description != "fake-description" {
				t.Fatal("Get result mapping changed")
			}
			if md.Item == nil || md.Item.Key != item.Key || md.Item.Label != item.Label || md.Item.Description != item.Description || md.Item.Data != nil || !md.ModificationTime.Equal(modified) {
				t.Fatal("metadata result mapping changed")
			}
		})
	}
}
