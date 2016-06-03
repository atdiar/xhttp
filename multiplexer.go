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
	catchAll        HandlerLinker
	routeHandlerMap map[string]verbsHandlerList
	timeout         time.Duration
	*http.ServeMux
	pool *sync.Pool
}

type option func(*ServeMux)

// ChangeMux returns a configuration option for the ServeMux constructor
// which enables the choice of an alternate Muxer.
func ChangeMux(mux *http.ServeMux) func(*ServeMux) {
	return func(s *ServeMux) {
		s.ServeMux = mux
	}
}

// SetTimeout returns a functional configuration option which provides a
// time boundary for the handling of requests under the form of a timeout.
func SetTimeout(t time.Duration) func(*ServeMux) {
	return func(sm *ServeMux) {
		sm.timeout = t
	}
}

// NewServeMux creates a new multiplexer wrapper which holds the request
// servicing logic.
// The mux wrapped by default is http.DefaultServeMux.
// That can be changed by using the ChangeMux configuration option.
func NewServeMux(options ...option) ServeMux {
	sm := ServeMux{}
	sm.ServeMux = http.DefaultServeMux
	sm.routeHandlerMap = make(map[string]verbsHandlerList)
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
	if vh, ok := sm.routeHandlerMap[pattern]; ok {

		// Let's extract the http Method and apply the handler if it exists.
		method := strings.ToUpper(req.Method)
		switch method {
		case "GET":
			// Let's create the datastore and then the execution context
			rawS := sm.pool.Get()
			S := rawS.(*sac.Instance)
			ctx := execution.NewContext(S)
			if sm.timeout != 0 {
				ctx = ctx.CancelAfter(execution.Timeout(sm.timeout))
			}
			// Let's handle the request
			vh.get.ServeHTTP(ctx, w, req)
			// Let's cleanup
			ctx.Cancel()
			S.Clear()
			sm.pool.Put(S)

		case "POST":
			// Let's create the datastore and then the execution context
			rawS := sm.pool.Get()
			S := rawS.(*sac.Instance)
			ctx := execution.NewContext(S)
			if sm.timeout != 0 {
				ctx = ctx.CancelAfter(execution.Timeout(sm.timeout))
			}
			// Let's handle the request
			vh.post.ServeHTTP(ctx, w, req)
			// Let's cleanup
			ctx.Cancel()
			S.Clear()
			sm.pool.Put(S)

		case "PUT":
			// Let's create the datastore and then the execution context
			rawS := sm.pool.Get()
			S := rawS.(*sac.Instance)
			ctx := execution.NewContext(S)
			if sm.timeout != 0 {
				ctx = ctx.CancelAfter(execution.Timeout(sm.timeout))
			}
			// Let's handle the request
			vh.put.ServeHTTP(ctx, w, req)
			// Let's cleanup
			ctx.Cancel()
			S.Clear()
			sm.pool.Put(S)

		case "PATCH":
			// Let's create the datastore and then the execution context
			rawS := sm.pool.Get()
			S := rawS.(*sac.Instance)
			ctx := execution.NewContext(S)
			if sm.timeout != 0 {
				ctx = ctx.CancelAfter(execution.Timeout(sm.timeout))
			}
			// Let's handle the request
			vh.patch.ServeHTTP(ctx, w, req)
			// Let's cleanup
			ctx.Cancel()
			S.Clear()
			sm.pool.Put(S)

		case "DELETE":
			// Let's create the datastore and then the execution context
			rawS := sm.pool.Get()
			S := rawS.(*sac.Instance)
			ctx := execution.NewContext(S)
			if sm.timeout != 0 {
				ctx = ctx.CancelAfter(execution.Timeout(sm.timeout))
			}
			// Let's handle the request
			vh.delete.ServeHTTP(ctx, w, req)
			// Let's cleanup
			ctx.Cancel()
			S.Clear()
			sm.pool.Put(S)

		case "HEAD":
			// Let's create the datastore and then the execution context
			rawS := sm.pool.Get()
			S := rawS.(*sac.Instance)
			ctx := execution.NewContext(S)
			if sm.timeout != 0 {
				ctx = ctx.CancelAfter(execution.Timeout(sm.timeout))
			}
			// Let's handle the request
			vh.head.ServeHTTP(ctx, w, req)
			// Let's cleanup
			ctx.Cancel()
			S.Clear()
			sm.pool.Put(S)

		case "OPTIONS":
			// Let's create the datastore and then the execution context
			rawS := sm.pool.Get()
			S := rawS.(*sac.Instance)
			ctx := execution.NewContext(S)
			if sm.timeout != 0 {
				ctx = ctx.CancelAfter(execution.Timeout(sm.timeout))
			}
			// Let's handle the request
			vh.options.ServeHTTP(ctx, w, req)
			// Let's cleanup
			ctx.Cancel()
			S.Clear()
			sm.pool.Put(S)

		case "CONNECT":
			// Let's create the datastore and then the execution context
			rawS := sm.pool.Get()
			S := rawS.(*sac.Instance)
			ctx := execution.NewContext(S)
			if sm.timeout != 0 {
				ctx = ctx.CancelAfter(execution.Timeout(sm.timeout))
			}
			// Let's handle the request
			vh.connect.ServeHTTP(ctx, w, req)
			// Let's cleanup
			ctx.Cancel()
			S.Clear()
			sm.pool.Put(S)

		case "TRACE":
			// Let's create the datastore and then the execution context
			rawS := sm.pool.Get()
			S := rawS.(*sac.Instance)
			ctx := execution.NewContext(S)
			if sm.timeout != 0 {
				ctx = ctx.CancelAfter(execution.Timeout(sm.timeout))
			}
			// Let's handle the request
			vh.trace.ServeHTTP(ctx, w, req)
			// Let's cleanup
			ctx.Cancel()
			S.Clear()
			sm.pool.Put(S)
		default:
			http.Error(w, http.StatusText(405), 405)
		}
	} else {
		// If nothing was registered by any other entity, h will default to
		// a page not found handler (404)
		h, _ := sm.ServeMux.Handler(req)
		h.ServeHTTP(w, req)
	}
}

