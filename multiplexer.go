package xhttp

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/atdiar/goroutine/execution"
	"github.com/atdiar/sac"
)

// ServeMux holds the multiplexing logic of incoming http requests.
// It wraps around a net/http multiplexer.
// It facilitates the registration of request handlers.
type ServeMux struct {
	catchAll HandlerLinker
	handlers map[string]verbsHandler
	timeout  time.Duration
	*http.ServeMux
	pool *sync.Pool
}

type option func(*ServeMux)

// ChangeMux returns a configuration option for the ServeMux constructor
// which enables the choice of an alternate Muxer.
func ChangeMux(mux *http.ServeMux) func(*ServeMux) {
	return func(i *ServeMux) {
		i.ServeMux = mux
	}
}

// NewServeMux creates a new multiplexer which holds the request servicing logic.
// The mux used by default is http.DefaultServeMux.
// That can be changed by using the ChangeMux configuration option.
func NewServeMux(options ...option) ServeMux {
	sm := ServeMux{}
	sm.ServeMux = http.DefaultServeMux
	sm.handlers = make(map[string]verbsHandler)
	sm.pool = sac.Pool()

	// The below applies the options if any were passed.
	for _, opt := range options {
		opt(&sm)
	}
	return sm
}

// ServeHTTP is the request-servicing function for an object of type ServeMux.
func (sm ServeMux) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Let's get the pattern first.
	_, pattern := sm.ServeMux.Handler(req)

	// Let's check whether a handler has been registered for this pattern.
	if vh, ok := sm.handlers[pattern]; ok {

		// Let's extract the http Method and apply the handler if it exists.
		method := strings.ToUpper(req.Method)
		switch method {
		case "GET":
			S := sac.New(sm.pool)
			ctx := execution.NewContext(S)
			if sm.timeout != 0 {
				ctx = ctx.CancelAfter(execution.Timeout(sm.timeout))
			}
			defer ctx.Cancel()
			vh.get.ServeHTTP(ctx, w, req)
		case "POST":
			S := sac.New(sm.pool)
			ctx := execution.NewContext(S)
			if sm.timeout != 0 {
				ctx = ctx.CancelAfter(execution.Timeout(sm.timeout))
			}
			defer ctx.Cancel()
			vh.post.ServeHTTP(ctx, w, req)
		case "PUT":
			S := sac.New(sm.pool)
			ctx := execution.NewContext(S)
			if sm.timeout != 0 {
				ctx = ctx.CancelAfter(execution.Timeout(sm.timeout))
			}
			defer ctx.Cancel()
			vh.put.ServeHTTP(ctx, w, req)
		case "PATCH":
			S := sac.New(sm.pool)
			ctx := execution.NewContext(S)
			if sm.timeout != 0 {
				ctx = ctx.CancelAfter(execution.Timeout(sm.timeout))
			}
			defer ctx.Cancel()
			vh.patch.ServeHTTP(ctx, w, req)
		case "DELETE":
			S := sac.New(sm.pool)
			ctx := execution.NewContext(S)
			if sm.timeout != 0 {
				ctx = ctx.CancelAfter(execution.Timeout(sm.timeout))
			}
			defer ctx.Cancel()
			vh.delete.ServeHTTP(ctx, w, req)
		case "HEAD":
			S := sac.New(sm.pool)
			ctx := execution.NewContext(S)
			if sm.timeout != 0 {
				ctx = ctx.CancelAfter(execution.Timeout(sm.timeout))
			}
			defer ctx.Cancel()
			vh.head.ServeHTTP(ctx, w, req)
		case "OPTIONS":
			S := sac.New(sm.pool)
			ctx := execution.NewContext(S)
			if sm.timeout != 0 {
				ctx = ctx.CancelAfter(execution.Timeout(sm.timeout))
			}
			defer ctx.Cancel()
			vh.options.ServeHTTP(ctx, w, req)
		case "CONNECT":
			S := sac.New(sm.pool)
			ctx := execution.NewContext(S)
			if sm.timeout != 0 {
				ctx = ctx.CancelAfter(execution.Timeout(sm.timeout))
			}
			defer ctx.Cancel()
			vh.connect.ServeHTTP(ctx, w, req)
		case "TRACE":
			S := sac.New(sm.pool)
			ctx := execution.NewContext(S)
			if sm.timeout != 0 {
				ctx = ctx.CancelAfter(execution.Timeout(sm.timeout))
			}
			defer ctx.Cancel()
			vh.trace.ServeHTTP(ctx, w, req)
		default:
			http.Error(w, http.StatusText(405), 405)
		}
	} else {
		http.Error(w, http.StatusText(405), 405)
	}
}

// verbsHandler defines the request handling that is attached to each http
// verb.
type verbsHandler struct {
	get     transformationHandler
	post    transformationHandler
	put     transformationHandler
	patch   transformationHandler
	delete  transformationHandler
	head    transformationHandler
	options transformationHandler
	connect transformationHandler
	trace   transformationHandler
}

func (vh *verbsHandler) prepend(h HandlerLinker) {
	vh.get.prepend(h)
	vh.post.prepend(h)
	vh.put.prepend(h)
	vh.patch.prepend(h)
	vh.delete.prepend(h)
	vh.head.prepend(h)
	vh.options.prepend(h)
	vh.connect.prepend(h)
	vh.trace.prepend(h)
}

// transformationHandler is defined per pattern and per verb.
// This format allows for the modification of a handler. For instance, it is
// used to prepend catchall request handlers more easily.
// It implements the Handler interface.
type transformationHandler struct {
	input   Handler
	Handler // output
}

