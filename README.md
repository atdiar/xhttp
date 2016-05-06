#xhttp

[![GoDoc](https://godoc.org/github.com/atdiar/xhttp/gddo?status.svg)](http://godoc.org/github.com/atdiar/xhttp)

##Description

This package defines a wrapper around the `*http.ServeMux` that can be found
in Go's Standard library. The goal is to provide convenience methods to
register request handlers and facilitate the usage of an execution context
per goroutine spawned (which should be the norm).

##What are the main additions compared to net/http?

The ServeMux wrapper allows to define a deadline for the completion of
subtasks that would be launched on behalf of the request handling goroutine.

Convenience methods for request handler registration are also provided.

Lastly, two new interfaces are defined:
* `xhttp.Handler`
* `xhttp.HandlerLinker`

An `xhttp.Handler` differs from the traditional http.Handler by the signature
of its ServeHTTP method: it demands to be provided with one more argument
which is an `execution.Context`.

An `xhttp.HandlerLinker` enables the linking of two Handler objects so that one
can call the other. By recursion, it allows to create a linked list (chain) of
handlers.

``` go

 // Handler ServeHTTP signature from the standard library.
ServeHTTP(w http.ResponseWriter, r *http.Request)

// Handler ServeHTTP signature from the xhttp package.
ServeHTTP(ctx execution.Context, w http.ResponseWriter, r *http.Request)

// HandlerLinker ServeHTTP and CallNext signatures from the xhttp package.
ServeHTTP(ctx execution.Context, w http.ResponseWriter, r *http.Request)
CallNext(h Handler) HandlerLinker

```

##Convenience methods

As a wrapper around `*http.ServeMux`, we have defined several methods that
should render the task of registering request handlers per route and verb easier.

Registration consists in defining a handler and using it
as argument.

``` go
s := xhttp.NewServeMux()

s.GET("/foo",someHandler)
s.PUT("/bar/", someOtherHandler)

```
where someHandler and someOtherHandler implements the Handler interface.

To register handlers that apply regardless of the request verb, the USE
variadic method, which accepts HandlerLinkers as arguments, exists :

``` go

s.USE(handlerlinkerA, handlerlinkerB, handlerlinkerC)
// Calling it again will queue another handler to the previous list
// For instance here, handlerlinkerD will be called right after
// handlerlinkerC
s.USE(handlerlinkerD)

```

##More about chaining/linking Handler objects

If a given route & request.Method requires a response to be processed
by a chain of handlers, the `xhttp.Link` function can be used to create such
a chain.

``` go
postHandler := xhttp.Link(hlinkerA, hlinkerB, hlinkerC).CallNext(Handler)
s.POST("/foobar", postHandler)
```

##Basic Handlers

The `/handler/` subfolder contains some general use request handlers
for response compression, CSRF protection, session management,...

## License
MIT
