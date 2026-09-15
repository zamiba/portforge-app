package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// emit delivers a backend event to whichever frontend is attached. In the
// desktop build that is the Wails runtime; under -server it is the SSE hub,
// which App reaches through the events field rather than by knowing about it.
//
// Every backend event goes through here, so the two modes cannot drift: an
// event added for the desktop window shows up in the browser for free.
func (a *App) emit(name string, data interface{}) {
	if a.events != nil {
		a.events(name, data)
		return
	}
	wailsruntime.EventsEmit(a.ctx, name, data)
}

// sseEvent is one message on the wire. Data is a list because Wails' EventsEmit
// is variadic on the JS side and the shim spreads it back into the callback;
// keeping the shape identical is what lets the frontend stay unmodified.
type sseEvent struct {
	// Seq numbers every event the hub has ever emitted, from 1. It is written as
	// the SSE id: line, which is what the browser hands back as Last-Event-ID
	// when it reconnects, and so is the cursor a resumed stream continues from.
	Seq  uint64        `json:"seq"`
	Name string        `json:"name"`
	Data []interface{} `json:"data"`
}

// eventHub fans backend events out to every connected browser tab. More than one
// may be open at a time, and each gets its own queue.
type eventHub struct {
	mu      sync.Mutex
	clients map[chan sseEvent]struct{}
	seq     uint64
	// replay holds the newest replayDepth events so a browser that drops its
	// connection can be handed what it missed when it comes back. EventSource
	// reconnects by itself and resends the last id: it saw, so the whole
	// resume path is: keep a little history, and honour that header.
	replay []sseEvent
}

func newEventHub() *eventHub {
	return &eventHub{clients: make(map[chan sseEvent]struct{})}
}

// eventQueueDepth is deliberately generous: a build emits install:log for every
// line a compiler writes, and a browser that stalls for a moment should not lose
// the middle of a transcript.
const eventQueueDepth = 1024

// replayDepth bounds the history kept for reconnecting clients. It is a real
// bound, not a formality: a tab gone for longer than this many events loses
// the overflow, so it is sized to the same burst the queue is — a compiler's
// worth of install:log lines during a momentary stall, not an afternoon away.
// A reloaded page does not use it at all; it refetches state on mount.
const replayDepth = 1024

func (h *eventHub) remove(ch chan sseEvent) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
	close(ch)
}

// subscribe registers a client and returns, in one locked step, the events
// still in the ring with Seq > since. Zero means live-only: a fresh page wants
// nothing it did not see happen.
func (h *eventHub) subscribe(since uint64) (chan sseEvent, []sseEvent) {
	ch := make(chan sseEvent, eventQueueDepth)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[ch] = struct{}{}
	if since == 0 {
		return ch, nil
	}
	var missed []sseEvent
	for _, ev := range h.replay {
		if ev.Seq > since {
			missed = append(missed, ev)
		}
	}
	return ch, missed
}

// resumeFrom reads the client's resume cursor. The header is what EventSource
// sends by itself on reconnect; the query parameter exists because EventSource
// cannot set headers on a first connection, so a caller that has kept the seq
// some other way still has a route to it. Malformed values mean live-only.
func resumeFrom(r *http.Request) uint64 {
	v := r.Header.Get("Last-Event-ID")
	if v == "" {
		v = r.URL.Query().Get("since")
	}
	since, _ := strconv.ParseUint(v, 10, 64)
	return since
}

// writeEvent emits one SSE frame: the id: line that makes resumption work, then
// the envelope. Unnamed on purpose — the shim listens on onmessage, and a named
// event: field would route around it. Reports false once the client has gone.
func writeEvent(w http.ResponseWriter, enc *json.Encoder, ev sseEvent) bool {
	if _, err := w.Write([]byte("id: " + strconv.FormatUint(ev.Seq, 10) + "\ndata: ")); err != nil {
		return false
	}
	if err := enc.Encode(ev); err != nil { // Encode writes the trailing newline
		return false
	}
	_, err := w.Write([]byte("\n"))
	return err == nil
}

// broadcast never blocks. A build must not stall because a browser tab stopped
// reading, so a client whose queue is full loses the event rather than holding
// up the run that produced it.
func (h *eventHub) broadcast(name string, data interface{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq++
	ev := sseEvent{Seq: h.seq, Name: name, Data: []interface{}{data}}
	h.replay = append(h.replay, ev)
	if len(h.replay) > replayDepth {
		h.replay = h.replay[len(h.replay)-replayDepth:]
	}
	for ch := range h.clients {
		select {
		case ch <- ev:
		default:
		}
	}
}

// ServeHTTP streams events to one browser tab for as long as it stays connected.
func (h *eventHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Proxies that buffer would defeat the point of streaming.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	enc := json.NewEncoder(w)
	ch, missed := h.subscribe(resumeFrom(r))
	defer h.remove(ch)

	// Catch-up before live: anything still in the ring that the client had not
	// seen when it dropped. The ring is snapshotted under the same lock that
	// registers the channel, so no event can fall between the two.
	for _, ev := range missed {
		if !writeEvent(w, enc, ev) {
			return
		}
	}
	flusher.Flush()

	// A comment line keeps the connection from being reaped by an idle timeout
	// during the long quiet stretches between installs.
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			if !writeEvent(w, enc, ev) {
				return
			}
			flusher.Flush()
		case <-ping.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
