package main

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/server"
	"cercano/source/server/pkg/devhostauth"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Exercise the actual retirement handler and the entrypoint's transport/shutdown
// wiring. No cancellation or transport loss may be needed for a successful exit.
func TestDevelopmentHostRetirementWakesOwner(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			dir := t.TempDir()
			path := filepath.Join(dir, "fixture.db")
			if err := conversation.PrepareCoexistence(ctx, path); err != nil {
				t.Fatal(err)
			}
			token := filepath.Join(dir, "token")
			if err := devhostauth.Create(token); err != nil {
				t.Fatal(err)
			}
			auth, err := devhostauth.Load("host", token)
			if err != nil {
				t.Fatal(err)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			sentinel := errors.New("retirement disposal failure")
			resources := newPreparedShutdown(nil, func() error {
				close(entered)
				<-release
				if fail {
					return sentinel
				}
				return nil
			})
			contract := conversation.CurrentCoexistenceDeclaration()
			srv, err := server.NewCoexistenceServer(ctx, server.CoexistenceOptions{
				StorePath: path, Host: "host", Build: "build", Contract: contract,
				ControlAuthorize:   auth.Authorize,
				ControlDial:        func(context.Context, string) (*grpc.ClientConn, error) { return nil, errors.New("unused peer dial") },
				Reload:             func(context.Context, conversation.Store, *conversation.Ownership) error { return nil },
				CloseParticipation: resources.Wait,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer srv.CloseCoexistenceStore()
			transport, shutdown := newDevelopmentHostTransport(srv, resources, auth)
			defer transport.Stop()
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			served := make(chan error, 1)
			go func() { served <- transport.Serve(listener) }()
			ownerDone := make(chan error, 1)
			go func() {
				err := waitDevelopmentHost(ctx, served, srv.CoexistenceRetired())
				ownerDone <- retainPreparedShutdown(finishPreparedShutdown(shutdown, err))
			}()
			conn, err := grpc.NewClient(listener.Addr().String(), append([]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}, auth.DialOptions()...)...)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			client := proto.NewAgentClient(conn)
			request := &proto.LifecycleControlRequest{Target: &proto.LifecycleIdentity{Build: "build", Host: "host", Store: path, Contract: contract.Contract, Schema: int32(contract.SchemaVersion)}}
			rpcDone := make(chan error, 1)
			go func() {
				response, err := client.LifecycleRetire(ctx, request)
				if err == nil && !response.Ready {
					err = errors.New("retirement not ready")
				}
				rpcDone <- err
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("retirement did not enter disposal")
			}
			select {
			case <-srv.CoexistenceRetired():
				t.Fatal("incomplete retirement notified owner")
			default:
			}
			select {
			case err := <-ownerDone:
				t.Fatalf("owner exited before disposal: %v", err)
			default:
			}
			close(release)
			select {
			case err := <-rpcDone:
				if fail == (err == nil) {
					t.Fatalf("retirement response: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("RPC response did not drain")
			}
			if fail {
				select {
				case <-srv.CoexistenceRetired():
					t.Fatal("failed retirement notified owner")
				default:
				}
				if _, err := client.LifecycleRetire(ctx, request); err == nil {
					t.Fatal("retirement failure not retained/observable")
				}
				select {
				case err := <-ownerDone:
					t.Fatalf("failed retirement stopped host: %v", err)
				default:
				}
				cancel() // Only failed retirement requires external owner shutdown.
			}
			select {
			case err := <-ownerDone:
				if fail {
					if !errors.Is(err, sentinel) {
						t.Fatalf("lost failure: %v", err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("successful retirement did not wake host owner")
			}
		})
	}
}
