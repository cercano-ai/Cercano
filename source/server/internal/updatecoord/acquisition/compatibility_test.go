package acquisition

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

const syntheticDeclaration = `{"version":1,"contract":"fixture-matrix-v1","domains":{"database":{"prior_format":"fixture-db-v1","prior_reads_candidate_writes":true},"config":{"prior_format":"fixture-config-v1","prior_reads_candidate_writes":true},"credentials":{"prior_format":"fixture-credentials-v1","prior_reads_candidate_writes":true}}}`

func syntheticContract() *CompatibilityContract {
	return &CompatibilityContract{ID: "fixture-matrix-v1", DatabaseFormat: "fixture-db-v1", ConfigFormat: "fixture-config-v1", CredentialsFormat: "fixture-credentials-v1"}
}

func invalidDeclarations() map[string]string {
	cases := map[string]string{
		"unknown domain":   strings.Replace(syntheticDeclaration, `"domains":{`, `"domains":{"other":{},`, 1),
		"nested duplicate": strings.Replace(syntheticDeclaration, `"prior_reads_candidate_writes":true`, `"prior_reads_candidate_writes":true,"prior_reads_candidate_writes":true`, 1),
		"missing":          "", "null": "null", "array": "[]", "malformed": "{", "trailing": syntheticDeclaration + ` {}`,
		"unknown field":     strings.Replace(syntheticDeclaration, `"version":1`, `"extra":true,"version":1`, 1),
		"duplicate":         strings.Replace(syntheticDeclaration, `"version":1`, `"version":1,"version":1`, 1),
		"escaped duplicate": strings.Replace(syntheticDeclaration, `"version":1`, `"version":1,"\u0076ersion":1`, 1),
		"case alias":        strings.Replace(syntheticDeclaration, `"version"`, `"Version"`, 1),
		"version":           strings.Replace(syntheticDeclaration, `"version":1`, `"version":2`, 1),
		"version type":      strings.Replace(syntheticDeclaration, `"version":1`, `"version":"1"`, 1),
		"contract":          strings.Replace(syntheticDeclaration, "fixture-matrix-v1", "unknown", 1),
		"oversize":          strings.Repeat(" ", 8193),
	}
	for _, domain := range []string{"database", "config", "credentials"} {
		var obj map[string]json.RawMessage
		_ = json.Unmarshal([]byte(syntheticDeclaration), &obj)
		var domains map[string]json.RawMessage
		_ = json.Unmarshal(obj["domains"], &domains)
		original := string(domains[domain])
		variants := map[string]string{
			"null": "null", "unsupported": strings.Replace(original, "fixture-", "unknown-", 1),
			"false": strings.Replace(original, "true", "false", 1), "unknown": strings.Replace(original, "true", `"unknown"`, 1),
			"missing claim": `{"prior_format":"fixture-db-v1"}`, "extra": strings.Replace(original, `{`, `{"extra":1,`, 1),
		}
		for name, raw := range variants {
			domains[domain] = json.RawMessage(raw)
			obj["domains"], _ = json.Marshal(domains)
			b, _ := json.Marshal(obj)
			cases[domain+" "+name] = string(b)
		}
		delete(domains, domain)
		obj["domains"], _ = json.Marshal(domains)
		b, _ := json.Marshal(obj)
		cases[domain+" omitted"] = string(b)
	}
	return cases
}

func TestCompatibilityStrictParsing(t *testing.T) {
	if err := validateCompatibility([]byte(syntheticDeclaration), *syntheticContract()); err != nil {
		t.Fatal(err)
	}
	for name, raw := range invalidDeclarations() {
		t.Run(name, func(t *testing.T) {
			if err := validateCompatibility([]byte(raw), *syntheticContract()); err == nil {
				t.Fatal("qualified invalid declaration")
			}
		})
	}
	for _, c := range []CompatibilityContract{{}, {ID: "fixture-matrix-v1"}, {ID: "fixture-matrix-v1", DatabaseFormat: "fixture-db-v1", ConfigFormat: "fixture-config-v1"}} {
		if err := validateCompatibility([]byte(syntheticDeclaration), c); err == nil {
			t.Fatal("incomplete caller policy accepted")
		}
	}
}

func TestCompatibilitySignedTarget(t *testing.T) {
	cases := invalidDeclarations()
	// Invalid JSON cannot be serialized into signed metadata; parser tests above
	// cover it. Canonical signing may normalize duplicates: exercise those above.
	cases["valid"] = syntheticDeclaration
	for name, raw := range cases {
		if name != "missing" && (!json.Valid([]byte(raw)) || strings.Contains(name, "duplicate")) {
			continue
		}
		t.Run(name, func(t *testing.T) {
			var custom *json.RawMessage
			if name != "missing" {
				v := json.RawMessage(raw)
				custom = &v
			}
			fx := newFixtureSpecs(t, time.Now().Add(time.Hour), nil, targetSpec{path: "candidate.zip", data: []byte("synthetic candidate"), custom: custom})
			opts := fixtureOptions(fx, t.TempDir(), t.TempDir())
			opts.RequiredCompatibility = syntheticContract()
			receipt, err := acquire(context.Background(), opts, "candidate.zip", fx.server.Client())
			if name == "valid" {
				if err != nil || receipt == nil || !receipt.CompatibilityQualified {
					t.Fatalf("receipt=%+v err=%v", receipt, err)
				}
			} else {
				if err == nil || receipt != nil {
					t.Fatalf("qualified invalid signed declaration: %+v %v", receipt, err)
				}
				if fx.targetRequests != 0 {
					t.Fatal("fetched target before compatibility refusal")
				}
				entries, e := os.ReadDir(opts.OutputDir)
				if e != nil || len(entries) != 0 {
					t.Fatalf("output after refusal: %v %v", entries, e)
				}
			}
		})
	}
}

