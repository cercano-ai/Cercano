// Package recoveryguard classifies only expected live-source refusals.
// All other errors, including unknown inspection failures, remain fail-closed.
package recoveryguard

// LiveSourceRefusal is an expected inability to prove absence, not permission
// to stop a process. Only exact live-process detection or launch-lock contention
// may produce this outcome; metadata and inspection failures must not use it.
type LiveSourceRefusal struct{}

func (*LiveSourceRefusal) Error() string { return "source is live or launch is in progress" }

var ErrLiveSource = &LiveSourceRefusal{}
