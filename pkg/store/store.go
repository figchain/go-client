package store

import (
	"sync"

	"github.com/figchain/go-client/pkg/model"
)

// Store defines the interface for storing FigFamilies.
type Store interface {
	Put(figFamily model.FigFamily)
	Get(namespace, key string) (*model.FigFamily, bool)
	GetAll() []model.FigFamily
}

// MemoryStore is an in-memory implementation of the Store interface.
type MemoryStore struct {
	mu   sync.RWMutex
	data map[string]model.FigFamily
}

// NewMemoryStore creates a new MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data: make(map[string]model.FigFamily),
	}
}

func (s *MemoryStore) Put(figFamily model.FigFamily) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := s.makeKey(figFamily.Definition.Namespace, figFamily.Definition.Key)
	s.data[key] = figFamily
}

func (s *MemoryStore) Get(namespace, key string) (*model.FigFamily, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	k := s.makeKey(namespace, key)
	val, ok := s.data[k]
	if !ok {
		return nil, false
	}
	return &val, true
}

func (s *MemoryStore) GetAll() []model.FigFamily {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var all []model.FigFamily
	for _, v := range s.data {
		all = append(all, v)
	}
	return all
}

func (s *MemoryStore) makeKey(namespace, key string) string {
	return namespace + ":" + key
}
