package xhttp

// This file defines a new request handler format that takes into account the
// existence of an execution context for each request handling goroutine.

import (
	"encoding/json"
	"net/http"
)

// Handler is the interface implemented by a request servicing object.
// If Handler is not also a HandlerLinker, it means that it can not call for
// further processing.
type Handler = http.Handler

// HandlerLinker is the interface of a request Handler to which we can attach
// another Handler. It enables the ServeHTTP method of the attached handler to
// be called from the ServeHTTP method of the first handler, if needed.
// The Link method returns the fully linked HandlerLinker.
type HandlerLinker interface {
	ServeHTTP(w http.ResponseWriter, r *http.Request)
	Link(Handler) HandlerLinker
}

// HandlerFunc defines a type of functions implementing the Handler interface.
type HandlerFunc = http.HandlerFunc

type handlerlinker struct {
	handler Handler
	next    Handler
}

func (h handlerlinker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handler.ServeHTTP(w, r) // NOTE if the context is changed, it needs to be reflected in r.Context()

	if h.next != nil {
		h.next.ServeHTTP(w, r)
	}
}

func (h handlerlinker) Link(ha Handler) HandlerLinker {
	h.next = ha
	return h
}

// LinkableHandler is a function that tunr an Handler into a HandlerLinker suitable for further chaining.
// If the Handler happens to modify the context object, it should make sure to
// swap the *http.Request internal context for the new updated context via the
// WithContext method.
// A LinkableHandler always uses a cancelable context whose cancellation function
// can be retrieved by using the xhttp.CancelingKey.
func LinkableHandler(h Handler) HandlerLinker {
	return handlerlinker{h, nil}
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

// WriteJSON can be used  to write a json encoded response
func WriteJSON(w http.ResponseWriter, data interface{}, statusCode int) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	return json.NewEncoder(w).Encode(data)
}
