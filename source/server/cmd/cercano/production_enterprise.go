package main

import (
	"errors"
	"sync"

	"cercano/source/server/internal/enterprise"
	"cercano/source/server/internal/server"
	"cercano/source/server/pkg/proto"
	"context"
	"google.golang.org/grpc"
)

// productionEnterpriseDependencies is invocation-local. An explicit HostOptions
// supplies disposable authority, memory credentials and issuer HTTP transport in
// tests; nil retains ordinary authority resolution and lazy credential opening.
// The real Host always retains its connection lock and enforcement semantics.
type productionEnterpriseDependencies struct {
	options     *enterprise.HostOptions
	openBrowser func(context.Context, string) error
}

type productionEnterprisePolicy int

const (
	enterpriseLegacy productionEnterprisePolicy = iota + 1
	enterprisePreparedUnsupported
)

var errPreparedEnterpriseUnsupported = errors.New("prepared enterprise composition unsupported: connection ownership and policy freshness are not qualified")

type productionEnterprise struct {
	*enterprise.Host
	openBrowser func(context.Context, string) error
	once        sync.Once
}

// Reject prepared construction BEFORE resolving directories or opening credentials.
// This is not a standalone/no-policy fallback: prepared activation must remain off.
func composeProductionEnterprise(d productionEnterpriseDependencies, policy productionEnterprisePolicy) (*productionEnterprise, error) {
	if policy == enterprisePreparedUnsupported {
		return nil, errPreparedEnterpriseUnsupported
	}
	if policy != enterpriseLegacy {
		return nil, errors.New("explicit enterprise policy required")
	}
	var host *enterprise.Host
	var err error
	if d.options == nil {
		host, err = newEnterpriseHost()
	} else {
		host, err = enterprise.NewHost(*d.options)
	}
	if err != nil {
		return nil, err
	}
	browser := d.openBrowser
	if browser == nil {
		browser = openEnterpriseBrowser
	}
	return &productionEnterprise{Host: host, openBrowser: browser}, nil
}

func (e *productionEnterprise) serverOptions() []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(server.RecoveryUnaryInterceptor(), e.UnaryInterceptor()),
		grpc.ChainStreamInterceptor(server.RecoveryStreamInterceptor(), e.StreamInterceptor()),
	}
}
func (e *productionEnterprise) bindAdmission(s *server.Server) { s.SetBackgroundWorkAdmission(e.Begin) }
func (e *productionEnterprise) register(s grpc.ServiceRegistrar) {
	proto.RegisterEnterpriseServer(s, enterprise.NewRPCServer(e.Host, e.openBrowser))
}
func (e *productionEnterprise) Close() { e.once.Do(e.Host.Close) }
