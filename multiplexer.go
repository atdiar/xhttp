// Package xhttp provides a convenience wrapper around a net/http multiplexer.
// Its main goal is to provide an easier configuration of the multiplexer.
package xhttp

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// ServeMux holds the multiplexing logic of incoming http requests.
// It wraps around a net/http multiplexer.
// It facilitates the registration of request handlers.
type ServeMux struct {
	catchAll        HandlerLinker
	Once            *sync.Once
	routeHandlerMap map[string]httpVerbFunctions
	ServeMux        *http.ServeMux
	initErr         []error
}

// NewServeMux creates a new multiplexer wrapper which holds the request
// servicing logic.
// The mux wrapped by default is http.DefaultServeMux.
// That can be changed by using the ChangeMux configuration option.
func NewServeMux() ServeMux {
	sm := ServeMux{}
	sm.ServeMux = http.NewServeMux()
	sm.Once = new(sync.Once)
	sm.routeHandlerMap = make(map[string]httpVerbFunctions)
	sm.initErr = nil

	return sm
}

// ServeHTTP is the request-servicing function for an object of type ServeMux.
func (sm *ServeMux) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if sm.initErr != nil {
		var errstr string
		for _, s := range sm.initErr {
			errstr = errstr + s.Error()
		}
		panic(errstr)
	}

	// Let's check whether a handler has been registered for the path
	var longestpath string
	vh, ok := sm.routeHandlerMap[req.URL.Path]
	method := strings.ToUpper(req.Method)
	if !ok {
		for pathname, v := range sm.routeHandlerMap {
			if strings.HasSuffix(pathname, "/") {
				if strings.HasPrefix(req.URL.Path, pathname) {
					if len(pathname) > len(longestpath) {
						longestpath = pathname
						vh = v
					}
				}
			}
		}
	} else {
		longestpath = req.URL.Path
	}
	if longestpath != "" {
		// Let's extract the http Method and apply the handler if it exists.
		switch method {
		case "GET":
			sm.catchAll.Link(vh.get).ServeHTTP(req.Context(), w, req)
		case "POST":
			sm.catchAll.Link(vh.post).ServeHTTP(req.Context(), w, req)
		case "PUT":
			sm.catchAll.Link(vh.put).ServeHTTP(req.Context(), w, req)
		case "PATCH":
			sm.catchAll.Link(vh.patch).ServeHTTP(req.Context(), w, req)
		case "DELETE":
			sm.catchAll.Link(vh.delete).ServeHTTP(req.Context(), w, req)
		case "HEAD":
			sm.catchAll.Link(vh.head).ServeHTTP(req.Context(), w, req)
		case "OPTIONS":
			sm.catchAll.Link(vh.options).ServeHTTP(req.Context(), w, req)
		case "CONNECT":
			sm.catchAll.Link(vh.connect).ServeHTTP(req.Context(), w, req)
		case "TRACE":
			sm.catchAll.Link(vh.trace).ServeHTTP(req.Context(), w, req)
		default:
			http.Error(w, http.StatusText(405), 405)
		}
	}

	// todo check if a handler exists that is not http.ServeMux
	// 404

}

