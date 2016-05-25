package session

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

// data is a type for the information that will eventually be stored inside
// a session cookie.
type data struct {
	ID       string
	ExpireOn time.Time

	// Value will be stored in the session cookie.
	// Serialization and encryption is left at the
	// behest of the client.
	Value string

	delimiter   string
	needsUpdate bool
	mu          *sync.Mutex
}

func newToken() data {
	return data{
		// the delimiter should be sendable via cookie.
		// It can't belong to the base64 list of accepted sigils.
		delimiter:   ":",
		needsUpdate: true,
		mu:          new(sync.Mutex),
	}
}

// Retrieve retrieves the session data.
func (session *data) Retrieve() data {
	session.mu.Lock()
	d := *session
	session.mu.Unlock()
	return d
}

// GetID returns the session ID.
func (session *data) GetID() string {
	session.mu.Lock()
	i := session.ID
	session.mu.Unlock()
	return i
}

// SetID changes the session ID.
func (session *data) SetID(s string) {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.ID = s
	session.needsUpdate = true
}

// GetExpiry returns the validity limit for a session.
func (session *data) GetExpiry() time.Time {
	e := session.ExpireOn
	return e
}

// SetExpiry changes the validity limit for a session.
func (session *data) SetExpiry(t time.Time) {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.ExpireOn = t
	session.needsUpdate = true
}

// IsUpdated returns the status of a session. i.e. whether the client and the
// server session information are synchronized.
func (session *data) IsUpdated() bool {
	u := session.needsUpdate
	return u
}

// Update notifies about the synchronization status between the client and the server
// session.
func (session *data) Update(b bool) {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.needsUpdate = b
}

// Encode is used to serialize the session data into a string format that can be stored
// into a session cookie.
func (session *data) Encode(secret string) string {
	j, err := json.Marshal(session)
	if err != nil {
		panic("JSON encoding internal failure. Exceptional behaviour while encoding session metadata.")
	}
	return computeHmac256(j, []byte(secret)) + session.delimiter + base64.StdEncoding.EncodeToString(j)
}

// Decode is used to deserialize the session cookie in order to make the stored
// session data accessible.
// If we detect that the client has tampered with the session cookie somehow,
// an error is returned.
func (session *data) Decode(metadata string, secret string) error {
	// let's split the two components on the string-marshalled metadata (raw + Encoded)
	s := strings.Split(secret, session.delimiter)
	if len(s) <= 1 || len(s) > 4096 {
		return ErrBadCookie
	}

	ok, err := verifySignature(s[1], s[0], secret)
	if !ok {
		return ErrBadSession
	}
	str, err := base64.StdEncoding.DecodeString(s[1])
	if err != nil {
		return err
	}

	err = json.Unmarshal(str, session)
	return err
}

// AddValue allows the storage of session data onto the client.
// To be used with care.
// The responsibility of making sure the data is cryptographically secure is
// at the behest of the client of this package.
// Likewise, the max size for a cookie is 4Kb while a base64 string max size is
// 48k. The client may want to do its own sanitizing checks.
func (session *data) AddValue(str string) {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.Value = str
	session.needsUpdate = true
}
