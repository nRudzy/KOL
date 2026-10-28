package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

var metaBucket = []byte("entries-v1")
var dataBucket = []byte("payloads-v1")
var ErrNotFound = errors.New("cette copie n’existe plus")

type Store struct {
	mu      sync.RWMutex
	db      *bolt.DB
	cipher  *Cipher
	entries map[string]Entry
	closed  bool
}

func Open(path string, key []byte) (*Store, error) {
	c, err := NewCipher(key)
	if err != nil {
		return nil, err
	}
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, fmt.Errorf("ouvrir l’historique : %w", err)
	}
	s := &Store{db: db, cipher: c, entries: make(map[string]Entry)}
	err = db.Update(func(tx *bolt.Tx) error {
		meta, err := tx.CreateBucketIfNotExists(metaBucket)
		if err != nil {
			return err
		}
		if _, err = tx.CreateBucketIfNotExists(dataBucket); err != nil {
			return err
		}
		return meta.ForEach(func(k, v []byte) error {
			if v == nil {
				return errors.New("structure d’historique invalide")
			}
			var e Entry
			if err := s.decode(v, &e); err != nil {
				return fmt.Errorf("historique illisible (clé ou données) : %w", err)
			}
			if e.ID != string(k) {
				return errors.New("identifiant d’historique invalide")
			}
			s.entries[e.ID] = e
			return nil
		})
	})
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) encode(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return s.cipher.Seal(b)
}

func (s *Store) decode(data []byte, v any) error {
	b, err := s.cipher.Open(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func (s *Store) Add(clip Snapshot, at time.Time) (Entry, error) {
	if len(clip.Formats) == 0 {
		return Entry{}, errors.New("aucun format conservable dans cette copie")
	}
	e := Summarize(clip)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Entry{}, bolt.ErrDatabaseNotOpen
	}
	old, exists := s.entries[e.ID]
	e.FirstAt = at
	e.LastAt = at
	e.Copies = 1
	if exists {
		e.FirstAt = old.FirstAt
		e.Copies = old.Copies + 1
		e.Pinned = old.Pinned
	}
	err := s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(metaBucket)
		var err error
		e.Order, err = meta.NextSequence()
		if err != nil {
			return err
		}
		if !exists {
			payload, err := s.encode(clip)
			if err != nil {
				return err
			}
			if err = tx.Bucket(dataBucket).Put([]byte(e.ID), payload); err != nil {
				return err
			}
		}
		encoded, err := s.encode(e)
		if err != nil {
			return err
		}
		return meta.Put([]byte(e.ID), encoded)
	})
	if err == nil {
		s.entries[e.ID] = e
	}
	return cloneEntry(e), err
}

func cloneEntry(e Entry) Entry {
	e.Files = append([]string(nil), e.Files...)
	e.FormatNames = append([]string(nil), e.FormatNames...)
	e.Omitted = append([]string(nil), e.Omitted...)
	return e
}

func (s *Store) List(query, kind string) []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Entry, 0, len(s.entries))
	query = strings.ToLower(query)
	for _, e := range s.entries {
		if kind == "pinned" && !e.Pinned {
			continue
		}
		if kind != "" && kind != "all" && kind != "pinned" {
			if kind == "text" {
				if e.Kind != "text" && e.Kind != "link" {
					continue
				}
			} else if e.Kind != kind {
				continue
			}
		}
		if query != "" && !strings.Contains(strings.ToLower(e.Text+"\n"+e.Title+"\n"+e.Source+"\n"+strings.Join(e.Files, "\n")+"\n"+strings.Join(e.FormatNames, "\n")), query) {
			continue
		}
		result = append(result, cloneEntry(e))
	}
	sort.Slice(result, func(i, j int) bool {
		if !result[i].LastAt.Equal(result[j].LastAt) {
			return result[i].LastAt.After(result[j].LastAt)
		}
		return result[i].Order > result[j].Order
	})
	return result
}

func (s *Store) Get(id string) (Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var clip Snapshot
	if s.closed {
		return clip, bolt.ErrDatabaseNotOpen
	}
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(dataBucket).Get([]byte(id))
		if v == nil {
			return ErrNotFound
		}
		return s.decode(v, &clip)
	})
	return clip, err
}

func (s *Store) Pin(id string, pinned bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return bolt.ErrDatabaseNotOpen
	}
	e, ok := s.entries[id]
	if !ok {
		return ErrNotFound
	}
	e.Pinned = pinned
	encoded, err := s.encode(e)
	if err != nil {
		return err
	}
	err = s.db.Update(func(tx *bolt.Tx) error { return tx.Bucket(metaBucket).Put([]byte(id), encoded) })
	if err == nil {
		s.entries[id] = e
	}
	return err
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return bolt.ErrDatabaseNotOpen
	}
	err := s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(dataBucket).Delete([]byte(id)); err != nil {
			return err
		}
		return tx.Bucket(metaBucket).Delete([]byte(id))
	})
	if err == nil {
		delete(s.entries, id)
	}
	return err
}

// Clear deliberately retains pins. Individual pinned entries can be deleted.
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return bolt.ErrDatabaseNotOpen
	}
	var removed []string
	err := s.db.Update(func(tx *bolt.Tx) error {
		for id, e := range s.entries {
			if e.Pinned {
				continue
			}
			if err := tx.Bucket(dataBucket).Delete([]byte(id)); err != nil {
				return err
			}
			if err := tx.Bucket(metaBucket).Delete([]byte(id)); err != nil {
				return err
			}
			removed = append(removed, id)
		}
		return nil
	})
	if err == nil {
		for _, id := range removed {
			delete(s.entries, id)
		}
	}
	return err
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.db.Close()
}
