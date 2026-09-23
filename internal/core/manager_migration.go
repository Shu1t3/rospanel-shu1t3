package core

import (
	"errors"
	"sync/atomic"

	"github.com/Shu1t3/rospanel-shu1t3/internal/migration"
)

// ErrFenced is returned when the panel is fenced during master node cutover.
var ErrFenced = errors.New("server is fenced for migration: mutations are halted")

// migrationState holds migration coordinator references on Manager.
type migrationState struct {
	fenced atomic.Bool
	coord  *migration.Coordinator
}

// SetFenced enables or disables mutation fencing on this panel instance.
func (m *Manager) SetFenced(fenced bool) {
	m.migration.fenced.Store(fenced)
}

// IsFenced reports whether mutations are blocked because the master is being migrated.
func (m *Manager) IsFenced() bool {
	return m.migration.fenced.Load()
}

// InitMigration initializes the migration coordinator for the manager.
func (m *Manager) InitMigration(dataDir string) error {
	coord, err := migration.NewCoordinator(dataDir, m.store)
	if err != nil {
		return err
	}
	m.migration.coord = coord
	// Sync fencing state from persistent migration session
	if coord.StateManager().IsFenced() {
		m.migration.fenced.Store(true)
	}
	return nil
}

// MigrationCoordinator returns the active migration coordinator (may be nil).
func (m *Manager) MigrationCoordinator() *migration.Coordinator {
	return m.migration.coord
}
