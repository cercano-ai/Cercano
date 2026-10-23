package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"cercano/source/server/internal/secrets"
	"cercano/source/server/internal/updatecoord/enrollment"
	"cercano/source/server/internal/updatecoord/exclusion"
	"cercano/source/server/internal/updatecoord/nativeprocess"
	"cercano/source/server/internal/updatecoord/privdir"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/installcontext"
)

const maxInput = 64 * 1024
const maxParticipants = 64

var errInput = errors.New("updater: invalid controlled operation input")
var errIncomplete = errors.New("updater: incomplete; event acknowledgement, owned workers/runtime evidence and publication unavailable; selection unchanged")

type participant struct {
	Role     string                 `json:"role"`
	Identity nativeprocess.Identity `json:"identity"`
}
type operationInput struct {
	InstallRoot    string        `json:"install_root"`
	InstallID      string        `json:"install_id"`
	StateRoot      string        `json:"state_root"`
	OperationToken string        `json:"operation_token"`
	AgentEndpoint  string        `json:"agent_endpoint"`
	Participants   []participant `json:"participants"`
	// Only this fixed sibling name is accepted; never an arbitrary output path.
	StatusFile string `json:"status_file"`
}

// exactJSON follows only the fixed Go schema: every field is required, exact
// case, non-null, unique, and known. It bounds arrays before unmarshalling.
func exactJSON(d *json.Decoder, t reflect.Type) error {
	switch t.Kind() {
	case reflect.Struct:
		tok, err := d.Token()
		if err != nil || tok != json.Delim('{') {
			return errInput
		}
		fields := map[string]reflect.Type{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name := f.Tag.Get("json")
			if name == "" {
				name = f.Name
			}
			fields[name] = f.Type
		}
		seen := map[string]bool{}
		for d.More() {
			tok, err = d.Token()
			if err != nil {
				return errInput
			}
			key, ok := tok.(string)
			ft, known := fields[key]
			if !ok || !known || seen[key] {
				return errInput
			}
			seen[key] = true
			if err = exactJSON(d, ft); err != nil {
				return err
			}
		}
		if len(seen) != len(fields) {
			return errInput
		}
		tok, err = d.Token()
		if err != nil || tok != json.Delim('}') {
			return errInput
		}
	case reflect.Slice:
		tok, err := d.Token()
		if err != nil || tok != json.Delim('[') {
			return errInput
		}
		n := 0
		for d.More() {
			n++
			if n > maxParticipants {
				return errInput
			}
			if err = exactJSON(d, t.Elem()); err != nil {
				return err
			}
		}
		tok, err = d.Token()
		if err != nil || tok != json.Delim(']') {
			return errInput
		}
	default:
		var raw json.RawMessage
		if d.Decode(&raw) != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return errInput
		}
		if json.Unmarshal(raw, reflect.New(t).Interface()) != nil {
			return errInput
		}
	}
	return nil
}
func parseInput(data []byte) (operationInput, error) {
	var in operationInput
	if len(data) > maxInput || !utf8.Valid(data) {
		return in, errInput
	}
	d := json.NewDecoder(bytes.NewReader(data))
	if err := exactJSON(d, reflect.TypeOf(in)); err != nil {
		return in, err
	}
	if _, err := d.Token(); err != io.EOF {
		return in, errInput
	}
	if err := json.Unmarshal(data, &in); err != nil {
		return in, errInput
	}
	loc := installcontext.Locators{InstallRoot: in.InstallRoot, InstallID: in.InstallID}
	if loc.Validate() != nil || !filepath.IsAbs(in.StateRoot) || filepath.Clean(in.StateRoot) != in.StateRoot || len(in.OperationToken) != 32 || strings.Trim(in.OperationToken, "0123456789abcdef") != "" || in.StatusFile != "status.json" || len(in.Participants) == 0 {
		return in, errInput
	}
	ep, err := netip.ParseAddrPort(in.AgentEndpoint)
	if err != nil || !ep.Addr().IsLoopback() || ep.Port() == 0 || ep.String() != in.AgentEndpoint {
		return in, errInput
	}
	seen := map[int]bool{}
	agents := 0
	for _, p := range in.Participants {
		id := p.Identity
		if p.Role == "agent" {
			agents++
		} else if p.Role != "client" {
			return in, errInput
		}
		if id.PID <= 0 || seen[id.PID] || id.UID != uint32(os.Getuid()) || !filepath.IsAbs(id.Executable) || filepath.Clean(id.Executable) != id.Executable || id.StartSeconds == 0 && id.StartTicks == 0 && id.WindowsStartFileTime == 0 {
			return in, errInput
		}
		seen[id.PID] = true
	}
	if agents != 1 {
		return in, errInput
	}
	return in, nil
}

