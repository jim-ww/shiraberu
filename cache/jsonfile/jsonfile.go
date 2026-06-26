package jsonfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

const currentVersion = 1

type Entry struct {
	Expires time.Time `json:"expires"`
	Value   []byte    `json:"value"`
}

type data struct {
	Version int              `json:"version"`
	Entries map[string]Entry `json:"entries"`
}

type JSONFileStore struct {
	path       string
	dirty      bool
	ttl        time.Duration
	cache      map[string]Entry
	mu         sync.RWMutex
	maxEntries int
}

const cacheFileName = "cache.json"

func New(path string, entryTTL time.Duration, maxEntries int) (*JSONFileStore, error) {
	store := &JSONFileStore{
		path:       path,
		ttl:        entryTTL,
		cache:      map[string]Entry{},
		maxEntries: maxEntries,
	}
	f, err := os.Open(filepath.Join(path, cacheFileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return store, nil
		}
		return nil, fmt.Errorf("file open: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	var data data
	if err := json.NewDecoder(f).Decode(&data); err != nil {
		if errors.Is(err, io.EOF) {
			return store, nil
		}
		return nil, fmt.Errorf("json decode: %w", err)
	}
	if data.Version != currentVersion {
		return nil, fmt.Errorf("version mismatch: got=%v expected=%v", data.Version, currentVersion)
	}
	store.cache = data.Entries

	return store, nil
}

func (s *JSONFileStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.dirty {
		return nil
	}
	return s.writeChangesToDiskNoLock()
}

func (s *JSONFileStore) writeChangesToDiskNoLock() error {
	if s.maxEntries > 0 && len(s.cache) > int(s.maxEntries) {
		s.evictOldestEntries()
	}
	data, err := json.Marshal(data{
		Version: currentVersion,
		Entries: s.cache,
	})
	if err != nil {
		return fmt.Errorf("json encode: %w", err)
	}

	if err := os.MkdirAll(s.path, 0o755); err != nil {
		return fmt.Errorf("mkdir all: %w", err)
	}

	tempPath := filepath.Join(s.path, cacheFileName+".tmp")

	if err := os.WriteFile(tempPath, data, 0o644); err != nil {
		return fmt.Errorf("write temp cache file: %w", err)
	}

	if err := os.Rename(tempPath, filepath.Join(s.path, cacheFileName)); err != nil {
		return fmt.Errorf("rename: %w", err)
	}

	s.dirty = false
	return nil
}

func (s *JSONFileStore) evictOldestEntries() {
	type kv struct {
		key   string
		value Entry
	}
	converted := func() iter.Seq[kv] {
		return func(yield func(kv) bool) {
			for k, v := range s.cache {
				if !yield(kv{key: k, value: v}) {
					return
				}
			}
		}
	}
	sorted := slices.SortedFunc(converted(), func(a, b kv) int {
		return a.value.Expires.Compare(b.value.Expires)
	})
	s.cache = make(map[string]Entry, s.maxEntries)
	for _, keyValue := range sorted[len(sorted)-s.maxEntries:] {
		s.cache[keyValue.key] = keyValue.value
	}
}

func (s *JSONFileStore) Get(key string) (value []byte, exists bool) {
	s.mu.RLock()
	v, ok := s.cache[key]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(v.Expires) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if v2, ok := s.cache[key]; ok && time.Now().After(v2.Expires) {
			delete(s.cache, key)
			s.dirty = true
		}
		return nil, false
	}
	return v.Value, true
}

func (s *JSONFileStore) Set(key string, value []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[key] = Entry{
		Value:   value,
		Expires: time.Now().Add(s.ttl),
	}
	s.dirty = true
}
