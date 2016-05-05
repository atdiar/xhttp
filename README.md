#xhttp

[![GoDoc](https://godoc.org/github.com/atdiar/xhttp/gddo?status.svg)](http://godoc.org/github.com/atdiar/xhttp)

##Description

This package defines a wrapper around `*http.ServeMux` that can be found
in Go's Standard library. The goal is to provide convenience methods to
register request handlers and facilitate the usage of an execution context
per goroutine spawned (whcih should be the norm).

##What are the main additions compared to net/http?

The ServeMux wrapper allows to define a deadline for the completion of
subtasks that would be launched on behalf of the request handling goroutine.

Convenience methods for request handler registration are also provided.

Lastly, two new interfaces are defined:
* `xhttp.Handler`
* `xhttp.HandlerLinker` (linkable request Handler a.k.a. middleware)

An xhttp.Handler differs from the traditional http.Handler by the signature
of its ServeHTTP method: it demands to be provided with one more argument
which is an `execution.Context`.

``` go
// NOTE: this is pseudo go code to illustrate the method signatures.
// It does not compile.
import(
  "github.com/atdiar/goroutine/execution"
  "github.com/atdiar/xhttp"
  "net/http"
)
 // From the standard library
func (h http.Handler) ServeHTTP(w http.ResponseWriter, r *http.Request)

// From the xhttp package
func (h xhttp.Handler) ServeHTTP(ctx execution.Context, w http.ResponseWriter, r *http.Request)

```
It also defines HandlerLinker: an interface implemented by Handler types
that are linkable.
By linkable, we mean that these request handlers can be used to form a
chain through which a request can be processed.

``` go
// From the xhttp package
func (hl xhttp.Handlerlinker) ServeHTTP(ctx execution.Context, w http.ResponseWriter, r *http.Request)
// CallNext is used to register the successor Handler and returns
// the result of the linking. Implementing HandlerLinker is implementing Handler.
func (hl xhttp.Handlerlinker) CallNext(n Handler) HandlerLinker
```

##Convenience methods

As a wrapper around `*http.ServeMux`, we have defined several methods that should
render the task of registering request handlers per route or verb easier.

Typically, registration is as simple as defining a handler and using it
as argument to one of such methods :
``` go
s := xhttp.NewServeMux()

s.GET("/foo",someHandler)
```
where someHandler implements the Handler interface.

To register handlers that apply regardless of the request verb, the USE
variadic method, which accepts linkable Handlers, exists :

``` go
s.USE(handlerlinkerA,handlerlinkerB,handlerlinkerC,...)
// Calling it again will queue another handler to the previous list
// For instance here, handlerlinkerD will be called right after
// handlerlinkerC
s.USE(handlerlinkerD)
```

##More about chaining/linking Handler objects

If a given route & request.Method requires a response to be processed
by a chain of handlers, the 'xhttp.Link' function should be used.
It allows to define the resulting handler as such :

``` go
reqHandler := xhttp.Link(hlinkerA, hlinkerB, hlinkerC).CallNext(Handler)
```

##Basic Handlers

The `/handler/` subfolder contains some general use request handlers
for response compression, CSRF protection, session management,...

## License
MIT
