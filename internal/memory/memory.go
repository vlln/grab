package memory

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

type Entry struct {
	Fingerprint  string `json:"fingerprint"`
	Referer      string `json:"referer"`
	LastSuccess  string `json:"last_success"`
	SuccessCount int    `json:"success_count"`
}

type storeData struct {
	Entries map[string]Entry `json:"entries"`
}

type Store struct {
	path string
	data storeData
}

func NewStore(path string) *Store {
	return &Store{
		path: path,
		data: storeData{Entries: make(map[string]Entry)},
	}
}

func (s *Store) Load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &s.data)
}

func (s *Store) Save() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	// Atomic write: temp file + rename
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) Get(domain string) (Entry, bool) {
	e, ok := s.data.Entries[domain]
	return e, ok
}

func (s *Store) Set(domain string, fingerprint, referer string) {
	s.data.Entries[domain] = Entry{
		Fingerprint:  fingerprint,
		Referer:      referer,
		LastSuccess:  time.Now().UTC().Format(time.RFC3339),
		SuccessCount: s.data.Entries[domain].SuccessCount + 1,
	}
}

func Domain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.Host
}