func TestCompatibilityBindingAndOptIn(t *testing.T) {
	for _, mode := range []string{"tampered bytes", "wrong target", "untrusted signer", "no policy"} {
		t.Run(mode, func(t *testing.T) {
			raw := json.RawMessage(syntheticDeclaration)

			fx := newFixtureSpecs(t, time.Now().Add(time.Hour), nil,
				targetSpec{path: "qualified.zip", data: []byte("synthetic candidate"), custom: &raw},
				targetSpec{path: "other.zip", data: []byte("synthetic candidate")})
			if mode == "tampered bytes" {
				fx.onTargetRequest = func(w http.ResponseWriter, _ *http.Request, _ string) bool {
					_, _ = w.Write([]byte("substituted payload"))
					return true
				}
			}
			opts := fixtureOptions(fx, t.TempDir(), t.TempDir())
			opts.RequiredCompatibility = syntheticContract()
			target := "qualified.zip"
			if mode == "wrong target" {
				target = "other.zip"
			}
			if mode == "untrusted signer" {
				opts.TrustedRoot = newFixture(t).root
			}
			if mode == "no policy" {
				opts.RequiredCompatibility = nil
			}
			r, err := acquire(context.Background(), opts, target, fx.server.Client())
			if mode == "no policy" {
				if err != nil || r == nil || r.CompatibilityQualified {
					t.Fatalf("acquisition alone: %+v %v", r, err)
				}
			} else if err == nil || r != nil {
				t.Fatalf("binding failed: %+v %v", r, err)
			}
		})
	}
}

func TestCompatibilityMalformedIdentifierAliases(t *testing.T) {
	for _, field := range []string{"contract", "database", "config", "credentials"} {
		for name, malformed := range map[string]string{"utf8": string([]byte{0xff}), "high surrogate": `\uD800`, "low surrogate": `\uDC00`} {
			t.Run(field+"/"+name, func(t *testing.T) {
				c := syntheticContract()
				dst := &c.ID
				switch field {
				case "database":
					dst = &c.DatabaseFormat
				case "config":
					dst = &c.ConfigFormat
				case "credentials":
					dst = &c.CredentialsFormat
				}
				raw := strings.Replace(syntheticDeclaration, *dst, malformed, 1)
				*dst = "�"
				if err := validateCompatibility([]byte(raw), *c); err == nil {
					t.Fatal("malformed declaration aliased caller U+FFFD")
				}
			})
		}
	}
}

func TestCompatibilityUnicodeIdentifiers(t *testing.T) {
	for _, value := range []string{"�", "日本語", "é", "e\u0301", "😀", `\uD800`} {
		c := CompatibilityContract{value, value, value, value}
		encoded, _ := json.Marshal(value)
		raw := syntheticDeclaration
		for _, old := range []string{"fixture-matrix-v1", "fixture-db-v1", "fixture-config-v1", "fixture-credentials-v1"} {
			raw = strings.Replace(raw, `"`+old+`"`, string(encoded), 1)
		}
		if err := validateCompatibility([]byte(raw), c); err != nil {
			t.Fatalf("%q: %v", value, err)
		}
		if value == "😀" {
			raw = strings.ReplaceAll(raw, "😀", `\uD83D\uDE00`)
			if err := validateCompatibility([]byte(raw), c); err != nil {
				t.Fatal(err)
			}
		}
	}
	for i := 0; i < 4; i++ {
		c := syntheticContract()
		fields := []*string{&c.ID, &c.DatabaseFormat, &c.ConfigFormat, &c.CredentialsFormat}
		*fields[i] = string([]byte{0xed, 0xa0, 0x80}) // UTF-8 encoding of a surrogate is invalid.
		if err := c.validate(); err == nil {
			t.Fatalf("invalid caller field %d accepted", i)
		}
	}
}

func TestCompatibilityPublicationFailures(t *testing.T) {
	for _, mode := range []string{"cancel", "close"} {
		t.Run(mode, func(t *testing.T) {
			raw := json.RawMessage(syntheticDeclaration)
			fx := newFixtureSpecs(t, time.Now().Add(time.Hour), nil, targetSpec{path: "candidate.zip", data: []byte("candidate"), custom: &raw})
			opts := fixtureOptions(fx, t.TempDir(), t.TempDir())
			opts.RequiredCompatibility = syntheticContract()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := fx.server.Client()
			if mode == "cancel" {
				client = &http.Client{Transport: &cancelingTransport{base: client.Transport, cancel: cancel}}
			}
			if mode == "close" {
				original := osClose
				osClose = func(*os.File) error { return os.ErrPermission }
				defer func() { osClose = original }()
			}
			receipt, err := acquire(ctx, opts, "candidate.zip", client)
			if err == nil || receipt != nil {
				t.Fatalf("receipt=%+v err=%v", receipt, err)
			}
			if mode == "close" && fx.targetRequests == 0 {
				t.Fatal("did not reach target download")
			}
			if names := outputEntries(t, opts.OutputDir); len(names) != 0 {
				t.Fatalf("output: %v", names)
			}
		})
	}
}
