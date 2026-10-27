package main

import (
	"fmt"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
)

// productionAuthorityResolvers resolves canonical authorities, not prepared
// snapshots. Startup admission resolves these before mutation-capable stages.
type productionAuthorityResolvers struct {
	configPath       func() string
	conversationPath func() (string, error)
}

type productionAuthorityPaths struct{ config, conversations string }

func productionAuthorityDefaults() productionAuthorityResolvers {
	return productionAuthorityResolvers{config.DefaultPath, conversation.DefaultPath}
}

func (r productionAuthorityResolvers) resolve() (productionAuthorityPaths, error) {
	if r.configPath == nil || r.conversationPath == nil {
		return productionAuthorityPaths{}, fmt.Errorf("authority resolvers required")
	}
	p := productionAuthorityPaths{config: r.configPath()}
	var err error
	p.conversations, err = r.conversationPath()
	if err != nil {
		return productionAuthorityPaths{}, fmt.Errorf("resolve conversation store path: %w", err)
	}
	if p.config == "" || p.conversations == "" {
		return productionAuthorityPaths{}, fmt.Errorf("authority paths required")
	}
	return p, nil
}

// The same invocation-local opener is used by ordinary and prepared production
// composition. No mutable global or environment-selected fake is supported.
type productionCredentialDependencies struct{ openSecrets func() (secrets.Store, error) }

func productionCredentialDefaults() productionCredentialDependencies {
	return productionCredentialDependencies{openSecrets: secrets.OpenKeychain}
}

func (d productionCredentialDependencies) open(options secrets.Options) (secrets.Store, error) {
	// Admission must precede even construction of a native credential backend.
	if err := secrets.CheckAllowed(); err != nil {
		return nil, err
	}
	if d.openSecrets == nil {
		return nil, fmt.Errorf("credential opener required")
	}
	st, err := d.openSecrets()
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, fmt.Errorf("credential opener returned nil store")
	}
	return secrets.Guard(st, options), nil
}
