//go:build devhostfixture

package server

import "cercano/source/server/internal/worker"

// WireDevHostFixtureTasks connects the existing server-owned task service to
// the explicitly injected runner. This seam does not exist in release builds.
func (s *Server) WireDevHostFixtureTasks() {
	s.workerRunner.(worker.TasksSetter).SetTasks(serverTasks{s})
}
