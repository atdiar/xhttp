# session

This package defines a request handler for http requests that implements user sessions.
A session represents data that are identified as belonging to a particular web user.
A session can be stored on the client or the server.
This package defines the interface that server-side session storage should implement.

## Configuration

A session request handler requires to specify:
* a name
* a validity limit

These parameters can be set on the handler's Cookie field which is also used to keep the settings of the session cookie that will be sent to the client.
Note that one may also want to provide a value for the Path, HttpOnly,Secure,MaxAge etc. fields of the session cookie.

Other than that, a session requires to specify :
* server side storage emplacement

Optionally, a data caching facility can be specified to improve response speed.

## User-Interface

## Methods

A session handler displays three types of methods:

* configuration methods that returns a new session handler with the correct setting each time
* session management methods that enable to load/save/renew a session
* data management methods to add/retrieve session data to a given session

The main exported methods are:

``` go

// Get will retrieve the value corresponding to a given store key from
// the session store.
func (h Handler) Get(key string) ([]byte, error) 
  
// Put will save a key/value pair in the session store.
func (h Handler) Put(key string, value []byte) error 
  
// Delete will erase a session store item.
func (h Handler) Delete(key string) error 
  
// SetExpiry sets the duration of a single user session.
// On the server-side, the duration is set on the session storage.
// On the client-side, the duration is set on the client cookie.
//
// Hence, this function mutates session data in a way that should eventually
// be visible to the client.
//
// The rules are the following:
// * if t <0, the session expires immediately.
// * if t = 0, the session expires when the browser is closed. (browser session)
// * if t > 0, the session expires after t seconds.
func (h Handler) SetExpiry(t time.Duration) (Handler, error)  
  
// Load will try to recover the session handler state if it was previously
// handled. Otherwise, it will try loading the metadata directly from the request
// object if it exists. If none works, an error is returned.
// Not safe for concurrent use by multiple goroutines. (Would not make sense)
func (h *Handler) Load(ctx execution.Context, res http.ResponseWriter, req *http.Request) error  
  
// Save will keep the session handler state in the per-request context store.
// It needs to be called to apply changes due to a session reconfiguration.
// Not safe for concurrent use by multiple goroutines.
func (h *Handler) Save(ctx execution.Context, res http.ResponseWriter, req *http.Request)  
  
// Renew will regenerate the session with a brand new session id.
// This is the method to call when loading the session failed, for instance.
func (h *Handler) Renew(ctx execution.Context, res http.ResponseWriter, req *http.Request) 

```

### Session store

A session store shall implement the Store interface:

``` go 
// Store defines the interface that a session store should implement.
// It should be made safe for concurrent use by multiple goroutines.
//
// NOTE: Expire sets a timeout for the validity of a session.
// if t = 0, the session should expire immediately.
// if t < 0, the session does not expire.
type Store interface {
	Get(id, hkey string) (res []byte, err error)
	Put(id string, hkey string, content []byte) error
	Delete(id, hkey string) error
	SetExpiry(id string, t time.Duration) error
}

```

An example of session store is the one provided by the function ` TestStore()`
This is a basic in-memory, non-distributed key/value store that runs within the same app instance and is useful for 
development purposes only.

### Data Cache

A data cache can be provided. The only requirement is that it implements the below interface:

``` go 
// Cache defines the interface that a session cache should implement.
// It should be made safe for concurrent use by multiple goroutines.
type Cache interface {
	Get(id, hkey string) (res []byte, err error)
	Put(id string, hkey string, content []byte) error
	Delete(id, hkey string) error
	Clear()
}
```
## Dependencies
This package depends on:
* [Execution Context package](https://github.com/atdiar/goroutine/execution)
* [xhttp package](https://github.com/atdiar/xhttp)

## License
MIT
