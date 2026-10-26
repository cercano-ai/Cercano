package modelhost

import "sync"

// holdTable records which clients hold which instances. A hold is a
// (clientID, instanceID) pair; an instance is stopped only when its last hold
// is released. One mutex guards the whole table.
type holdTable struct {
	mu         sync.Mutex
	byInstance map[string]map[string]struct{}
}

func newHoldTable() *holdTable {
	return &holdTable{byInstance: map[string]map[string]struct{}{}}
}

// add records client's hold on instance.
func (h *holdTable) add(client, instance string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.byInstance[instance]
	if set == nil {
		set = map[string]struct{}{}
		h.byInstance[instance] = set
	}
	set[client] = struct{}{}
}

// remove drops client's hold on instance and returns how many holds remain.
func (h *holdTable) remove(client, instance string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.byInstance[instance]
	delete(set, client)
	if len(set) == 0 {
		delete(h.byInstance, instance)
		return 0
	}
	return len(set)
}

// count returns the number of holds on instance.
func (h *holdTable) count(instance string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.byInstance[instance])
}

// heldBy returns the instances client holds.
func (h *holdTable) heldBy(client string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for instance, set := range h.byInstance {
		if _, ok := set[client]; ok {
			out = append(out, instance)
		}
	}
	return out
}

// move transfers every hold on from to to, for a restart that changed the
// instance ID.
func (h *holdTable) move(from, to string) {
	if from == to {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.byInstance[from]
	if len(set) == 0 {
		return
	}
	delete(h.byInstance, from)
	dst := h.byInstance[to]
	if dst == nil {
		dst = map[string]struct{}{}
		h.byInstance[to] = dst
	}
	for c := range set {
		dst[c] = struct{}{}
	}
}

// drop removes every hold on instance.
func (h *holdTable) drop(instance string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.byInstance, instance)
}

// clear drops every hold.
func (h *holdTable) clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.byInstance = map[string]map[string]struct{}{}
}

// has reports whether this client owns a hold. Lifecycle callers hold the model lock.
func (h *holdTable) has(client, instance string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, ok := h.byInstance[instance][client]
	return ok
}
