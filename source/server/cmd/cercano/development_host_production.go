package main

import (
	"cercano/source/server/internal/server"
	"cercano/source/server/pkg/devhostauth"
	"context"
	"fmt"
)

// Fail closed until the prepared production worker bridge can carry both child
// and parent ownership through every dispatch persistence callback. In
// particular, do not substitute SelectExecutionMode or fixture execution here.
func newProductionDevelopmentHost(ctx context.Context, o developmentHostOptions, auth *devhostauth.Auth) (*server.Server, func(), error) {
	return nil, nil, fmt.Errorf("production development-host runtime unavailable: prepared provider and exact-owned child worker bridge are not installed")
}
