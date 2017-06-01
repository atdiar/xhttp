# cors

[![GoDoc](https://godoc.org/github.com/atdiar/xhttp/handlers/cors?status.svg)](https://godoc.org/github.com/atdiar/xhttp/handlers/cors)

This package defines a handler that can be used for endpoints that serve a
resource that needs to be accessible from multiple origins.

A Cross Origin http request for a given resource is made when a user agent is
used to retrieve resources from a  given domain which themselves depend on
resources from another domain (such as an image stored on a foreign  CDN for
instance).

The current package can be used to specify the conditions under which we allow a
resource at a given endpoint to be accessed.

The default being *same-origin* policy (same domain, same protocol, same port,
same host), it can be relaxed by specifying the type of Cross Origin requests the
server allows (by Origin, by Headers, Content-type, etc.)

### Specification
[CORS Specification]

## How to use it?

A CORS Handler controls the access to resources available on the server by defining
constraints (request origin, http methods allowed, headers allowed, etc.)



```go
type Handler struct {
	Parameters
	Preflight *PreflightHandler
	next      xhttp.Handler
}
```
The `Parameter` field holds the configuration options.

```go

// Parameters defines the set of actionable components that are used to define a
// response to a Cross-Origin request.
// "*" is used to denote that anything is accepted (resp. Headers, Methods,
// Content-Types).
// The fields AllowedOrigins, AllowedHeaders, AllowedMethods, ExposeHeaders and
// AllowedContentTypes are sets of strings. A string may be inserted by using
// the `Add(str string, caseSensitive bool)` method.
// It is also possible to lookup for the existence of a string within a set
// thanks to the `Contains(str string, caseSensitive bool)` method.
type Parameters struct {
	AllowedOrigins      set
	AllowedHeaders      set
	AllowedContentTypes set
	ExposeHeaders       set
	AllowedMethods      set
	AllowCredentials    bool
}

```

Except for the case of simple requests (as defined in the spec.), a preflight request
is sent, which aims at verifying that a request is well-formed for a given endpoint, i.e.
the headers, method and origin are expected by the server.

```go
// PreflightHandler holds the elements required to build and register
// the http response logic to a preflight request.
type PreflightHandler struct {
	*Parameters
	MxAge time.Duration
	mux   *xhttp.ServeMux
	pat   string

	next xhttp.Handler
}

```
The preflight result may be cached on the user-agent and it is even possible to
pick for how long the result will stay valid in cache.
The handler is automatically registered on the OPTION method of a xhttp.ServeMux

It is likely that this handler will be registered early in the the request-handling chain.
Registration is only for an **explicitly** given path.

## Dependencies

* [Package xhttp]
* [Package execution]

These are the only two external dependencies required as they are necessary
to take into account the execution context of a request-handling goroutine.

## License

BSD 3-clause

[Package xhttp]:http://github.com/atdiar/xhttp
[Package execution]:http://github.com/atdiar/goroutine/execution
[CORS Specification]:https://www.w3.org/TR/cors/
