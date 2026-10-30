package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPinStoreTreatsMissingFileAsEmpty(t *testing.T) {
	store, err := loadPinStore(filepath.Join(t.TempDir(), "missing", "pins.json"))
	if err != nil {
		t.Fatalf("expected missing file to load as empty, got %v", err)
	}
	if store.pinned("19:anything") {
		t.Fatal("expected no pins in a fresh store")
	}
}

func TestPinStoreTogglePersistsAcrossLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "pins.json")
	store, err := loadPinStore(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	pinned, err := store.toggle("19:chat")
	if err != nil || !pinned {
		t.Fatalf("expected first toggle to pin, got pinned=%t err=%v", pinned, err)
	}

	reloaded, err := loadPinStore(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !reloaded.pinned("19:chat") {
		t.Fatal("expected pin to survive reload")
	}

	pinned, err = reloaded.toggle("19:chat")
	if err != nil || pinned {
		t.Fatalf("expected second toggle to unpin, got pinned=%t err=%v", pinned, err)
	}
	final, _ := loadPinStore(path)
	if final.pinned("19:chat") {
		t.Fatal("expected unpin to persist")
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("expected pins file with 0600 permissions, got %v err=%v", info, err)
	}
}

func TestPinStoreRejectsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pins.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPinStore(path); err == nil {
		t.Fatal("expected corrupt pins file to be reported")
	}
}
