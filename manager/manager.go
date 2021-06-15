package manager

import "sync"

type Manager struct {
	mux sync.Mutex
}

func (m *Manager) ListDisks() error {
	return nil
}
