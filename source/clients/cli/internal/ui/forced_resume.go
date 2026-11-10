package ui

import (
	"cercano/source/server/pkg/proto"
	tea "charm.land/bubbletea/v2"
)

type forcedResumeOffer struct {
	original    *proto.LifecycleOriginalRecovery
	destination DevHostSelection
	requester   *proto.LifecycleIdentity
}

func (o *forcedResumeOffer) Error() string {
	return "contained source recovery requires fresh whole-host interruption confirmation"
}
func (m Model) offerForcedResume(offer *forcedResumeOffer) (tea.Model, tea.Cmd) {
	return m.offerContainedRecovery(offer)
}
