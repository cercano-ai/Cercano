package context

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	skillsconsumer "cercano/source/server/internal/skills/consumer"
)

const (
	// CercanoDir is the directory name inside a project root.
	CercanoDir = ".cercano"
	// ContextFile is the context file name inside CercanoDir.
	ContextFile = "context.md"
)

// ContextPath returns the full path to the context file for a project.
func ContextPath(projectDir string) string {
	return filepath.Join(projectDir, CercanoDir, ContextFile)
}

// Loader reads and caches project context from .cercano/context.md.
type Loader struct {
	mu      sync.RWMutex
	cache   map[string]string // projectDir -> context content
	nudged  map[string]bool   // projectDir -> already nudged this session

	// homeDirOverride is an injectable os.UserHomeDir stand-in for hermetic
	// tests of skill discovery (nil in production).
	homeDirOverride func() string
}

// NewLoader creates a new context Loader.
func NewLoader() *Loader {
	return &Loader{
		cache:  make(map[string]string),
		nudged: make(map[string]bool),
	}
}

// NewLoaderWithHomeDir is like NewLoader but injects the home-directory
// lookup used by skill discovery (normally os.UserHomeDir). Tests use it to
// stay hermetic; embedding hosts can use it to relocate skill roots.
func NewLoaderWithHomeDir(home func() string) *Loader {
	l := NewLoader()
	l.homeDirOverride = home
	return l
}

// homeDirFn injects the home-directory lookup used by skill discovery. It
// defaults to the production helper; tests override it to stay hermetic.
func (l *Loader) homeDirFn() string {
	if l.homeDirOverride != nil {
		return l.homeDirOverride()
	}
	return skillsconsumer.HomeDir()
}

// skillsSection discovers the skill catalog for projectDir and renders the
// prompt section for it. Returns "" when no valid skills are found (and
// then, by design, nothing is added to the prompt).
func (l *Loader) skillsSection(projectDir string) string {
	entries := skillsconsumer.Discover(projectDir, l.homeDirFn())
	return skillsconsumer.Catalog(entries)
}

// Load reads the context file for a project. Returns empty string if not found.
// Results are cached per project directory.
func (l *Loader) Load(projectDir string) (string, error) {
	if projectDir == "" {
		return "", nil
	}

	l.mu.RLock()
	if cached, ok := l.cache[projectDir]; ok {
		l.mu.RUnlock()
		return cached, nil
	}
	l.mu.RUnlock()

	path := ContextPath(projectDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			l.mu.Lock()
			l.cache[projectDir] = ""
			l.mu.Unlock()
			return "", nil
		}
		return "", fmt.Errorf("failed to read context file %q: %w", path, err)
	}

	content := string(data)
	l.mu.Lock()
	l.cache[projectDir] = content
	l.mu.Unlock()
	return content, nil
}

// IsInitialized returns true if .cercano/context.md exists for the project.
func (l *Loader) IsInitialized(projectDir string) bool {
	if projectDir == "" {
		return false
	}
	_, err := os.Stat(ContextPath(projectDir))
	return err == nil
}

// Invalidate clears the cached context for a project, forcing a re-read on next Load.
func (l *Loader) Invalidate(projectDir string) {
	l.mu.Lock()
	delete(l.cache, projectDir)
	l.mu.Unlock()
}

// PrependContext loads the project context and prepends it to the prompt.
// Returns the original prompt unchanged if no context is available.
func (l *Loader) PrependContext(projectDir, prompt string) string {
	if projectDir == "" {
		return prompt
	}
	// The skills catalog is re-discovered on every call: no global state, so
	// one workspace's skills never leak into another's requests. It is
	// deliberately checked before the context file — a project without
	// context.md still gets its skills, including user-home-only skills.
	skills := l.skillsSection(projectDir)

	ctx, _ := l.Load(projectDir)
	if ctx == "" && skills == "" {
		return prompt
	}
	if ctx == "" {
		return fmt.Sprintf("%s\n\n---\n\n%s", skills, prompt)
	}
	if skills == "" {
		return fmt.Sprintf("Project Context:\n%s\n\n---\n\n%s", ctx, prompt)
	}
	return fmt.Sprintf("Project Context:\n%s\n\n---\n\n%s\n\n---\n\n%s", ctx, skills, prompt)
}

// NudgeNeeded returns true if the project has no context.md and hasn't been
// nudged yet this session. Returns false for empty projectDir or initialized projects.
func (l *Loader) NudgeNeeded(projectDir string) bool {
	if projectDir == "" {
		return false
	}
	if l.IsInitialized(projectDir) {
		return false
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.nudged[projectDir] {
		return false
	}
	l.nudged[projectDir] = true
	return true
}
