package server

import (
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/proto"
	"context"
	"errors"
	"testing"
)

func TestDisabledInlineCredentialUpdateRefusedBeforeMutation(t *testing.T) {
	t.Setenv(secrets.DisableEnv, "1")
	// No initialized services/files/backend: rejection must precede all of them.
	var s Server
	_, err := s.UpdateConfig(context.Background(), &proto.UpdateConfigRequest{CloudApiKey: "inline"})
	if !errors.Is(err, secrets.ErrDisabled) {
		t.Fatal(err)
	}
}
