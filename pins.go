package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

const pinMarker = "★ "

// pinStore keeps a local list of pinned conversation IDs. Teams no longer
// exposes pinned chats through the APIs this client uses, so pins live in a
// small JSON file under the user's config directory instead.
type pinStore struct {
	path string
	mu   sync.Mutex
	ids  map[string]struct{}
}

type pinFile struct {
	Pinned []string `json:"pinned"`
}

func defaultPinsFilePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("unable to determine user config directory: %v", err)
	}

	return filepath.Join(configDir, "teams-cli", "pins.json"), nil
}

func loadPinStore(path string) (*pinStore, error) {
	store := &pinStore{path: path, ids: map[string]struct{}{}}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("unable to read %s: %v", path, err)
	}

	var contents pinFile
	if err := json.Unmarshal(data, &contents); err != nil {
		return nil, fmt.Errorf("unable to parse %s: %v", path, err)
	}
	for _, id := range contents.Pinned {
		if id != "" {
			store.ids[id] = struct{}{}
		}
	}

	return store, nil
}

func (p *pinStore) pinned(id string) bool {
	if p == nil {
		return false
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.ids[id]
	return ok
}

// toggle flips the pin state of id, persists the result, and reports the new state.
func (p *pinStore) toggle(id string) (bool, error) {
	if p == nil {
		return false, fmt.Errorf("pin store is unavailable")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if _, ok := p.ids[id]; ok {
		delete(p.ids, id)
	} else {
		p.ids[id] = struct{}{}
	}

	if err := p.saveLocked(); err != nil {
		return false, err
	}

	_, nowPinned := p.ids[id]
	return nowPinned, nil
}

func (p *pinStore) saveLocked() error {
	ids := make([]string, 0, len(p.ids))
	for id := range p.ids {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	data, err := json.MarshalIndent(pinFile{Pinned: ids}, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(p.path), 0o700); err != nil {
		return fmt.Errorf("unable to create %s: %v", filepath.Dir(p.path), err)
	}

	tmp := p.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("unable to write %s: %v", tmp, err)
	}
	if err := os.Rename(tmp, p.path); err != nil {
		return fmt.Errorf("unable to replace %s: %v", p.path, err)
	}

	return os.Chmod(p.path, 0o600)
}
