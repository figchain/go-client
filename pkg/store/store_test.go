package store

import (
	"reflect"
	"testing"

	"github.com/figchain/go-client/pkg/model"
)

func TestMemoryStore(t *testing.T) {
	s := NewMemoryStore()

	figFamily := model.FigFamily{
		Definition: model.FigDefinition{
			Key:       "key1",
			Namespace: "ns1",
		},
	}

	// Test Put
	s.Put(figFamily)

	// Test Get
	got, ok := s.Get("ns1", "key1")
	if !ok {
		t.Error("Get() returned false, want true")
	}
	if !reflect.DeepEqual(*got, figFamily) {
		t.Errorf("Get() = %v, want %v", got, figFamily)
	}

	// Test Get missing
	_, ok = s.Get("ns1", "missing")
	if ok {
		t.Error("Get() returned true for missing key")
	}

	// Test GetAll
	all := s.GetAll()
	if len(all) != 1 {
		t.Errorf("GetAll() returned %d items, want 1", len(all))
	}
	if !reflect.DeepEqual(all[0], figFamily) {
		t.Errorf("GetAll()[0] = %v, want %v", all[0], figFamily)
	}
}
