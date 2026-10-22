// Package candidate binds native lifetime observations to an exact owned launch.
// It grants no health, listener exclusivity, signaling, activation or descendant
// completion authority. All identity observations can become stale immediately.
package candidate

import (
	"cercano/source/server/internal/updatecoord/launch"
	"cercano/source/server/internal/updatecoord/nativeprocess"
	"errors"
	"fmt"
	"os"
)

var ErrIdentity = errors.New("candidate: owned launch identity unavailable")

type Candidate struct {
	process  *launch.Process
	identity nativeprocess.Identity
	bound    bool
	image    os.FileInfo // executable identity captured while binding the owned launch
}

// Start never adopts a PID. On post-start error the returned candidate retains
// Process for completion, but Identity refuses. A caller must not blindly retry.
func Start(opts launch.Options) (*Candidate, error) { return start(opts, nativeprocess.Inspect) }
func start(opts launch.Options, inspect func(int) (nativeprocess.Identity, error)) (*Candidate, error) {
	c := &Candidate{}
	p, e := launch.LaunchObserved(opts, func(pid int) error {
		id, e := inspect(pid)
		if e != nil {
			return e
		}
		if id.PID != pid {
			return ErrIdentity
		}
		wanted, e := os.Stat(opts.Executable)
		if e != nil {
			return e
		}
		actual, e := os.Stat(id.Executable)
		if e != nil {
			return e
		}
		if !os.SameFile(wanted, actual) {
			return ErrIdentity
		}
		again, e := inspect(pid)
		if e != nil {
			return e
		}
		if !id.SameProcess(again) {
			return ErrIdentity
		}
		c.identity = id
		c.image = actual
		return nil
	})
	if p == nil {
		return nil, e
	}
	c.process = p
	if e != nil {
		return c, fmt.Errorf("%w: %w", ErrIdentity, e)
	}
	c.bound = true
	return c, nil
}

// SelectedIdentity additionally refuses a replaced or changed executable since
// launch binding. It is read-only, never adopts a PID, and does not verify image
// content; callers must independently check persisted manifest fingerprints.
func (c *Candidate) SelectedIdentity() (nativeprocess.Identity, error) {
	id, err := c.Identity()
	if err != nil {
		return id, err
	}
	if c.image == nil {
		return nativeprocess.Identity{}, ErrIdentity
	}
	current, err := os.Stat(id.Executable)
	if err != nil {
		return nativeprocess.Identity{}, fmt.Errorf("%w: %w", ErrIdentity, err)
	}
	if !sameSelectedImage(c.image, current) {
		return nativeprocess.Identity{}, ErrIdentity
	}
	return id, nil
}

func sameSelectedImage(bound, current os.FileInfo) bool {
	return bound != nil && current != nil && os.SameFile(bound, current) && bound.Size() == current.Size() && bound.ModTime().Equal(current.ModTime())
}

func (c *Candidate) Process() *launch.Process {
	if c == nil {
		return nil
	}
	return c.process
}

// Identity reobserves the captured lifetime; it never substitutes a new process.
// The result is observational, not a lease on continued liveness.
func (c *Candidate) Identity() (nativeprocess.Identity, error) {
	if c == nil || !c.bound {
		return nativeprocess.Identity{}, ErrIdentity
	}
	select {
	case <-c.process.Done():
		return nativeprocess.Identity{}, ErrIdentity
	default:
	}
	now, e := nativeprocess.Inspect(c.process.Pid())
	if e != nil {
		return nativeprocess.Identity{}, fmt.Errorf("%w: %w", ErrIdentity, e)
	}
	if !c.identity.SameProcess(now) {
		return nativeprocess.Identity{}, ErrIdentity
	}
	select {
	case <-c.process.Done():
		return nativeprocess.Identity{}, ErrIdentity
	default:
	}
	return now, nil
}
