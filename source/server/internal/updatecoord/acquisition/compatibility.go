package acquisition

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// CompatibilityContract is trusted caller policy, never policy learned from a
// release. ID must identify a reviewed, complete prior-reader/candidate-write
// test matrix covering ALL persistent state in database, config and credentials.
// The formats identify the caller's actual prior state/readers. No production
// IDs or supported formats are defined here. The caller must also trust its TUF
// release signer to enforce the artifact-bound release-test gate; this client
// authenticates that signer's declaration, not independent test evidence.
type CompatibilityContract struct {
	ID                string
	DatabaseFormat    string
	ConfigFormat      string
	CredentialsFormat string
}

func (c CompatibilityContract) validate() error {
	for _, s := range []string{c.ID, c.DatabaseFormat, c.ConfigFormat, c.CredentialsFormat} {
		if s == "" || len(s) > 256 {
			return errors.New("compatibility: explicit contract and all prior formats required (1..256 bytes)")
		}
	}
	return nil
}

// exactObject rejects missing, unknown, duplicate (including escaped aliases),
// non-object and trailing JSON. Unlike struct decoding it is case-sensitive.
func exactObject(raw []byte, keys ...string) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	tok, err := d.Token()
	if err != nil || tok != json.Delim('{') {
		return nil, errors.New("compatibility: expected object")
	}
	result := make(map[string]json.RawMessage)
	for d.More() {
		tok, err = d.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, errors.New("compatibility: invalid key")
		}
		allowed := false
		for _, k := range keys {
			if k == key {
				allowed = true
			}
		}
		if !allowed {
			return nil, fmt.Errorf("compatibility: unknown field %q", key)
		}
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("compatibility: duplicate field %q", key)
		}
		var value json.RawMessage
		if err = d.Decode(&value); err != nil {
			return nil, err
		}
		result[key] = value
	}
	if _, err = d.Token(); err != nil {
		return nil, err
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, errors.New("compatibility: trailing JSON")
	}
	if len(result) != len(keys) {
		return nil, errors.New("compatibility: missing field")
	}
	return result, nil
}

func validateCompatibility(raw []byte, c CompatibilityContract) error {
	if err := c.validate(); err != nil {
		return err
	}
	if len(raw) > 8192 {
		return errors.New("compatibility: declaration too large")
	}
	obj, err := exactObject(raw, "version", "contract", "domains")
	if err != nil {
		return err
	}
	if string(obj["version"]) != "1" {
		return errors.New("compatibility: unsupported version")
	}
	var id string
	if err = json.Unmarshal(obj["contract"], &id); err != nil || id != c.ID {
		return errors.New("compatibility: unsupported contract")
	}
	domains, err := exactObject(obj["domains"], "database", "config", "credentials")
	if err != nil {
		return err
	}
	for name, format := range map[string]string{"database": c.DatabaseFormat, "config": c.ConfigFormat, "credentials": c.CredentialsFormat} {
		domain, err := exactObject(domains[name], "prior_format", "prior_reads_candidate_writes")
		if err != nil {
			return fmt.Errorf("compatibility: %s: %w", name, err)
		}
		var prior string
		if err = json.Unmarshal(domain["prior_format"], &prior); err != nil || prior != format {
			return fmt.Errorf("compatibility: unsupported %s prior format", name)
		}
		if string(domain["prior_reads_candidate_writes"]) != "true" {
			return fmt.Errorf("compatibility: %s backward readability not declared", name)
		}
	}
	return nil
}
