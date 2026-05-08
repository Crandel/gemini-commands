package server

import "sync"

// Event is a server-sent event payload. Fields are JSON-serialised and broadcast
// to all subscribed SSE clients. Additional fields can be added without breaking
// existing clients.
type Event struct {
	Type    string `json:"type"`           // "progress" | "done" | "failed" | "status"
	Message string `json:"message"`        // human-readable label shown in the UI
	Step    string `json:"step,omitempty"` // pipeline_step value, populated for "status" events
}

// FeatureHub is a fan-out broadcast channel for a single feature's background
// job. Any goroutine can Publish events; any number of SSE handlers can
// Subscribe. Safe for concurrent use.
type FeatureHub struct {
	mu          sync.Mutex
	subscribers map[chan Event]struct{}
	closed      bool
}

// NewFeatureHub returns an initialised FeatureHub.
func NewFeatureHub() *FeatureHub {
	return &FeatureHub{
		subscribers: make(map[chan Event]struct{}),
	}
}

// Publish sends e to all current subscribers. If a subscriber's buffer is full
// the event is dropped (non-blocking send) to avoid blocking the publisher.
func (h *FeatureHub) Publish(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	for ch := range h.subscribers {
		select {
		case ch <- e:
		default:
		}
	}
}

// Subscribe registers a new subscriber and returns its receive channel plus an
// unsubscribe function. The caller must call unsubscribe when done to release
// resources. If the hub is already closed the returned channel is pre-closed so
// the caller's receive loop exits immediately.
func (h *FeatureHub) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 16)
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	h.subscribers[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subscribers, ch)
		h.mu.Unlock()
	}
}

// Close closes all subscriber channels and marks the hub as closed. Further
// Publish or Close calls are no-ops.
func (h *FeatureHub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for ch := range h.subscribers {
		close(ch)
	}
	h.subscribers = make(map[chan Event]struct{})
}