// operationFiles keeps all I/O anchored to the originally owned directory.
// Path checks reject rebinding; relative I/O prevents a race from writing into
// a replacement even if it occurs immediately after those checks.
type operationFiles struct {
	root       *os.Root
	parent     string
	parentInfo os.FileInfo
	inputInfo  os.FileInfo
	data       []byte
}

func (o *operationFiles) validate() error {
	canonical, err := filepath.EvalSymlinks(o.parent)
	if err != nil || canonical != o.parent {
		return errInput
	}
	if err := privdir.VerifyExisting(o.parent); err != nil {
		return err
	}
	info, err := os.Lstat(o.parent)
	if err != nil || !os.SameFile(info, o.parentInfo) {
		return errInput
	}
	f, err := privdir.OpenPrivateStateFile(filepath.Join(o.parent, "operation.json"))
	if err != nil {
		return err
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil || !sameInputState(info, o.inputInfo) {
		return errInput
	}
	f2, err := o.root.OpenFile("operation.json", protectedReadFlags, 0)
	if err != nil {
		return err
	}
	defer f2.Close()
	info, err = f2.Stat()
	if err != nil || !sameInputState(info, o.inputInfo) {
		return errInput
	}
	data, err := io.ReadAll(io.LimitReader(f2, maxInput+1))
	if err != nil || !bytes.Equal(data, o.data) {
		return errInput
	}
	return nil
}
func sameInputState(a, b os.FileInfo) bool {
	return os.SameFile(a, b) && a.Mode() == b.Mode() && a.Size() == b.Size() && a.ModTime() == b.ModTime()
}
func openInput(path string) (operationInput, *operationFiles, error) {
	var zero operationInput
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || filepath.Base(path) != "operation.json" {
		return zero, nil, errInput
	}
	parent := filepath.Dir(path)
	canonical, err := filepath.EvalSymlinks(parent)
	if err != nil || canonical != parent {
		return zero, nil, errInput
	}
	if err := privdir.VerifyExisting(parent); err != nil {
		return zero, nil, err
	}
	info, err := os.Lstat(parent)
	if err != nil {
		return zero, nil, err
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		return zero, nil, err
	}
	ok := false
	defer func() {
		if !ok {
			root.Close()
		}
	}()
	anchored, err := root.Stat(".")
	if err != nil || !os.SameFile(info, anchored) {
		return zero, nil, errInput
	}
	protected, err := privdir.OpenPrivateStateFile(path)
	if err != nil {
		return zero, nil, err
	}
	defer protected.Close()
	inputInfo, err := protected.Stat()
	if err != nil {
		return zero, nil, err
	}
	f, err := root.OpenFile("operation.json", protectedReadFlags, 0)
	if err != nil {
		return zero, nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !sameInputState(inputInfo, opened) {
		return zero, nil, errInput
	}
	data, err := io.ReadAll(io.LimitReader(f, maxInput+1))
	if err != nil {
		return zero, nil, err
	}
	o := &operationFiles{root, parent, info, inputInfo, data}
	if err := o.validate(); err != nil {
		return zero, nil, err
	}
	in, err := parseInput(data)
	if err != nil {
		return zero, nil, err
	}
	ok = true
	return in, o, nil
}
func readInput(path string) (operationInput, error) {
	in, o, err := openInput(path)
	if o != nil {
		o.root.Close()
	}
	return in, err
}
func (o *operationFiles) createStatus() (*os.File, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}
	return o.root.OpenFile("status.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
}

func execute(ctx context.Context, path string) error {
	return executeBeforeStatus(ctx, path, nil)
}

func executeBeforeStatus(ctx context.Context, path string, beforeStatus func()) error {
	return executeAtSeams(ctx, path, beforeStatus, nil)
}

// Explicit per-call seams keep inert race tests deterministic without globals.
func executeAtSeams(ctx context.Context, path string, beforeStatus, beforeDrain func()) error {
	if err := secrets.ValidateEnvironment(); err != nil {
		return err
	}
	in, files, err := openInput(path)
	if err != nil {
		return err
	}
	defer files.root.Close()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	dir, err := installcontext.BootDirectory(in.StateRoot, in.InstallID)
	if err != nil {
		return err
	}
	lockCtx, stop := context.WithTimeout(ctx, 250*time.Millisecond)
	barrier, err := exclusion.Acquire(lockCtx, dir, exclusion.Update)
	stop()
	if err != nil {
		return err
	}
	defer barrier.Close()
	return installcontext.WithUpdateSelection(ctx, installcontext.Locators{InstallRoot: in.InstallRoot, InstallID: in.InstallID}, in.StateRoot, barrier, func(_ enrollment.Record, agentPath, clientPath string) error {
		// Retain EVERY independent observation before any drain request. Completeness
		// is the controlled harness's assertion, never inferred from subscribers.
		evidence := make([]nativeprocess.Identity, 0, len(in.Participants))
		var agent nativeprocess.Identity
		for _, p := range in.Participants {
			want := clientPath
			if p.Role == "agent" {
				want = agentPath
			}
			if p.Identity.Executable != want {
				return fmt.Errorf("participant role/image mismatch")
			}
			observed, err := nativeprocess.Inspect(p.Identity.PID)
			if err != nil {
				return err
			}
			if !observed.SameProcess(p.Identity) {
				return fmt.Errorf("participant lifetime changed")
			}
			evidence = append(evidence, observed)
			if p.Role == "agent" {
				agent = observed
			}
		}
		// Keep the complete independently observed set until ownership ends,
		// not merely until its count has been written to the status snapshot.
		defer runtime.KeepAlive(evidence)
		endpoint, _ := netip.ParseAddrPort(in.AgentEndpoint)
		held, err := nativeprocess.HoldsListener(agent, endpoint)
		if err != nil {
			return err
		}
		if !held {
			return fmt.Errorf("agent does not hold endpoint")
		}
		conn, err := agentclient.DialExistingPinned(ctx, in.AgentEndpoint)
		if err != nil {
			return err
		}
		defer conn.Close()
		held, err = nativeprocess.HoldsListener(agent, endpoint)
		if err != nil {
			return err
		}
		if !held {
			return fmt.Errorf("agent endpoint changed")
		}
		if beforeDrain != nil {
			beforeDrain()
		}
		drain, err := conn.OpenUpdateDrain(ctx, in.OperationToken)
		if err != nil {
			return err
		}
		defer drain.Close()
		if _, err = drain.Recv(); err != nil {
			return err
		} // validated SEALED, not TUI acknowledgement
		if beforeStatus != nil {
			beforeStatus()
		}
		status, err := files.createStatus()
		if err != nil {
			return err
		}
		payload := struct {
			OperationToken string `json:"operation_token"`
			State          string `json:"state"`
			Participants   int    `json:"participants"`
		}{in.OperationToken, "owner_evidence_sealed_incomplete", len(evidence)}
		err = json.NewEncoder(status).Encode(payload)
		closeErr := status.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		// Consume DRAINING and IDLE, then wait for cancellation/deadline or owner
		// stream failure. No shutdown or exit inference from a stream is permitted.
		for i := 0; i < 2; i++ {
			if _, err = drain.Recv(); err != nil {
				return fmt.Errorf("%w: drain: %v", errIncomplete, err)
			}
		}
		_, err = drain.Recv()
		return fmt.Errorf("%w: owner stream ended: %v", errIncomplete, err)
	})
}
