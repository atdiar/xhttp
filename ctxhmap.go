package xhttp

import (
	"errors"
	"sync"
)

// This file wraps an interface around the traditional Go hashmap that shall be
// safe for concurrent use.
// We use it as the storage datastructure for a execution.Context

var (
	// ErrNotFound is returned by the Get method of the hashmap when no value could be found
	// for a given key.
	ErrNotFound = errors.New("Item not found.")
)

// Mutex protected map[interface{}]interface{} for use by the execution.Context
type hmap struct {
	mutex *sync.RWMutex
	store map[interface{}]interface{}
}

func newHMap() *hmap {
	h := new(hmap)
	h.mutex = new(sync.RWMutex)
	h.store = make(map[interface{}]interface{})
	return h
}

func (h *hmap) Get(k interface{}) (interface{}, error) {
	h.mutex.RLock()
	res, ok := h.store[k]
	if !ok {
		h.mutex.RUnlock()
		return nil, ErrNotFound
	}
	h.mutex.RUnlock()
	return res, nil
}

func (h *hmap) Put(k, v interface{}) {
	h.mutex.Lock()
	h.store[k] = v
	h.mutex.Unlock()
}

func (h *hmap) Delete(key interface{}) {
	h.mutex.Lock()
	delete(h.store, key)
	h.mutex.Unlock()
}

func (h *hmap) Clear() {
	h.mutex.Lock()
	h.store = make(map[interface{}]interface{})
	h.mutex.Unlock()
}