// httpVerbFunctions is a structure that lists the request handlers for each http
// verb.
type httpVerbFunctions struct {
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

func (vh httpVerbFunctions) prepend(h HandlerLinker) httpVerbFunctions {
	vh.get = vh.get.prepend(h)
	vh.post = vh.post.prepend(h)
	vh.put = vh.put.prepend(h)
	vh.patch = vh.patch.prepend(h)
	vh.delete = vh.delete.prepend(h)
	vh.head = vh.head.prepend(h)
	vh.options = vh.options.prepend(h)
	vh.connect = vh.connect.prepend(h)
	vh.trace = vh.trace.prepend(h)
	return vh
}

// transformableHandler is defined per pattern and per verb.
// This format allows for the modification of a handler. For instance, it is
// used to prepend catchall request handlers more easily.
// It implements the Handler interface.
type transformableHandler struct {
	in      Handler
	Handler // output
}

func (t transformableHandler) register(h Handler) transformableHandler {
	t.in = h
	t.Handler = h
	return t
}

func (t transformableHandler) prepend(h HandlerLinker) transformableHandler {
	if h != nil {
		t.Handler = h.Link(t.in)
	}
	return t
}

// HANDLER REGISTRATION

func muxCheck(sm *ServeMux, method string, pattern string, h Handler) {
	if h == nil {
		sm.initErr = append(sm.initErr, error(errors.New(method+" "+pattern+": request handler nil\n")))
		return
	}

	if pattern == "" {
		sm.initErr = append(sm.initErr, error(errors.New(method+" "+pattern+": request pattern invalid\n")))
		return
	}

	r, err := http.NewRequest(method, pattern, nil)
	if err != nil {
		sm.initErr = append(sm.initErr, error(errors.New(method+" "+pattern+": request handler nil\n")))
		return
	}
	rh, path := sm.ServeMux.Handler(r)
	if path == "" || path != pattern {
		// it means that no handler has been registered for this route on the underlying
		// ServeMux. We can thus register sm.
		sm.ServeMux.Handle(pattern, sm)
	} else {
		// A handler has already been registered. If it is sm, we can continue.
		// Otherwise, we can't.
		if han, ok := rh.(*ServeMux); !ok || (han != sm) {
			sm.initErr = append(sm.initErr, error(errors.New(method+" "+pattern+": request handler already exists\n")))
			return
		}
	}
}

// GET registers the request Handler for the servicing of http GET requests.
// It also handles HEAD requests wby creating an identical
// response to GET requests without the request body.
func (sm *ServeMux) GET(pattern string, h Handler) {
	muxCheck(sm, "GET", pattern, h)

	routehandler, _ := sm.routeHandlerMap[pattern]

	routehandler.get = routehandler.get.register(h)

	routehandler.head = routehandler.head.register(h)

	sm.routeHandlerMap[pattern] = routehandler

}

// POST registers the request Handler for the servicing of http POST requests.
func (sm *ServeMux) POST(pattern string, h Handler) {

	muxCheck(sm, "POST", pattern, h)

	routehandler, _ := sm.routeHandlerMap[pattern]

	routehandler.post = routehandler.post.register(h)

	sm.routeHandlerMap[pattern] = routehandler

}

// PUT registers the request Handler for the servicing of http PUT requests.
func (sm *ServeMux) PUT(pattern string, h Handler) {

	muxCheck(sm, "PUT", pattern, h)

	routehandler, _ := sm.routeHandlerMap[pattern]

	routehandler.put = routehandler.put.register(h)

	sm.routeHandlerMap[pattern] = routehandler

}

// PATCH registers the request Handler for the servicing of http PATCH requests.
func (sm *ServeMux) PATCH(pattern string, h Handler) {

	muxCheck(sm, "PATCH", pattern, h)

	routehandler, _ := sm.routeHandlerMap[pattern]

	routehandler.patch = routehandler.patch.register(h)

	sm.routeHandlerMap[pattern] = routehandler

}

// DELETE registers the request Handler for the servicing of http DELETE requests.
func (sm *ServeMux) DELETE(pattern string, h Handler) {

	muxCheck(sm, "DELETE", pattern, h)

	routehandler, _ := sm.routeHandlerMap[pattern]

	routehandler.delete = routehandler.delete.register(h)

	sm.routeHandlerMap[pattern] = routehandler

}

// OPTIONS registers the request Handler for the servicing of http OPTIONS requests.
func (sm *ServeMux) OPTIONS(pattern string, h Handler) {

	muxCheck(sm, "OPTIONS", pattern, h)

	routehandler, _ := sm.routeHandlerMap[pattern]

	routehandler.options = routehandler.options.register(h)

	sm.routeHandlerMap[pattern] = routehandler

}

// CONNECT registers the request Handler for the servicing of http CONNECT requests.
func (sm *ServeMux) CONNECT(h Handler) {
	pattern := "/"

	muxCheck(sm, "CONNECT", pattern, h)

	routehandler, _ := sm.routeHandlerMap[pattern]

	routehandler.connect = routehandler.connect.register(h)

	sm.routeHandlerMap[pattern] = routehandler

}

// TRACE registers the request Handler for the servicing of http TRACE requests.
func (sm *ServeMux) TRACE(pattern string, h Handler) {

	muxCheck(sm, "TRACE", pattern, h)

	routehandler, _ := sm.routeHandlerMap[pattern]

	routehandler.trace = routehandler.trace.register(h)

	sm.routeHandlerMap[pattern] = routehandler

}

// USE registers linkable request Handlers (i.e. implementing HandlerLinker)
// which shall be servicing any path, regardless of the request method.
// This function should only be called once.
func (sm *ServeMux) USE(handlers ...HandlerLinker) {
	linkable := Chain(handlers...)
	if sm.catchAll != nil {
		sm.initErr = append(sm.initErr, error(errors.New("USE has already been called once.\n")))
	} else {
		sm.catchAll = linkable
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

func (h handlerchain) ServeHTTP(ctx context.Context, res http.ResponseWriter, req *http.Request) {
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

// noopBodywriter implements http.ResponseWriter but does not allow writing
// a message-body in response to a http request. It is used to derive the
// response to a HEAD request from the response that would be returned from a
// GET request.
type noopBodywriter struct {
	http.ResponseWriter
}

func (nbw noopBodywriter) Write([]byte) (int, error) { return 200, nil }

func (nbw noopBodywriter) Wrappee() http.ResponseWriter { return nbw.ResponseWriter }

func patternMatch(url *url.URL, pattern string, vars map[string]string) bool {
	uri := url.RequestURI()
	pathsplit := strings.SplitN(uri, "/", -1)
	patternsplit := strings.SplitN(pattern, "/", -1)
	if len(pathsplit) != len(patternsplit) {
		return false
	}
	for i, str := range patternsplit {
		if str[0:1] != ":" {
			if str != pathsplit[i] {
				return false
			}
		} else {
			if vars != nil {
				vars[str[1:]] = pathsplit[i]
			}
		}
	}
	return true
}

// PathMatch allows for the retrieval of URL parameters by name when an URL
// matches a given pattern.
// For instance https://example.com/track/2589556/comments/1879545 will match
// the following pattern https://example.com/track/:tracknumber/comments/:commentnumber
// In the vars map, we will have the following key/value pairs entered:
// ("tracknumber","2589556") and ("commentnumber","1879545")
// NB Everything remains stored as strings.
// This function should be used on a path registered in the muxer as /track/
func PathMatch(req *http.Request, pattern string, vars map[string]string) bool {
	return patternMatch(req.URL, pattern, vars)
}
