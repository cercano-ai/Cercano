package config

// ConfigCoordinator is the shared publication boundary for versioned saves.
// It does not apply environment overrides or activate runtime state. Advisory
// locking protects participating writers, not external editors or old binaries.
import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"

	"cercano/source/server/internal/filelock"
	"cercano/source/server/internal/secrets"
	"gopkg.in/yaml.v3"
)

var ErrConfigConflict = errors.New("configuration changed since base snapshot; reload and reconcile before saving")

// ConfigRevision is a hash of the exact persisted bytes, not effective defaults
// or environment overrides. It contains no config values.
type ConfigRevision [sha256.Size]byte

// ConfigSnapshot is an opaque disk base. It deliberately cannot be installed as
// a live Config: callers must separately reload and reconcile after publication.
type ConfigSnapshot struct {
	Revision  ConfigRevision
	data      []byte
	authority string
	absent    bool
}

// ConfigChange edits exactly one stored YAML mapping path. YAML is a single
// replacement value; Delete removes the key (restoring ordinary Load defaults).
// Sequences are replaced as a whole. No environment-derived values are persisted
// implicitly. Unknown/unrelated keys and explicit zero/null presence survive.
type ConfigChange struct {
	Path   []string
	YAML   []byte
	Delete bool
}

// ConfigCoordinator coordinates an existing or genuinely absent authority. Symlinks resolve to the
// same target-side stable lock. Hardlinks are refused: rename would split their
// authority. The parent directory must be trusted, as with permission stores.
type ConfigCoordinator struct {
	path    string
	publish func(string, []byte, os.FileMode) error
}

// PrepareConfigCoordination preserves the snapshot's immutable write policy
// without treating its effective values as the persisted base.
func (c Config) PrepareConfigCoordination(path string) (*ConfigCoordinator, error) {
	if c.sharedReadOnly {
		return nil, secrets.ErrReadOnly
	}
	return NewConfigCoordinator(path)
}

