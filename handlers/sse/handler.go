package sse

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/atdiar/xhttp/handlers/session"
)

type Handler struct {
	Session session.Handler

	mu       sync.Mutex
	Channels map[string]chan string
}

func New(s session.Handler) *Handler {
	return &Handler{s, sync.Mutex{}, make(map[string]chan string)}
}

func (h *Handler) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	ctx, err := h.Session.Load(ctx, w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	id, err := h.Session.ID()
	if err != nil {
		http.Error(w, "Unknown user session id. Cannot start streaming.", http.StatusInternalServerError)
	}

	h.mu.Lock()
	c, ok := h.Channels[id]
	if !ok {
		c = make(chan string)
		h.Channels[id] = c
	}
	h.mu.Unlock()

	// Make sure that the writer supports flushing.
	//
	fw, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	// Listen to the closing of the http connection via the CloseNotifier
	go func() {
		<-ctx.Done()

		// Remove the client channel of corresponding id
		h.mu.Lock()
		delete(h.Channels, id)
		h.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	for {

		// Retrieve message
		select {
		case msg := <-c:
			// Write to the ResponseWriter, `w`.
			fmt.Fprintf(w, "%s", msg)
			// Flush the response. Only possible if streaming is supported.
			fw.Flush()
		case <-ctx.Done():
			return
		}
	}
}

func (h *Handler) Broadcast(message string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id := range h.Channels {
		c, ok := h.Channels[id]
		if ok {
			c <- message
		}
	}
}

func (h *Handler) Send(chanid, message string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	c, ok := h.Channels[chanid]
	if ok {
		c <- message
	}
}

type Message struct {
	event string
	data  string
	id    string
	retry string
}

func Msg() Message {
	return Message{}
}

func (m Message) Event(name string) Message {
	if m.data != "" {
		m.event = name
	}
	return m
}

func (m Message) Data(lines ...string) Message {
	if len(lines) == 0 {
		m.data = ""
		return m
	}
	for _, l := range lines {
		m.data = "data:" + l + "\n"
	}
	return m
}

func (m Message) Id(id string) Message {
	m.id = "id:" + "\n"
	return m
}

func (m Message) Retry(n int) Message {
	m.retry = "retry:" + strconv.Itoa(n) + "\n"
	return m
}

func (m Message) End() string {
	return m.event + m.data + m.id + m.retry + "\n"
}
