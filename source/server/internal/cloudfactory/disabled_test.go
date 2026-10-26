package cloudfactory

import (
	"cercano/source/server/internal/anthropicauth"
	"cercano/source/server/internal/chatgptauth"
	"cercano/source/server/internal/hostsvc/credentials"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
	"context"
	"errors"
	"testing"
)

func TestDisabledCredentialsCannotBypassStore(t *testing.T) {
	t.Setenv(secrets.DisableEnv, "1")
	for _, flavor := range []string{FlavorMessages, FlavorChatCompletions, FlavorResponses, FlavorBedrock} {
		_, err := BuildCloudProvider(config.CloudProfile{Flavor: flavor}, "inline-or-env-secret")
		if !errors.Is(err, secrets.ErrDisabled) {
			t.Fatalf("%s: %v", flavor, err)
		}
	}
	ctx := context.Background()
	svc := credentials.New(secrets.NewMemory())
	_, login := svc.BeginLogin(ctx, "p", "anthropic")
	_, aStart := (anthropicauth.Flow{}).Start(ctx)
	_, aRefresh := (anthropicauth.Flow{}).Refresh(ctx, "secret")
	_, cStart := (chatgptauth.Flow{}).Start(ctx)
	_, cRefresh := (chatgptauth.Flow{}).Refresh(ctx, "secret")
	for _, err := range []error{login, aStart, aRefresh, cStart, cRefresh} {
		if !errors.Is(err, secrets.ErrDisabled) {
			t.Fatal(err)
		}
	}
}
