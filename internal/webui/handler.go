package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/chaojimct/cli-agent-gateway/internal/admin"
	"github.com/chaojimct/cli-agent-gateway/internal/config"
	"nhooyr.io/websocket"
)

// Hub manages WebSocket connections.
type Hub struct {
	mu           sync.RWMutex
	clients      map[*websocket.Conn]bool
	tapSubs      map[chan []byte]struct{}
	logger       *slog.Logger
}

// NewHub creates a new WebSocket hub.
func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		clients: make(map[*websocket.Conn]bool),
		tapSubs: make(map[chan []byte]struct{}),
		logger:  logger,
	}
}

// BroadcastTap sends a claude-tap record JSON line to SSE subscribers.
func (h *Hub) BroadcastTap(record []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.tapSubs {
		select {
		case ch <- record:
		default:
		}
	}
}

// SubscribeTap registers an SSE subscriber channel.
func (h *Hub) SubscribeTap() chan []byte {
	ch := make(chan []byte, 32)
	h.mu.Lock()
	h.tapSubs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// UnsubscribeTap removes an SSE subscriber channel.
func (h *Hub) UnsubscribeTap(ch chan []byte) {
	h.mu.Lock()
	delete(h.tapSubs, ch)
	h.mu.Unlock()
}

// Broadcast sends a message to all connected clients.
func (h *Hub) Broadcast(event TraceEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		h.logger.Error("failed to marshal event", "error", err)
		return
	}

	h.mu.RLock()
	var stale []*websocket.Conn
	for conn := range h.clients {
		if err := conn.Write(context.Background(), websocket.MessageText, data); err != nil {
			h.logger.Warn("failed to write to websocket", "error", err)
			conn.Close(websocket.StatusInternalError, "write failed")
			stale = append(stale, conn)
		}
	}
	h.mu.RUnlock()

	// Clean up stale connections under write lock
	if len(stale) > 0 {
		h.mu.Lock()
		for _, conn := range stale {
			delete(h.clients, conn)
		}
		h.mu.Unlock()
	}
}

// Register adds a new WebSocket connection.
func (h *Hub) Register(conn *websocket.Conn) {
	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()
}

// Unregister removes a WebSocket connection.
func (h *Hub) Unregister(conn *websocket.Conn) {
	h.mu.Lock()
	delete(h.clients, conn)
	h.mu.Unlock()
}

// Handler serves the Web UI.
type Handler struct {
	store               *Store
	hub                 *Hub
	logger              *slog.Logger
	cfgMgr              *config.Manager
	restart             *admin.Coordinator
	authEnabled         bool
	allowUnauthConfig   bool
	allowedOrigins      []string
}

// NewHandler creates a new Web UI handler.
func NewHandler(store *Store, hub *Hub, logger *slog.Logger) *Handler {
	return &Handler{store: store, hub: hub, logger: logger}
}

// Routes registers the Web UI routes.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/", h.ServeIndex)
	r.Get("/legacy", h.ServeLegacyIndex)
	r.Get("/ws/events", h.HandleWebSocket)
	r.Get("/api/tap/events", h.HandleTapSSE)
	r.Get("/api/tap/records", h.GetTapRecords)
	r.Get("/api/traces", h.GetTraces)
	r.Get("/api/traces/{id}", h.GetTrace)
	r.Get("/api/stats", h.GetStats)
}

// ServeIndex serves the claude-tap style trace viewer.
func (h *Handler) ServeIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(tapIndexHTML))
}

// ServeLegacyIndex serves the compact legacy trace UI.
func (h *Handler) ServeLegacyIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(legacyIndexHTML))
}

// HandleTapSSE streams claude-tap records for live viewer updates.
func (h *Handler) HandleTapSSE(w http.ResponseWriter, r *http.Request) {
	if !h.checkOrigin(r) {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	for _, rec := range TapRecordsFromStore(h.store) {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", rec)
		flusher.Flush()
	}

	sub := h.hub.SubscribeTap()
	defer h.hub.UnsubscribeTap(sub)

	ctx := r.Context()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case rec := <-sub:
			_, _ = fmt.Fprintf(w, "data: %s\n\n", rec)
			flusher.Flush()
		case <-ticker.C:
			_, _ = fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// HandleWebSocket handles WebSocket connections.
func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !h.checkOrigin(r) {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}
	patterns := h.allowedOrigins
	if len(patterns) == 0 {
		patterns = []string{"*"}
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: false,
		OriginPatterns:     patterns,
	})
	if err != nil {
		h.logger.Error("websocket accept failed", "error", err)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")

	h.hub.Register(conn)
	defer h.hub.Unregister(conn)

	// Send initial traces
	traces := h.store.GetTraces()
	data, _ := json.Marshal(map[string]interface{}{
		"type":   "init",
		"traces": traces,
		"stats":  h.store.Stats(),
	})
	conn.Write(r.Context(), websocket.MessageText, data)

	// Keep connection alive
	for {
		_, _, err := conn.Read(r.Context())
		if err != nil {
			break
		}
	}
}

// GetTrace returns a specific trace.
func (h *Handler) GetTrace(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	trace := h.store.GetTrace(id)
	if trace == nil {
		http.Error(w, `{"error":"trace not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(trace)
}

// GetStats returns aggregate statistics.
func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	stats := h.store.Stats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
