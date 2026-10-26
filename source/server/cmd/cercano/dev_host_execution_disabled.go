//go:build !devhostfixture

package main

import "cercano/source/server/internal/server"
import "cercano/source/server/internal/worker"

func configureDevHostExecution(*server.Server) {}
func devHostExecutionAllowed(string) bool      { return false }

func configureDevHostWorker(worker.ProfileReaderSetter) {}
