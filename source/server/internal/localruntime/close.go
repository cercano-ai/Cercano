package localruntime

import (
	"context"
	"errors"
	"time"
)

// admitWork fences late startup warming/resume callbacks as well as requests.
func (m *InMemoryManager) admitWork(ctx context.Context) (context.Context, func(), error) {
	m.lifeMu.Lock()
	defer m.lifeMu.Unlock()
	if m.closing {
		return nil, nil, errors.New("runtime manager closed")
	}
	if m.lifeCtx == nil {
		m.lifeCtx, m.lifeCancel = context.WithCancel(context.Background())
	}
	m.lifeWork.Add(1)
	child, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(m.lifeCtx, cancel)
	return child, func() { stop(); cancel(); m.lifeWork.Done() }, nil
}

// Close is only for this in-process manager, never the shared model host.
// Cancel and join admitted starts/downloads before enumerating instances: a
// startup warm may publish its instance while cancellation is being handled.
// Providers must honor cancellation; this is drainage, not retirement readiness.
func (m *InMemoryManager) Close() error {
	m.closeOnce.Do(func() {
		m.lifeMu.Lock()
		m.closing = true
		if m.lifeCancel != nil {
			m.lifeCancel()
		}
		m.lifeMu.Unlock()
		m.lifeWork.Wait()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		instances, err := m.Instances(ctx)
		m.closeErr = err
		for _, instance := range instances {
			if instance.ID != "" && instance.State != InstanceStopped {
				m.closeErr = errors.Join(m.closeErr, m.Stop(ctx, StopRequest{InstanceID: instance.ID}))
			}
		}
	})
	return m.closeErr
}
