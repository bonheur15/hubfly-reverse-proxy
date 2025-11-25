package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/hubfly/hubfly-reverse-proxy/internal/models"
)

type Store interface {
	ListSites() ([]models.Site, error)
	GetSite(id string) (*models.Site, error)
	SaveSite(site *models.Site) error
	DeleteSite(id string) error
}

type JSONStore struct {
	filePath string
	mu       sync.RWMutex
	sites    map[string]models.Site
}

func NewJSONStore(dir string) (*JSONStore, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	filePath := filepath.Join(dir, "metadata.json")
	s := &JSONStore{
		filePath: filePath,
		sites:    make(map[string]models.Site),
	}

	if err := s.load(); err != nil {
		// If file doesn't exist, that's fine, start empty
		if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return s, nil
}

func (s *JSONStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}

	if len(data) == 0 {
		return nil
	}

	return json.Unmarshal(data, &s.sites)
}

// save writes to disk. Caller must hold lock.
func (s *JSONStore) save() error {
	data, err := json.MarshalIndent(s.sites, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0644)
}

func (s *JSONStore) ListSites() ([]models.Site, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]models.Site, 0, len(s.sites))
	for _, site := range s.sites {
		list = append(list, site)
	}
	return list, nil
}

func (s *JSONStore) GetSite(id string) (*models.Site, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	site, ok := s.sites[id]
	if !ok {
		return nil, fmt.Errorf("site not found: %s", id)
	}
	return &site, nil
}

func (s *JSONStore) SaveSite(site *models.Site) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sites[site.ID] = *site
	return s.save()
}

func (s *JSONStore) DeleteSite(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sites, id)
	return s.save()
}

// saveAtomic is removed as it is no longer needed.
