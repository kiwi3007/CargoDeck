package monitor

import "sync"

// ImportStatus tracks which download IDs are currently being imported.
type ImportStatus struct {
	mu      sync.RWMutex
	active  map[string]bool
}

func NewImportStatus() *ImportStatus {
	return &ImportStatus{active: make(map[string]bool)}
}

func (s *ImportStatus) MarkImporting(id string) {
	s.mu.Lock()
	s.active[id] = true
	s.mu.Unlock()
}

func (s *ImportStatus) MarkFinished(id string) {
	s.mu.Lock()
	delete(s.active, id)
	s.mu.Unlock()
}

func (s *ImportStatus) IsImporting(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.active[id]
}