func NewConfigCoordinator(path string) (*ConfigCoordinator, error) {
	if path == "" {
		return nil, errors.New("configuration authority required")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	path, err = configAuthority(path)
	if err != nil {
		return nil, err
	}
	return &ConfigCoordinator{path: path, publish: publishConfig}, nil
}

// configAuthority resolves existing symlinks but permits genuinely absent leaves.
// A dangling symlink is not absence and must never authorize creation.
func configAuthority(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	if _, e := os.Lstat(path); !os.IsNotExist(e) {
		return "", err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return "", err
	}
	parent, err = configAuthority(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(path)), nil
}

func (s *ConfigCoordinator) locked(ctx context.Context, fn func([]byte, os.FileMode) error) error {
	// Refuse a redirected/replaced authority rather than locking an alias inode.
	resolved, err := configAuthority(s.path)
	if err != nil {
		return err
	}
	if resolved != s.path {
		return errors.New("configuration authority redirected")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	release, err := filelock.Lock(ctx, s.path+".lock")
	if err != nil {
		return err
	}
	defer release()
	info, err := os.Lstat(s.path)
	if os.IsNotExist(err) {
		return fn(nil, 0644)
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("configuration authority is not a regular file")
	}
	// Nlink is supplied by Unix Stat_t. Platforms lacking link-count evidence
	// fail closed instead of silently splitting a potentially shared authority.
	stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
	if !stat.IsValid() || stat.Kind() != reflect.Struct {
		return errors.New("cannot verify configuration link count")
	}
	links := stat.FieldByName("Nlink")
	if !links.IsValid() || !links.CanUint() || links.Uint() != 1 {
		return errors.New("configuration authority must have one link")
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	if _, err := configDocument(data); err != nil {
		return err
	}
	return fn(data, info.Mode().Perm())
}

func (s *ConfigCoordinator) snapshot(data []byte) ConfigSnapshot {
	return ConfigSnapshot{Revision: sha256.Sum256(data), data: bytes.Clone(data), authority: s.path, absent: data == nil}
}

func (s *ConfigCoordinator) Read(ctx context.Context) (base ConfigSnapshot, err error) {
	err = s.locked(ctx, func(data []byte, _ os.FileMode) error { base = s.snapshot(data); return nil })
	return
}

func (s *ConfigCoordinator) checkBase(base ConfigSnapshot) error {
	if base.authority != s.path || sha256.Sum256(base.data) != base.Revision {
		return ErrConfigConflict
	}
	return nil
}

// Save refuses ANY stale full snapshot. Serialization matches legacy Save;
// readonly/credential admission is preserved. It never installs runtime state.
func (s *ConfigCoordinator) Save(ctx context.Context, base ConfigSnapshot, cfg Config) (next ConfigSnapshot, err error) {
	if cfg.sharedReadOnly {
		return next, secrets.ErrReadOnly
	}
	if cfg.CloudAPIKey != "" {
		if err = secrets.CheckAllowed(); err != nil {
			return
		}
	}
	if err = cfg.LlamaServer.Validate(); err != nil {
		return
	}
	if err = s.checkBase(base); err != nil {
		return
	}
	cfg = cfg.Clone()
	if len(cfg.CloudProfiles) > 0 {
		cfg.CloudProvider = ""
		cfg.CloudModel = ""
		cfg.CloudAPIKey = ""
		cfg.CloudBaseURL = ""
	}
	stripBakedCloudCatalogForSave(&cfg)
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return next, err
	}
	if _, err := configDocument(data); err != nil {
		return next, err
	}
	err = s.locked(ctx, func(current []byte, mode os.FileMode) error {
		if (current == nil) != base.absent || sha256.Sum256(current) != base.Revision {
			return ErrConfigConflict
		}
		if err := s.publish(s.path, data, mode); err != nil {
			return err
		}
		next = s.snapshot(data)
		return nil
	})
	return
}

// Change merges only declared paths into a fresh locked read. Each touched
// value must still match the base (including absence); unrelated stale edits
// commute. Overlapping paths in one request are rejected. It is a disk-only
// operation, not a credential write or runtime activation API.
func (s *ConfigCoordinator) Change(ctx context.Context, base ConfigSnapshot, changes ...ConfigChange) (next ConfigSnapshot, err error) {
	if err = s.checkBase(base); err != nil {
		return
	}
	if base.absent {
		return next, ErrConfigConflict
	}
	original, err := configDocument(base.data)
	if err != nil {
		return next, err
	}
	for i, c := range changes {
		if len(c.Path) == 0 {
			return next, errors.New("empty configuration field path")
		}
		// Credential admission is not bypassed through raw field edits.
		for _, key := range c.Path {
			if key == "cloud_api_key" || key == "api_key" {
				if err := secrets.CheckAllowed(); err != nil {
					return next, err
				}
			}
		}
		for _, d := range changes[:i] {
			n := len(c.Path)
			if len(d.Path) < n {
				n = len(d.Path)
			}
			if reflect.DeepEqual(c.Path[:n], d.Path[:n]) {
				return next, errors.New("overlapping configuration changes")
			}
		}
	}
	err = s.locked(ctx, func(current []byte, mode os.FileMode) error {
		if current == nil {
			return ErrConfigConflict
		}
		doc, err := configDocument(current)
		if err != nil {
			return err
		}
		currentDoc, err := configDocument(current)
		if err != nil {
			return err
		}
		baseCandidate, err := configDocument(base.data)
		if err != nil {
			return err
		}
		for _, c := range changes {
			old, err := fieldNode(original.Content[0], c.Path)
			if err != nil {
				return err
			}
			now, err := fieldNode(currentDoc.Content[0], c.Path)
			if err != nil {
				return err
			}
			if !equalConfigNode(old, now) {
				return ErrConfigConflict
			}
			var value *yaml.Node
			if !c.Delete {
				var v, extra yaml.Node
				decoder := yaml.NewDecoder(bytes.NewReader(c.YAML))
				if err := decoder.Decode(&v); err != nil || len(v.Content) != 1 {
					return errors.New("invalid configuration replacement")
				}
				if err := decoder.Decode(&extra); err != io.EOF {
					return errors.New("multiple configuration replacement documents")
				}
				value = v.Content[0]
			}
			// Decode replacements separately: the two graphs must not share nodes.
			var baseValue *yaml.Node
			if !c.Delete {
				var v yaml.Node
				if err := yaml.Unmarshal(c.YAML, &v); err != nil {
					return err
				}
				baseValue = v.Content[0]
			}
			if err := setConfigNode(baseCandidate.Content[0], c.Path, baseValue); err != nil {
				return err
			}
			if err := setConfigNode(doc.Content[0], c.Path, value); err != nil {
				return err
			}
		}
		data, err := yaml.Marshal(doc)
		if err != nil {
			return err
		}
		validated, err := configDocument(data)
		if err != nil {
			return err
		}
		baseData, err := yaml.Marshal(baseCandidate)
		if err != nil {
			return err
		}
		baseResult, err := configDocument(baseData)
		if err != nil {
			return err
		}
		if err := configEffectConflict(original, currentDoc, baseResult, validated); err != nil {
			return err
		}
		var candidate Config
		if err := validated.Decode(&candidate); err != nil {
			return errors.New("invalid configuration fields")
		}
		if candidate.CloudAPIKey != "" {
			if err := secrets.CheckAllowed(); err != nil {
				return err
			}
		}
		if err := s.publish(s.path, data, mode); err != nil {
			return err
		}
		next = s.snapshot(data)
		return nil
	})
	return
}

func configDocument(data []byte) (*yaml.Node, error) {
	var doc yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&doc); err != nil || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("invalid configuration document")
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("multiple configuration documents")
	}
	// Decode all fields, including unknown ones, to guard cyclic aliases and
	// excessive expansion before any traversal or publication.
	var semantic any
	if err := doc.Decode(&semantic); err != nil {
		return nil, errors.New("invalid configuration YAML values")
	}
	c := Defaults()
	if err := doc.Decode(&c); err != nil {
		return nil, errors.New("invalid configuration fields")
	}
	if c.LlamaServer.Validate() != nil || !ValidateToolLoopMaxIterations(c.ToolLoop.MaxIterations) {
		return nil, errors.New("invalid configuration overrides")
	}
	return &doc, nil
}