func (t *transformationHandler) register(h Handler) {
	t.input = h
	t.Handler = h
}

func (t *transformationHandler) prepend(h HandlerLinker) {
	if h != nil && t.input != nil {
		t.Handler = h.CallNext(t.input)
	} else {
		panic("Nil handlers can't be linked together.")
	}
}

// HANDLER REGISTRATION

// GET registers the request Handler for the servicing of http GET requests.
func (sm *ServeMux) GET(pattern string, h Handler) {

	if h == nil {
		panic("ERROR: Handler should not be nil.")
	}

	routehandler, ok := sm.handlers[pattern]
	if !ok {
		sm.ServeMux.Handle(pattern, sm)
	}

	routehandler.get.register(h)
	routehandler.get.prepend(sm.catchAll)
	sm.handlers[pattern] = routehandler

}

// POST registers the request Handler for the servicing of http POST requests.
func (sm *ServeMux) POST(pattern string, h Handler) {

	if h == nil {
		panic("ERROR: Handler should not be nil.")
	}

	routehandler, ok := sm.handlers[pattern]
	if !ok {
		sm.ServeMux.Handle(pattern, sm)
	}

	routehandler.post.register(h)
	routehandler.post.prepend(sm.catchAll)
	sm.handlers[pattern] = routehandler

}

// PUT registers the request Handler for the servicing of http PUT requests.
func (sm *ServeMux) PUT(pattern string, h Handler) {

	if h == nil {
		panic("ERROR: Handler should not be nil.")
	}

	routehandler, ok := sm.handlers[pattern]
	if !ok {
		sm.ServeMux.Handle(pattern, *sm)
	}

	routehandler.put.register(h)
	routehandler.put.prepend(sm.catchAll)
	sm.handlers[pattern] = routehandler

}

// PATCH registers the request Handler for the servicing of http PATCH requests.
func (sm *ServeMux) PATCH(pattern string, h Handler) {

	if h == nil {
		panic("ERROR: Handler should not be nil.")
	}

	routehandler, ok := sm.handlers[pattern]
	if !ok {
		sm.ServeMux.Handle(pattern, *sm)
	}

	routehandler.patch.register(h)
	routehandler.patch.prepend(sm.catchAll)
	sm.handlers[pattern] = routehandler

}

// DELETE registers the request Handler for the servicing of http DELETE requests.
func (sm *ServeMux) DELETE(pattern string, h Handler) {

	if h == nil {
		panic("ERROR: Handler should not be nil.")
	}

	routehandler, ok := sm.handlers[pattern]
	if !ok {
		sm.ServeMux.Handle(pattern, *sm)
	}

	routehandler.delete.register(h)
	routehandler.delete.prepend(sm.catchAll)
	sm.handlers[pattern] = routehandler

}

// HEAD registers the request Handler for the servicing of http HEAD requests.
func (sm *ServeMux) HEAD(pattern string, h Handler) {

	if h == nil {
		panic("ERROR: Handler should not be nil.")
	}

	routehandler, ok := sm.handlers[pattern]
	if !ok {
		sm.ServeMux.Handle(pattern, *sm)
	}

	routehandler.head.register(h)
	routehandler.head.prepend(sm.catchAll)
	sm.handlers[pattern] = routehandler

}

// OPTIONS registers the request Handler for the servicing of http OPTIONS requests.
func (sm *ServeMux) OPTIONS(pattern string, h Handler) {

	if h == nil {
		panic("ERROR: Handler should not be nil.")
	}

	routehandler, ok := sm.handlers[pattern]
	if !ok {
		sm.ServeMux.Handle(pattern, *sm)
	}

	routehandler.options.register(h)
	routehandler.options.prepend(sm.catchAll)
	sm.handlers[pattern] = routehandler

}

// CONNECT registers the request Handler for the servicing of http CONNECT requests.
func (sm *ServeMux) CONNECT(pattern string, h Handler) {

	if h == nil {
		panic("ERROR: Handler should not be nil.")
	}

	routehandler, ok := sm.handlers[pattern]
	if !ok {
		sm.ServeMux.Handle(pattern, *sm)
	}

	routehandler.connect.register(h)
	routehandler.connect.prepend(sm.catchAll)
	sm.handlers[pattern] = routehandler

}

// TRACE registers the request Handler for the servicing of http TRACE requests.
func (sm *ServeMux) TRACE(pattern string, h Handler) {

	if h == nil {
		panic("ERROR: Handler should not be nil.")
	}

	routehandler, ok := sm.handlers[pattern]
	if !ok {
		sm.ServeMux.Handle(pattern, *sm)
	}

	routehandler.trace.register(h)
	routehandler.trace.prepend(sm.catchAll)
	sm.handlers[pattern] = routehandler

}

// USE registers linkable request Handlers (i.e. implementing HandlerLinker)
// which shall be servicing any path, regardless of the request method.
func (sm *ServeMux) USE(handlers ...HandlerLinker) {
	sm.catchAll = Link(handlers...)
	for method, vh := range sm.handlers {
		vh.prepend(sm.catchAll)
		sm.handlers[method] = vh
	}
}

// Link is a function that is used to create a chain of Handlers when provided
// with linkable Handlers (they must implement HandlerLinker).
// It returns the first link of the chain.
func Link(handlers ...HandlerLinker) HandlerLinker {
	l := len(handlers)

	if l == 0 {
		return nil
	}

	if l > 1 {
		// Starting from the penultimate element, we link the handlers using the
		// CallNext registration method.
		for i := range handlers[:l-2] {
			h := handlers[l-2-i].CallNext(handlers[l-1-i])
			handlers[l-2-i] = h
		}
	}
	return handlers[0]
}
