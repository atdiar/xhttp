// Package localmemstore provides a string-based in-memory K/V store.
// Data is not distributed, not encrypted and not persisted on disk.
// This is for development purpose only.
package localmemstore

import (
	"encoding/json"
	"errors"
	"sync"
	"time"
)

var (
	// ErrNoID is returned when no session ID was found or the value was invalid.
	ErrNoID = errors.New("No id or Invalid id.")
	// errJSONEncoding is the error returned when the JSON (un)marshaller failed.
	errJSONEncoding = errors.New("Error in JSON Serialization/Deserialization of session data.")
	// ErrExpired is returned when the session has expired.
	ErrExpired = errors.New("Session has expired.")
	// errBadSession is returned when the session is in an invalid state.
	errBadSession = errors.New("Session is invalid.")
)

// Store is the datastructure which implements the in-memory Key/Value store.
// It is safe for concurrent use by multiple goroutines.
type Store struct {
	container map[Key][]byte

	// Measured in seconds.
	// By default, the validity of a session object has been set to 6 hours.
	// (equivalent to 21600 seconds)
	expireAfter time.Duration

	mutex *sync.RWMutex
}

// New returns a local in-memory storage datastructure initialized with defaults.
func New() Store {
	return Store{
		container:   make(map[Key][]byte),
		expireAfter: 21600 * time.Second,
		mutex:       new(sync.RWMutex),
	}
}

// DefaultExpiry enables to create a K/V store with a different setting
// regarding the default data expiry.
func (s Store) DefaultExpiry(t time.Duration) Store {
	return Store{
		container:   make(map[Key][]byte),
		expireAfter: t,
		mutex:       new(sync.RWMutex),
	}
}

// The metadata type is used to add additional info about user ownership and
// user specific expiry of stored data.
type metadata struct {
	Key     Key
	Expires time.Time //timestamp
}

func (m *metadata) toJSON() ([]byte, error) {
	return json.Marshal(m)
}

func (m *metadata) fromJSON(buf []byte) error {
	return json.Unmarshal(buf, m)
}

// Key is the key type for the default session store.
// It consists in the UserID and the entry key (hash).
//
// Key{someID,""} is used to record the additional metadata
// corresponding to a specific user ID, more specifically the answer to when
// should the data expire.
type Key struct {
	ID      string
	hashkey string
}

func userkey(id string) Key {
	return Key{id, ""}
}

// Get attempts to return the value stored for a given key under a certain user.
func (s Store) Get(id, key string) (res []byte, err error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	userkey := userkey(id)
	userdata, ok := (s.container)[userkey]

	if !ok {
		return res, errors.New("User does not exist for ID: " + id)
	}

	m := new(metadata)
	if err = m.fromJSON(userdata); err != nil {
		return res, errJSONEncoding
	}
	if m.Expires.IsZero() {
		return res, ErrExpired
	}
	if m.Expires.Before(time.Now().UTC()) {
		err = s.invalidate(id)
		if err != nil {
			return res, err
		}
		return res, ErrExpired
	}

	res, ok = (s.container)[Key{id, key}]
	if !ok {
		err = errors.New("Not found")
	}
	return res, err
}

// Put will store some value in store for a given user under a given
// key string.
func (s Store) Put(id string, key string, value []byte) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if key == "" {
		return errors.New("The empty string is not a valid key.")
	}

	// If this is the first store of data for a given user ID, some metadata to handle
	// the expiration of the key needs to be registered.
	userkey := userkey(id)
	userdata, ok := (s.container)[userkey]

	if !ok {
		newuserdata, err := (&metadata{Key: userkey, Expires: time.Now().Add(s.expireAfter).UTC()}).toJSON()
		if err != nil {
			return errors.New("Initialization of storage location failed for id: " + id)
		}
		(s.container)[userkey] = newuserdata
	} else {
		// We have to verify that the ID is still valid.
		umtdt := new(metadata)
		if err := umtdt.fromJSON(userdata); err != nil {
			return errJSONEncoding
		}
		if umtdt.Expires.IsZero() {
			return ErrExpired
		}
		if umtdt.Expires.Before(time.Now().UTC()) {
			err := s.invalidate(id)
			if err != nil {
				return err
			}
			return ErrExpired
		}
	}
	(s.container)[Key{id, key}] = value
	return nil
}

// Delete erases the stored value corresponding to the provided key for a given user.
func (s Store) Delete(id, key string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if key == "" {
		return errors.New("The empty string is not a valid key.")
	}

	delete(s.container, Key{id, key})
	return nil
}

// SetExpiry modifies the deletion timeout for a given user.
// Each entry in the key value store belongs to a given user, identified by
// a unique ID number.
// Users do not necessarily have the same data retention policy.
// This method allows to specify a new data retention time per user.
func (s Store) SetExpiry(id string, t time.Duration) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	userkey := userkey(id)

	newuserdata, err := (&metadata{Key: userkey, Expires: time.Now().UTC().Add(t)}).toJSON()
	if err != nil {
		return errors.New("Setting expiration of storage location has failed for id: " + id)
	}
	(s.container)[userkey] = newuserdata
	return nil
}

// invalidate sets the expiration date to time 0.
func (s Store) invalidate(id string) error {
	userkey := userkey(id)
	userdataExpired, err := (&metadata{Key: userkey, Expires: time.Time{}}).toJSON()
	if err != nil {
		return errJSONEncoding
	}
	s.container[userkey] = userdataExpired
	return nil
}
