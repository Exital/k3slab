package labs

import (
	"context"
	"fmt"
	"os"
	"sync"

	"k3slab/cluster"
	"k3slab/engine"
	"k3slab/exposure"
	"k3slab/loghub"
)

// Catalog is the GET /api/labs response.
type Catalog struct {
	LabsRoot string `json:"labsRoot"`
	ActiveID string `json:"activeId"`
	Labs     []Meta `json:"labs"`
}

// WorkshopState is the workshop snapshot plus multi-lab context.
type WorkshopState struct {
	engine.Snapshot
	LabID    string `json:"labId,omitempty"`
	LabsRoot string `json:"labsRoot,omitempty"`
}

// Manager owns the active lab engine and lab selection.
type Manager struct {
	mu       sync.Mutex
	labsRoot string
	activeID string
	eng      *engine.Engine
	cluster  *cluster.Manager
	exposure *exposure.Watcher
	hub      *loghub.Hub
}

// NewManager resolves config, loads the initial engine, and returns a Manager.
func NewManager(hub *loghub.Hub, clusterMgr *cluster.Manager, watcher *exposure.Watcher) (*Manager, error) {
	labsRoot, activeID := ResolveConfig()
	eng, err := LoadEngine(labsRoot, activeID, hub)
	if err != nil {
		return nil, err
	}
	return &Manager{
		labsRoot: labsRoot,
		activeID: activeID,
		eng:      eng,
		cluster:  clusterMgr,
		exposure: watcher,
		hub:      hub,
	}, nil
}

// LabsRoot returns the configured labs parent directory.
func (m *Manager) LabsRoot() string {
	return m.labsRoot
}

// ActiveID returns the current lab folder name (may be empty).
func (m *Manager) ActiveID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeID
}

// ActiveLabRoot returns the active lab directory used for shell commands.
func (m *Manager) ActiveLabRoot() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.eng.LabRoot()
}

// WithEngine runs fn with the current engine while holding the manager lock.
func (m *Manager) WithEngine(fn func(eng *engine.Engine, labID, labsRoot string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fn(m.eng, m.activeID, m.labsRoot)
}

// WorkshopSnap builds API state from an engine snapshot (call while holding the manager lock via WithEngine).
func (m *Manager) WorkshopSnap(eng *engine.Engine, labID, labsRoot string) WorkshopState {
	return WorkshopState{
		Snapshot: eng.Snapshot(),
		LabID:    labID,
		LabsRoot: labsRoot,
	}
}

// WorkshopState returns the current workshop snapshot with lab metadata.
func (m *Manager) WorkshopState() WorkshopState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return WorkshopState{
		Snapshot: m.eng.Snapshot(),
		LabID:    m.activeID,
		LabsRoot: m.labsRoot,
	}
}

// Catalog returns discovered labs and the active id.
func (m *Manager) Catalog() (Catalog, error) {
	m.mu.Lock()
	activeID := m.activeID
	labsRoot := m.labsRoot
	m.mu.Unlock()

	labs, err := Discover(labsRoot)
	if err != nil {
		return Catalog{}, err
	}
	if labs == nil {
		labs = []Meta{}
	}
	return Catalog{
		LabsRoot: labsRoot,
		ActiveID: activeID,
		Labs:     labs,
	}, nil
}

// SelectLab activates a lab. Switching labs resets the cluster; re-selecting the same lab only restarts workshop progress.
func (m *Manager) SelectLab(ctx context.Context, id string) (WorkshopState, error) {
	if err := ValidateID(id); err != nil {
		return WorkshopState{}, err
	}
	meta, err := Discover(m.labsRoot)
	if err != nil {
		return WorkshopState{}, err
	}
	var found *Meta
	for i := range meta {
		if meta[i].ID == id {
			found = &meta[i]
			break
		}
	}
	if found == nil {
		return WorkshopState{}, ErrLabNotFound
	}
	if !found.Valid {
		if found.Error != "" {
			return WorkshopState{}, fmt.Errorf("%w: %s", ErrLabInvalid, found.Error)
		}
		return WorkshopState{}, ErrLabInvalid
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cluster.IsResetting() {
		return WorkshopState{}, cluster.ErrAlreadyResetting
	}

	sameLab := m.activeID == id

	if sameLab {
		if err := m.eng.Restart(); err != nil {
			return WorkshopState{}, err
		}
		return WorkshopState{
			Snapshot: m.eng.Snapshot(),
			LabID:    m.activeID,
			LabsRoot: m.labsRoot,
		}, nil
	}

	m.exposure.Clear()
	if err := m.cluster.Reset(ctx); err != nil {
		return WorkshopState{}, err
	}
	m.exposure.Sync()

	eng, err := LoadEngine(m.labsRoot, id, m.hub)
	if err != nil {
		return WorkshopState{}, err
	}
	m.eng = eng
	m.activeID = id
	_ = PersistActiveLab(id)
	_ = os.Setenv("K3SLAB_TERMINAL_CWD", eng.LabRoot())

	return WorkshopState{
		Snapshot: m.eng.Snapshot(),
		LabID:    m.activeID,
		LabsRoot: m.labsRoot,
	}, nil
}

// RestartWorkshop resets in-memory progress for the active lab.
func (m *Manager) RestartWorkshop() (WorkshopState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.eng.Restart(); err != nil {
		return WorkshopState{}, err
	}
	return WorkshopState{
		Snapshot: m.eng.Snapshot(),
		LabID:    m.activeID,
		LabsRoot: m.labsRoot,
	}, nil
}

// RestartLab resets the cluster and workshop progress for the active lab.
func (m *Manager) RestartLab(ctx context.Context) (WorkshopState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cluster.IsResetting() {
		return WorkshopState{}, cluster.ErrAlreadyResetting
	}
	if m.activeID == "" {
		return WorkshopState{
			Snapshot: m.eng.Snapshot(),
			LabID:    m.activeID,
			LabsRoot: m.labsRoot,
		}, nil
	}

	m.exposure.Clear()
	if err := m.cluster.Reset(ctx); err != nil {
		return WorkshopState{}, err
	}
	m.exposure.Sync()
	if err := m.eng.Restart(); err != nil {
		return WorkshopState{}, err
	}
	return WorkshopState{
		Snapshot: m.eng.Snapshot(),
		LabID:    m.activeID,
		LabsRoot: m.labsRoot,
	}, nil
}
