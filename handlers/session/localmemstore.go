package session

/*
File description

The session package contains two files(session.go and defaultStore.go).

session.go defines a http request handler type which creates a session per
request.

localstorage.go defines a simple implementation of a session store for
development purpose. It should not be used in production.
*/

import (
	"encoding/json"
	"errors"
	"sync"
	"time"
)

// TODO
// - tests

var (
	ERRNOSID    = errors.New("Session ID not found.")
	ERRINVALID  = errors.New("Invalid session ID.")
	ERRSEXPIRED = errors.New("Expired session ID.")
	ERRMETADATA = errors.New("Incorrect Session Metadata.")
	ERRJSON     = errors.New("Error in JSON Encoding/Decoding of session data.")
)

// DefaultStore : it's a string-based in-memory K/V store.
// It's not suitable for use in production.
// Data is not persisted on disk.
// Caching is not distributed (local memory only)
// Data is not encrypted : it is left at the discretion of the client.
var DefaultStore *store

func init() {
	DefaultStore = newstore()
}

type store struct {
	container map[Key][]byte

	// Measured in seconds.
	// By default, the validity of a session object is 6 hours.
	// (equivalent to 21600 seconds)
	expireAfter int64

	mutex *sync.Mutex
}

func newstore() *store {
	return &store{
		container:   make(map[Key][]byte),
		expireAfter: 21600,
		mutex:       new(sync.Mutex),
	}
}

// The metadata type is used to add additional info about ownership and
// expiry of stored data.
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
// It consists in the UserID and the entry key (hash)
// Key{someID,""} is used to record the additional metadata
// corresponding to a specific user ID.
type Key struct {
	ID   string
	hash string
}

func baseKey(id string) Key {
	return Key{id, ""}
}

// Get will retrieve the value stored under a given key string for a given user.
func (i *store) Get(id, hkey string) (res []byte, err error) {
	i.mutex.Lock()
	defer i.mutex.Unlock()

	initKey := baseKey(id)
	initValue, ok := (i.container)[initKey]

	if !ok {
		return res, errors.New("no session registered for that id: " + id)
	}

	m := new(metadata)
	if err := m.fromJSON(initValue); err != nil {
		return res, errors.New("Unable to retrieve JSON metadata for given store ID")
	}
	if m.Expires.IsZero() {
		return res, errors.New("This session is invalid.")
	}
	if m.Expires.Before(time.Now().UTC()) {
		err := i.invalidate(id)
		if err != nil {
			return res, err
		}
		return res, errors.New("This session has expired")
	}

	res = (i.container)[Key{id, hkey}]
	return res, err
}

// Put will store some content in store for a given user under a given key string.
func (i *store) Put(id string, hkey string, content []byte) error {
	i.mutex.Lock()
	defer i.mutex.Unlock()

	if hkey == "" {
		return errors.New("Empty string is not a valid key")
	}

	// If this is the first store of data for a given user ID, some metadata to handle
	// the expiration of the key needs to be registered.
	initKey := baseKey(id)
	initValue, ok := (i.container)[initKey]

	if !ok {
		val, err := (&metadata{Key: initKey, Expires: time.Now().Add(time.Duration(i.expireAfter) * time.Second)}).toJSON()
		if err != nil {
			return errors.New("Initialization of store storage location failed for id: " + id)
		}
		(i.container)[initKey] = val
	}

	// We have to verify that the ID is still valid.
	m := new(metadata)
	if err := m.fromJSON(initValue); err != nil {
		return errors.New("Unable to retrieve JSON metadata for given cachestore ID")
	}
	if m.Expires.IsZero() {
		return errors.New("This id is invalid.")
	}
	if m.Expires.Before(time.Now().UTC()) {
		err := i.invalidate(id)
		if err != nil {
			return err
		}
		return errors.New("This id has expired")
	}

	(i.container)[Key{id, hkey}] = content
	return nil
}

// Delete erases the stored value corresponding to the provided hkey for a given user.
func (i *store) Delete(id, hkey string) error { //TODO test
	i.mutex.Lock()
	defer i.mutex.Unlock()
	delete(i.container, Key{id, hkey})
	return nil
}

// Expire sets a timeout on a key for a given user.
func (i *store) Expire(id string, t int64) error {
	i.mutex.Lock()
	defer i.mutex.Unlock()

	initKey := baseKey(id)

	val, err := (&metadata{Key: initKey, Expires: time.Now().UTC().Add(time.Duration(t) * time.Second)}).toJSON()
	if err != nil {
		return errors.New("Setting expiration of storage location has failed for id: " + id)
	}
	(i.container)[initKey] = val
	return nil
}

// invalidate sets the expiration date to time 0.
func (i *store) invalidate(id string) error {
	initKey := baseKey(id)
	deadValue, err := (&metadata{Key: initKey, Expires: time.Time{}}).toJSON()
	if err != nil {
		return errors.New("JSON encoding failure while invalidating id")
	}
	i.container[initKey] = deadValue
	return nil
}
