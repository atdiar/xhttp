# csrf
[![GoDoc](https://godoc.org/github.com/atdiar/xhttp/handlers/csrf?status.svg)](https://godoc.org/github.com/atdiar/xhttp/handlers/csrf)

This package implements a request handler for incoming http requests.
It creates a browser session on the client side which provides a token that should
protect against Cross-Site Request Forgery (CSRF).

## Instantiation

An anti-CSRF request Handler relies upon a backing session-creating request handler.
The `Cookie` field holds the configuration options for the anti-CSRF session
cookie.
``` go

type Handler struct {
	Cookie  http.Cookie // anti-csrf cookie sent to client.
	Session session.Handler
	strict  bool // if true, a request is only valid if the xsrf Header is present.
	next    xhttp.Handler
}

```
The name of the anti-CSRF cookie should be different from the one used by the backing session.
Indeed the session is simply used for its server-side session data storage.

## User-Interface

### LaxMode
`LaxMode()` is a method that disables the requirements to set an anti-CSRF header. This is less secure as the protection now relies entirely on double-checking the anti-CSRF cookie value.

### Anti-CSRF value retrieval
The anti-CSRF value is stored in the context datastore during inflight request handling.
It can be retrieved via the `TokenFromCtx()` method.
This is useful for server-side rendering of html templates.

## Dependencies
This package depends on:
* [Execution Context package](https://github.com/atdiar/goroutine/execution)
* [session package](https://github.com/atdiar/xhttp/handlers/session)
* [xhttp package](https://github.com/atdiar/xhttp)

## License
MIT
