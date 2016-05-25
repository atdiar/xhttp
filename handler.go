package xhttp

// This file defines a new request handler format that takes into account the
// existence of an execution context for each request handling goroutine.

import (
	"net/http"

	"github.com/atdiar/goroutine/execution"
)

// Handler is the interface implemented by a request servicing object.
// If Handler is not also a HandlerLinker, it means that it can not call for
// further processing.
type Handler interface {
	ServeHTTP(ctx execution.Context, w http.ResponseWriter, r *http.Request)
}

// HandlerLinker is the interface of a request Handler to which we can attach
// another Handler. It enables the ServeHTTP method of the attached handler to
// be called from the ServeHTTP method of the first handler, if needed.
// The Link method returns the fully linked HandlerLinker.
type HandlerLinker interface {
	ServeHTTP(ctx execution.Context, w http.ResponseWriter, r *http.Request)
	Link(Handler) HandlerLinker
}

// HandlerFunc defines a type of functions implementing the Handler interface.
type HandlerFunc func(execution.Context, http.ResponseWriter, *http.Request)

func (f HandlerFunc) ServeHTTP(ctx execution.Context, w http.ResponseWriter, r *http.Request) {
	f(ctx, w, r)
}

/*
Example of a HandlerLinker construct:

type Handler struct{
	fieldA type A
	fieldB typeB
	.
	.
	.
	next Handler  // this is where the next handler will be registered by CallNext
}

The ServeHTTP method for this Handler can then call the next Handler if one has
been registered.
*/