// verbsHandlerList is a structure that lists the request handlers for each http
// verb.
type verbsHandlerList struct {
	get     transformableHandler
	post    transformableHandler
	put     transformableHandler
	patch   transformableHandler
	delete  transformableHandler
	head    transformableHandler
	options transformableHandler
	connect transformableHandler
	trace   transformableHandler
}

func (vh *verbsHandlerList) prepend(h HandlerLinker) {
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

// transformableHandler is defined per pattern and per verb.
// This format allows for the modification of a handler. For instance, it is
// used to prepend catchall request handlers more easily.
// It implements the Handler interface.
type transformableHandler struct {
	input   Handler
	Handler // output
}

func (t *transformableHandler) register(h Handler) {
	t.input = h
	t.Handler = h
}

func (t *transformableHandler) prepend(h HandlerLinker) {
	if h != nil {
		t.Handler = h.Link(t.input)
	}
}

// HANDLER REGISTRATION

// GET registers the request Handler for the servicing of http GET requests.
func (sm *ServeMux) GET(pattern string, h Handler) {

	if h == nil {
		panic("ERROR: Handler should not be nil.")
	}

	routehandler, ok := sm.routeHandlerMap[pattern]
	if !ok {
		sm.ServeMux.Handle(pattern, sm)
	}
	routehandler.get.register(h)
	routehandler.get.prepend(sm.catchAll)
	sm.routeHandlerMap[pattern] = routehandler
}

// POST registers the request Handler for the servicing of http POST requests.
func (sm *ServeMux) POST(pattern string, h Handler) {

	if h == nil {
		panic("ERROR: Handler should not be nil.")
	}

	routehandler, ok := sm.routeHandlerMap[pattern]
	if !ok {
		sm.ServeMux.Handle(pattern, sm)
	}

	routehandler.post.register(h)
	routehandler.post.prepend(sm.catchAll)
	sm.routeHandlerMap[pattern] = routehandler

}

// PUT registers the request Handler for the servicing of http PUT requests.
func (sm *ServeMux) PUT(pattern string, h Handler) {

	if h == nil {
		panic("ERROR: Handler should not be nil.")
	}

	routehandler, ok := sm.routeHandlerMap[pattern]
	if !ok {
		sm.ServeMux.Handle(pattern, *sm)
	}

	routehandler.put.register(h)
	routehandler.put.prepend(sm.catchAll)
	sm.routeHandlerMap[pattern] = routehandler

}

// PATCH registers the request Handler for the servicing of http PATCH requests.
func (sm *ServeMux) PATCH(pattern string, h Handler) {

	if h == nil {
		panic("ERROR: Handler should not be nil.")
	}

	routehandler, ok := sm.routeHandlerMap[pattern]
	if !ok {
		sm.ServeMux.Handle(pattern, *sm)
	}

	routehandler.patch.register(h)
	routehandler.patch.prepend(sm.catchAll)
	sm.routeHandlerMap[pattern] = routehandler

}

// DELETE registers the request Handler for the servicing of http DELETE requests.
func (sm *ServeMux) DELETE(pattern string, h Handler) {

	if h == nil {
		panic("ERROR: Handler should not be nil.")
	}

	routehandler, ok := sm.routeHandlerMap[pattern]
	if !ok {
		sm.ServeMux.Handle(pattern, *sm)
	}

	routehandler.delete.register(h)
	routehandler.delete.prepend(sm.catchAll)
	sm.routeHandlerMap[pattern] = routehandler

}

// HEAD registers the request Handler for the servicing of http HEAD requests.
func (sm *ServeMux) HEAD(pattern string, h Handler) {

	if h == nil {
		panic("ERROR: Handler should not be nil.")
	}

	routehandler, ok := sm.routeHandlerMap[pattern]
	if !ok {
		sm.ServeMux.Handle(pattern, *sm)
	}

	routehandler.head.register(h)
	routehandler.head.prepend(sm.catchAll)
	sm.routeHandlerMap[pattern] = routehandler

}

// OPTIONS registers the request Handler for the servicing of http OPTIONS requests.
func (sm *ServeMux) OPTIONS(pattern string, h Handler) {

	if h == nil {
		panic("ERROR: Handler should not be nil.")
	}

	routehandler, ok := sm.routeHandlerMap[pattern]
	if !ok {
		sm.ServeMux.Handle(pattern, *sm)
	}

	routehandler.options.register(h)
	routehandler.options.prepend(sm.catchAll)
	sm.routeHandlerMap[pattern] = routehandler

}

// CONNECT registers the request Handler for the servicing of http CONNECT requests.
func (sm *ServeMux) CONNECT(pattern string, h Handler) {

	if h == nil {
		panic("ERROR: Handler should not be nil.")
	}

	routehandler, ok := sm.routeHandlerMap[pattern]
	if !ok {
		sm.ServeMux.Handle(pattern, *sm)
	}

	routehandler.connect.register(h)
	routehandler.connect.prepend(sm.catchAll)
	sm.routeHandlerMap[pattern] = routehandler

}

// TRACE registers the request Handler for the servicing of http TRACE requests.
func (sm *ServeMux) TRACE(pattern string, h Handler) {

	if h == nil {
		panic("ERROR: Handler should not be nil.")
	}

	routehandler, ok := sm.routeHandlerMap[pattern]
	if !ok {
		sm.ServeMux.Handle(pattern, *sm)
	}

	routehandler.trace.register(h)
	routehandler.trace.prepend(sm.catchAll)
	sm.routeHandlerMap[pattern] = routehandler

}

// USE registers linkable request Handlers (i.e. implementing HandlerLinker)
// which shall be servicing any path, regardless of the request method.
func (sm *ServeMux) USE(handlers ...HandlerLinker) {
	linkable := Chain(handlers...)
	if sm.catchAll != nil {
		ca := sm.catchAll.Link(linkable)
		sm.catchAll = ca
	} else {
		sm.catchAll = linkable
	}
	for method, vh := range sm.routeHandlerMap {
		vh.prepend(sm.catchAll)
		sm.routeHandlerMap[method] = vh
	}
}

// Chain is a function that is used to create a chain of Handlers when provided
// with linkable Handlers (they must implement HandlerLinker).
// It returns the first link of the chain.
func Chain(handlers ...HandlerLinker) HandlerLinker {
	l := len(handlers)

	if l == 0 {
		return nil
	}

	if l > 1 {
		// Starting from the penultimate element, we link the handlers using the
		// Link registration method.
		for i := range handlers[:l-1] {
			h := handlers[l-2-i].Link(handlers[l-1-i])
			handlers[l-2-i] = h
		}
	}
	return handlerchain(handlers)
}

type handlerchain []HandlerLinker

func (h handlerchain) ServeHTTP(ctx execution.Context, res http.ResponseWriter, req *http.Request) {
	h[0].ServeHTTP(ctx, res, req)
}

func (h handlerchain) Link(l Handler) HandlerLinker {
	length := len(h)
	if length == 0 {
		panic("Linking to nothing is impossible.")
	}
	nh := h[length-1].Link(l)
	h[length-1] = nh

	if length > 1 {
		// Starting from the penultimate element, we link the handlers using the
		// Link registration method.
		for i := range h[:length-1] {
			nh := h[length-2-i].Link(h[length-1-i])
			h[length-2-i] = nh
		}
	}
	return h
}
