//go:build !developmentfixture || !cercano_development_acceptance

package worker

import "cercano/source/server/pkg/proto"

func observeAcceptanceTurn(*proto.StartTurn) {}

func ObserveAcceptanceStartup() {}
