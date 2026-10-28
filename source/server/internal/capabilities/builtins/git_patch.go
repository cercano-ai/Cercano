package builtins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"cercano/source/server/internal/capabilities"
	"cercano/source/server/internal/gitflow"
)

// decodeReadQuery rejects unadvertised options rather than silently ignoring them.
func decodeReadQuery(raw []byte, dst any) error {
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	if len(raw) > 65536 {
		return fmt.Errorf("query too large")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("expected one query object")
	}
	return nil
}

type gitPatchCap struct{}

func GitDiff() capabilities.Capability      { return gitPatchCap{} }
func (gitPatchCap) Name() string            { return "git_diff" }
func (gitPatchCap) Tier() capabilities.Tier { return capabilities.TierR }
func (gitPatchCap) Surfaces() capabilities.Surface {
	return capabilities.SurfaceAgent | capabilities.SurfaceMCP
}
func (gitPatchCap) Description() string {
	return "Read a bounded patch of unstaged tracked changes, or staged changes with staged=true. Literal relative file filters only; no shell, revision options, external diff or textconv. Output may be truncated; untracked files are not included."
}
func (gitPatchCap) Schema() capabilities.Schema {
	return capabilities.Schema(`{"type":"object","properties":{"path":{"type":"string"},"paths":{"type":"array","maxItems":100,"items":{"type":"string"}},"staged":{"type":"boolean"},"max_bytes":{"type":"integer","minimum":1,"maximum":65536}},"additionalProperties":false}`)
}
func (gitPatchCap) Execute(ctx context.Context, call *capabilities.Call) (*capabilities.Result, error) {
	a := struct {
		Path     string   `json:"path"`
		Paths    []string `json:"paths"`
		Staged   bool     `json:"staged"`
		MaxBytes int      `json:"max_bytes"`
	}{MaxBytes: 32768}
	if err := decodeReadQuery(call.Args, &a); err != nil {
		return nil, err
	}
	dir := a.Path
	if dir == "" {
		dir = call.WorkDir
	} else if !filepath.IsAbs(dir) {
		dir = filepath.Join(call.WorkDir, dir)
	}
	text, truncated, err := (&gitflow.Repo{Dir: dir}).Patch(ctx, a.Paths, a.Staged, a.MaxBytes)
	if err != nil {
		return nil, err
	}
	if truncated {
		text += "\n[patch truncated]"
	}
	return &capabilities.Result{Text: text}, nil
}
