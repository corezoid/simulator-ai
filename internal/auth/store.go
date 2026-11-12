// Package auth holds the desktop human login: OAuth2 PKCE through SuperAdmin, a per-origin token
// store and the selected workspace. Host credentials from the environment always win over it.
package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// Entry is what one API origin keeps: the person's Simulator JWT, the refresh token SuperAdmin may
// issue with it, and the workspace the person chose. Tokens never leave this package.
type Entry struct {
	AccessToken  string    `json:"accessToken,omitempty"`
	RefreshToken string    `json:"refreshToken,omitempty"`
	ExpiresAt    time.Time `json:"expiresAt,omitzero"`
	Workspace    string    `json:"workspace,omitempty"`
}

type storeFile struct {
	Version int               `json:"version"`
	Origins map[string]*Entry `json:"origins"`
}

const storeVersion = 1

// Store is credentials.json in the user's config directory, keyed by API origin. Several host
// processes (Claude Code, Codex) may share it: every read goes to disk and every write replaces the
// file atomically, so a login in one is seen by the others.
type Store struct {
	path string
	mu   sync.Mutex
}

// NewStore keeps credentials in dir/credentials.json; dir is created 0700 when missing.
func NewStore(dir string) *Store { return &Store{path: filepath.Join(dir, "credentials.json")} }

// Path is the credentials file location (no secret).
func (s *Store) Path() string { return s.path }

// Get returns the origin's entry; a missing file or origin is an empty entry.
func (s *Store) Get(origin string) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.read()
	if err != nil {
		return Entry{}, err
	}
	if e := f.Origins[origin]; e != nil {
		return *e, nil
	}
	return Entry{}, nil
}

// Update applies change to the origin's entry and saves the file; an emptied entry is removed.
func (s *Store) Update(origin string, change func(*Entry)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.read()
	if err != nil {
		return err
	}
	e := Entry{}
	if old := f.Origins[origin]; old != nil {
		e = *old
	}
	change(&e)
	if e == (Entry{}) {
		delete(f.Origins, origin)
	} else {
		f.Origins[origin] = &e
	}
	return s.write(f)
}

func (s *Store) read() (storeFile, error) {
	f := storeFile{Version: storeVersion, Origins: map[string]*Entry{}}
	info, err := os.Stat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return f, errors.New("cannot read the Simulator AI credentials file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return f, fmt.Errorf("%s is readable by other users: run chmod 600 on it, or logout and login again", s.path)
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		return f, errors.New("cannot read the Simulator AI credentials file")
	}
	if err = json.Unmarshal(b, &f); err != nil || f.Version != storeVersion {
		return storeFile{}, fmt.Errorf("%s is not a credentials file of this version: remove it and login again", s.path)
	}
	if f.Origins == nil {
		f.Origins = map[string]*Entry{}
	}
	return f, nil
}

func (s *Store) write(f storeFile) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return errors.New("cannot create the Simulator AI config directory")
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return errors.New("cannot encode the credentials file")
	}
	tmp, err := os.CreateTemp(dir, ".credentials-*.json")
	if err != nil {
		return errors.New("cannot write the credentials file")
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }() // No-op after a successful rename.
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(append(b, '\n'))
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(name, s.path)
	}
	if err != nil {
		return errors.New("cannot write the credentials file")
	}
	return nil
}
