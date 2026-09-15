package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// post calls one bound method the way the shim does.
func post(t *testing.T, h http.Handler, method, body string) (int, rpcResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/"+method, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out rpcResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s: response was not JSON: %v (%s)", method, err, rec.Body.String())
	}
	return rec.Code, out
}

func TestRPCDispatchesBoundMethods(t *testing.T) {
	h := rpcHandler(NewApp())

	code, res := post(t, h, "GetPlatform", "[]")
	if code != http.StatusOK || res.Error != "" {
		t.Fatalf("GetPlatform: got %d %q", code, res.Error)
	}
	// The platform string is what install specs match on, so an empty one would
	// mean every build silently fails to resolve.
	if s, _ := res.Result.(string); s == "" {
		t.Fatalf("GetPlatform returned no platform: %#v", res.Result)
	}
}

func TestRPCRejectsBadCalls(t *testing.T) {
	h := rpcHandler(NewApp())

	if code, res := post(t, h, "NoSuchMethod", "[]"); code != http.StatusNotFound || res.Error == "" {
		t.Errorf("unknown method: got %d %q, want 404 with an error", code, res.Error)
	}
	if code, res := post(t, h, "GetPlatform", "[1,2]"); code != http.StatusBadRequest || res.Error == "" {
		t.Errorf("wrong arity: got %d %q, want 400 with an error", code, res.Error)
	}
	if code, res := post(t, h, "GetPlatform", "not json"); code != http.StatusBadRequest || res.Error == "" {
		t.Errorf("malformed body: got %d %q, want 400 with an error", code, res.Error)
	}

	// Only exported methods are reachable: reflect cannot address the rest, and
	// the browser must not be able to drive internals like startup.
	if code, _ := post(t, h, "startup", "[]"); code != http.StatusNotFound {
		t.Errorf("unexported method was reachable: got %d, want 404", code)
	}

	req := httptest.NewRequest(http.MethodGet, "/GetPlatform", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET on an RPC path: got %d, want 405", rec.Code)
	}
}

// A method's error return is a normal outcome the UI renders, not a transport
// failure, so it comes back as 200 with the message in the body.
func TestRPCReturnsMethodErrorInBody(t *testing.T) {
	app := NewApp()
	app.events = func(string, interface{}) {} // no native window, as under -server
	code, res := post(t, rpcHandler(app), "AddStorageUnit", "[]")
	if code != http.StatusOK {
		t.Fatalf("got %d, want 200", code)
	}
	if res.Error == "" {
		t.Fatal("a method that failed reported no error")
	}
}

func TestEmitPrefersTheAttachedFrontend(t *testing.T) {
	var gotName string
	var gotData interface{}
	app := NewApp()
	app.events = func(name string, data interface{}) { gotName, gotData = name, data }

	app.emit("install:progress", map[string]interface{}{"percent": 42})

	if gotName != "install:progress" {
		t.Errorf("event name = %q", gotName)
	}
	if m, ok := gotData.(map[string]interface{}); !ok || m["percent"] != 42 {
		t.Errorf("event payload = %#v", gotData)
	}
}

func TestEventHubStreamsToConnectedClients(t *testing.T) {
	hub := newEventHub()
	srv := httptest.NewServer(hub)
	defer srv.Close()

	res, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q", ct)
	}

	// The client registers after its request is served, so keep emitting until
	// one gets through rather than racing the subscription.
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-done:
				return
			default:
			}
			hub.broadcast("install:step", map[string]interface{}{"label": "Fetching"})
			time.Sleep(5 * time.Millisecond)
		}
	}()

	scanner := bufio.NewScanner(res.Body)
	deadline := time.Now().Add(5 * time.Second)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			if time.Now().After(deadline) {
				t.Fatal("no event arrived within the deadline")
			}
			continue
		}
		var ev sseEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
			t.Fatalf("event was not JSON: %v (%s)", err, line)
		}
		if ev.Name != "install:step" {
			t.Fatalf("event name = %q", ev.Name)
		}
		// The payload is a list because the shim spreads it into the callback,
		// matching what Wails' EventsEmit delivers.
		if len(ev.Data) != 1 {
			t.Fatalf("payload = %#v, want exactly one argument", ev.Data)
		}
		return
	}
	t.Fatal("the stream closed before delivering an event")
}

