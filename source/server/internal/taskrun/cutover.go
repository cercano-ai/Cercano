package taskrun

import "cercano/source/server/internal/broker"

// CancelConversation fences new independent launches and cancels all in-memory
// executions in this conversation's descendant tree. Finished task records are
// not mistaken for running work. The host must still wait for admission tickets
// (including terminal persistence) before relinquishing ownership.
func (m *Manager) CancelConversation(parent, epoch string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cutoverParents == nil {
		m.cutoverParents = make(map[string]bool)
	}
	if m.cutoverEpoch == nil {
		m.cutoverEpoch = make(map[string]string)
	}
	m.cutoverEpoch[parent] = epoch
	m.cutoverParents[parent] = true
	// Keep ancestry for the manager lifetime: a finished task can have a live
	// independent descendant. All launch publication uses this same mutex.
	selected := map[string]bool{parent: true}
	for changed := true; changed; {
		changed = false
		for child, p := range m.childParents {
			if selected[p] && !selected[child] {
				selected[child] = true
				m.cutoverParents[child] = true
				changed = true
			}
		}
	}
	for _, e := range m.active {
		if selected[e.parent] && m.cutoverLocked(e.parent, e.owner) {
			e.stop = true
			e.cancel()
		}
	}
}

func (m *Manager) cutoverLocked(parent string, owner broker.Owner) bool {
	return m.cutoverParents[parent] && (m.cutoverEpoch[parent] == "" || m.cutoverEpoch[parent] == owner.Epoch)
}
