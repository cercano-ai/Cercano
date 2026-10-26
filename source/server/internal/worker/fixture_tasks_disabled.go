//go:build !devhostfixture

package worker

import (
	"cercano/source/server/internal/capabilities"
	"cercano/source/server/internal/runner"
)

func configureDevHostFixtureTasks(tools runner.ToolSvc, tasks capabilities.Tasks) {}