func fieldNode(node *yaml.Node, path []string) (*yaml.Node, error) {
	if len(path) == 0 {
		return node, nil
	}
	if node == nil {
		return nil, nil
	}
	var err error
	node, err = resolveConfigAlias(node)
	if err != nil {
		return nil, err
	}
	if node.Kind != yaml.MappingNode {
		return nil, errors.New("configuration path crosses non-mapping value")
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == "<<" {
			return nil, errors.New("configuration merge paths require full snapshot save")
		}
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == path[0] {
			return fieldNode(node.Content[i+1], path[1:])
		}
	}
	return nil, nil
}
func equalConfigNode(a, b *yaml.Node) bool {
	if a == nil || b == nil {
		return a == b
	}
	var x, y any
	if a.Decode(&x) != nil || b.Decode(&y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

func resolveConfigAlias(node *yaml.Node) (*yaml.Node, error) {
	seen := map[*yaml.Node]bool{}
	for node != nil && node.Kind == yaml.AliasNode {
		if seen[node] {
			return nil, errors.New("cyclic configuration alias")
		}
		seen[node] = true
		node = node.Alias
	}
	if node == nil {
		return nil, errors.New("invalid configuration alias")
	}
	return node, nil
}

// Include effects in BOTH graphs: a concurrent edit may have detached an alias
// from its anchor, hiding the downstream write in the current graph alone.
func configEffectConflict(base, current, baseResult, result *yaml.Node) error {
	values := make([]any, 4)
	for i, n := range []*yaml.Node{base, current, baseResult, result} {
		if err := n.Decode(&values[i]); err != nil {
			return errors.New("invalid configuration YAML values")
		}
	}
	var conflict func(any, any, any, any) bool
	conflict = func(b, c, br, r any) bool {
		if reflect.DeepEqual(b, c) {
			return false
		}
		bm, bok := b.(map[string]any)
		cm, cok := c.(map[string]any)
		brm, brok := br.(map[string]any)
		rm, rok := r.(map[string]any)
		if bok && cok && brok && rok {
			keys := map[string]bool{}
			for _, m := range []map[string]any{bm, cm, brm, rm} {
				for k := range m {
					keys[k] = true
				}
			}
			for k := range keys {
				bv, bp := bm[k]
				cv, cp := cm[k]
				brv, brp := brm[k]
				rv, rp := rm[k]
				if bp != brp || cp != rp || bp != cp {
					changed := bp != brp || cp != rp || !reflect.DeepEqual(bv, brv) || !reflect.DeepEqual(cv, rv)
					if changed && (bp != cp || !reflect.DeepEqual(bv, cv)) {
						return true
					}
				} else if conflict(bv, cv, brv, rv) {
					return true
				}
			}
			return false
		}
		return !reflect.DeepEqual(b, br) || !reflect.DeepEqual(c, r)
	}
	if conflict(values[0], values[1], values[2], values[3]) {
		return ErrConfigConflict
	}
	return nil
}

func setConfigNode(node *yaml.Node, path []string, value *yaml.Node) error {
	var err error
	node, err = resolveConfigAlias(node)
	if err != nil {
		return err
	}
	if node.Kind != yaml.MappingNode {
		return errors.New("configuration path crosses non-mapping value")
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == path[0] {
			if len(path) > 1 {
				return setConfigNode(node.Content[i+1], path[1:], value)
			}
			if value == nil {
				node.Content = append(node.Content[:i], node.Content[i+2:]...)
			} else {
				node.Content[i+1] = value
			}
			return nil
		}
	}
	if value == nil {
		return nil
	}
	child := value
	if len(path) > 1 {
		child = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		if err := setConfigNode(child, path[1:], value); err != nil {
			return err
		}
	}
	node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: path[0]}, child)
	return nil
}

func publishConfig(dst string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(dst), ".config-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Chmod(mode); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), dst); err != nil {
		return fmt.Errorf("publish configuration: %w", err)
	}
	return nil
}
