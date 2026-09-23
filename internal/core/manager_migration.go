package core

import (
	"errors"
)

// ErrFenced is returned when the panel is fenced during master node cutover.
var ErrFenced = errors.New("server is fenced for migration: mutations are halted")

// SetFenced enables or disables mutation fencing on this panel instance.
func (m *Manager) SetFenced(fenced bool) {
	m.fenced.Store(fenced)
}

// IsFenced reports whether mutations are blocked because the master is being migrated.
func (m *Manager) IsFenced() bool {
	return m.fenced.Load()
}