// A full queue must not stall the build that is producing the events.
func TestBroadcastDoesNotBlockOnAStalledClient(t *testing.T) {
	hub := newEventHub()
	hub.subscribe(0) // never read from

	done := make(chan struct{})
	go func() {
		for i := 0; i < eventQueueDepth*2; i++ {
			hub.broadcast("install:log", map[string]interface{}{"line": "x"})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("broadcast blocked on a client that stopped reading")
	}
}

func TestIndexGetsTheShimBeforeTheAppBundle(t *testing.T) {
	dist := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(
			`<!DOCTYPE html><html><head><script type="module" src="/assets/index.js"></script></head><body></body></html>`)},
	}
	out, err := indexWithShim(dist)
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)
	shim := strings.Index(html, "/wails-shim.js")
	bundle := strings.Index(html, "/assets/index.js")
	if shim < 0 {
		t.Fatal("the shim was not injected")
	}
	// The generated bindings read window.go at call time, so the shim has to be
	// evaluated before the bundle that calls them.
	if shim > bundle {
		t.Fatal("the shim was injected after the app bundle")
	}
}

func TestIndexWithShimReportsAMissingHead(t *testing.T) {
	dist := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte(`<html><body></body></html>`)}}
	if _, err := indexWithShim(dist); err == nil {
		t.Fatal("expected an error when there is nowhere to inject the shim")
	}
}

// DNS rebinding: a page on the open web points a name it controls at 127.0.0.1
// and talks to this server through it. It cannot forge the Host header, so the
// Host is what gives it away.
func TestLocalOnlyRejectsDomainHosts(t *testing.T) {
	h := localOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	allowed := []string{"localhost:34116", "127.0.0.1:34116", "192.168.1.10:34116", "[::1]:34116"}
	for _, host := range allowed {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("host %q: got %d, want 200", host, rec.Code)
		}
	}

	blocked := []string{"evil.example.com:34116", "portforge.local:34116", "evil.example.com"}
	for _, host := range blocked {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("host %q: got %d, want 403", host, rec.Code)
		}
	}
}

func TestSPAFallsBackToIndex(t *testing.T) {
	dist := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("<html><head></head></html>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log(1)")},
	}
	h := spaHandler(dist, []byte("INDEX"))

	for path, want := range map[string]string{
		"/":              "INDEX",
		"/games/some-id": "INDEX", // a refresh on a deep link must not 404
		"/assets/app.js": "console.log(1)",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if got := rec.Body.String(); got != want {
			t.Errorf("GET %s = %q, want %q", path, got, want)
		}
	}
}

// sseClient gives up on a stream that stops delivering. http.Client.Timeout
// covers reading the body, which is what makes a missing frame a fast failure
// rather than a wait for the next keepalive.
var sseClient = &http.Client{Timeout: 3 * time.Second}

// readFrames collects SSE frames from a stream until n data lines have arrived
// or the deadline passes. Each frame is returned with the id: that preceded it.
func readFrames(t *testing.T, body io.Reader, n int) (ids []string, events []sseEvent) {
	t.Helper()
	scanner := bufio.NewScanner(body)
	deadline := time.Now().Add(3 * time.Second)
	var lastID string
	for scanner.Scan() && len(events) < n {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "id: "):
			lastID = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "data: "):
			var ev sseEvent
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
				t.Fatalf("event was not JSON: %v (%s)", err, line)
			}
			ids = append(ids, lastID)
			events = append(events, ev)
		}
		if time.Now().After(deadline) {
			break
		}
	}
	if len(events) < n {
		t.Fatalf("only %d of %d events arrived", len(events), n)
	}
	return ids, events
}

