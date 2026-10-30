package server

import "cercano/source/server/internal/conversation"

// GenerationStore borrows the exact lifecycle store during ordinary composition.
// Its sole disposal owner remains CloseCoexistenceStore.
func (s *Server) GenerationStore() conversation.Store {
	if s == nil || s.coexistence == nil {
		return nil
	}
	return s.coexistence.store
}