// Every frame carries its sequence number as the SSE id, which is what the
// browser sends back as Last-Event-ID when it reconnects on its own.
func TestEventFramesCarryTheSequenceAsTheirID(t *testing.T) {
	hub := newEventHub()
	hub.broadcast("a", nil)
	hub.broadcast("b", nil)
	hub.broadcast("c", nil)

	srv := httptest.NewServer(hub)
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	req.Header.Set("Last-Event-ID", "1")
	res, err := sseClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	ids, events := readFrames(t, res.Body, 2)
	for i, ev := range events {
		if ids[i] != strconv.FormatUint(ev.Seq, 10) {
			t.Errorf("frame %d: id: line %q does not match seq %d", i, ids[i], ev.Seq)
		}
	}
	if events[0].Seq != 2 || events[1].Seq != 3 {
		t.Errorf("seqs = %d, %d; want 2, 3", events[0].Seq, events[1].Seq)
	}
}

// A reconnecting browser presents the last id it saw and gets everything after
// it — the events that happened while it was gone — before live delivery starts.
func TestReconnectReplaysEventsMissedWhileAway(t *testing.T) {
	hub := newEventHub()
	for i := 1; i <= 5; i++ {
		hub.broadcast("install:log", map[string]interface{}{"line": i})
	}

	srv := httptest.NewServer(hub)
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	req.Header.Set("Last-Event-ID", "2")
	res, err := sseClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	// 3, 4, 5 were missed; then one live event proves the stream continues.
	go func() {
		time.Sleep(50 * time.Millisecond)
		hub.broadcast("install:log", map[string]interface{}{"line": 6})
	}()
	_, events := readFrames(t, res.Body, 4)
	var got []uint64
	for _, ev := range events {
		got = append(got, ev.Seq)
	}
	want := []uint64{3, 4, 5, 6}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("seqs = %v, want %v", got, want)
		}
	}
}

// The query form exists for a caller that kept the cursor itself, since
// EventSource cannot set headers on a first connection.
func TestSinceQueryResumesLikeTheHeader(t *testing.T) {
	hub := newEventHub()
	hub.broadcast("a", nil)
	hub.broadcast("b", nil)

	srv := httptest.NewServer(hub)
	defer srv.Close()
	res, err := sseClient.Get(srv.URL + "?since=1")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	_, events := readFrames(t, res.Body, 1)
	if events[0].Seq != 2 || events[0].Name != "b" {
		t.Errorf("got seq %d %q, want 2 \"b\"", events[0].Seq, events[0].Name)
	}
}

// A fresh page — no cursor — must not be handed history it never saw. It
// refetches state on mount; replaying install:log lines into it would double
// them.
func TestFreshConnectionGetsNoHistory(t *testing.T) {
	hub := newEventHub()
	hub.broadcast("stale", nil)

	ch, missed := hub.subscribe(0)
	defer hub.remove(ch)
	if len(missed) != 0 {
		t.Errorf("a live-only subscription was handed %d old events", len(missed))
	}
}

// History is bounded. A client gone longer than replayDepth events gets the
// newest replayDepth and loses the rest — a real limit the depth is sized for.
func TestReplayIsBoundedToTheNewestEvents(t *testing.T) {
	hub := newEventHub()
	for i := 0; i < replayDepth+10; i++ {
		hub.broadcast("install:log", nil)
	}

	ch, missed := hub.subscribe(1)
	defer hub.remove(ch)
	if len(missed) != replayDepth {
		t.Fatalf("replayed %d events, want %d", len(missed), replayDepth)
	}
	if first := missed[0].Seq; first != 11 {
		t.Errorf("oldest replayed seq = %d, want 11 (the 10 oldest should have been dropped)", first)
	}
	if last := missed[len(missed)-1].Seq; last != uint64(replayDepth+10) {
		t.Errorf("newest replayed seq = %d, want %d", last, replayDepth+10)
	}
}

// A garbage cursor is treated as no cursor, not as an error and not as zero
// history from the beginning of time.
func TestMalformedCursorMeansLiveOnly(t *testing.T) {
	hub := newEventHub()
	hub.broadcast("a", nil)
	req, _ := http.NewRequest(http.MethodGet, "/events", nil)
	req.Header.Set("Last-Event-ID", "not-a-number")
	if got := resumeFrom(req); got != 0 {
		t.Errorf("resumeFrom(garbage) = %d, want 0", got)
	}
}
