package cursor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"
)

// SessionEntry represents a stored session mapping.
type SessionEntry struct {
	SessionID string    `json:"session_id"`
	Hash      string    `json:"hash"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SessionManager maps conversation history to cursor-agent session IDs.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*SessionEntry // hash -> entry
	filePath string
	lockFile *flock.Flock
	logger   *slog.Logger
	runner       *Runner
	dirty        bool
	saveTimer    *time.Timer
	lockTimeout  time.Duration
}

// NewSessionManager creates a new session manager.
func NewSessionManager(storagePath string, runner *Runner, logger *slog.Logger, lockTimeout time.Duration) (*SessionManager, error) {
	if err := os.MkdirAll(filepath.Dir(storagePath), 0755); err != nil && filepath.Dir(storagePath) != "." {
		return nil, fmt.Errorf("create session dir: %w", err)
	}

	sm := &SessionManager{
		sessions:    make(map[string]*SessionEntry),
		filePath:    storagePath,
		lockFile:    flock.New(storagePath + ".lock"),
		logger:      logger,
		runner:      runner,
		lockTimeout: lockTimeout,
	}

	if err := sm.load(); err != nil {
		logger.Warn("failed to load sessions, starting fresh", "error", err)
	}

	return sm, nil
}

var thinkBlockRe = regexp.MustCompile(`(?s)<think>.*?</think>`)

// Hash computes a deterministic hash of the conversation history.
func (sm *SessionManager) Hash(messages []Message) string {
	h := sha256.New()
	for _, msg := range messages {
		content := ExtractText(&msg)
		if msg.Role == "assistant" {
			content = stripThinkBlocks(content)
		}
		fmt.Fprintf(h, "%s:%s\n", msg.Role, content)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func stripThinkBlocks(content string) string {
	return strings.TrimSpace(thinkBlockRe.ReplaceAllString(content, ""))
}

// GetOrCreate finds an existing session. Returns empty string for new sessions
// (cursor-agent will auto-create a session and return session_id in init event).
func (sm *SessionManager) GetOrCreate(messages []Message) string {
	hash := sm.Hash(messages)

	sm.mu.RLock()
	entry, ok := sm.sessions[hash]
	sm.mu.RUnlock()

	if ok {
		sm.logger.Debug("session hit", "hash", hash[:8], "session", entry.SessionID)
		return entry.SessionID
	}

	sm.logger.Debug("session miss, will auto-create", "hash", hash[:8])
	return ""
}

// RegisterSession registers a session_id from a cursor-agent init event.
func (sm *SessionManager) RegisterSession(messages []Message, sessionID string) {
	if sessionID == "" {
		return
	}

	hash := sm.Hash(messages)

	sm.mu.Lock()
	sm.sessions[hash] = &SessionEntry{
		SessionID: sessionID,
		Hash:      hash,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	sm.mu.Unlock()

	sm.markDirty()
}

// UpdateAfterResponse updates the session mapping after a response.
func (sm *SessionManager) UpdateAfterResponse(messages []Message, responseText string) {
	oldHash := sm.Hash(messages)

	// Create new messages with assistant response appended
	newMessages := append(messages, Message{
		Role:    "assistant",
		Content: []ContentPart{{Type: "text", Text: responseText}},
	})
	newHash := sm.Hash(newMessages)

	sm.mu.Lock()
	if entry, ok := sm.sessions[oldHash]; ok {
		entry.Hash = newHash
		entry.UpdatedAt = time.Now()
		sm.sessions[newHash] = entry
		delete(sm.sessions, oldHash)
	}
	sm.mu.Unlock()

	sm.markDirty()
}

// markDirty schedules a debounced save (500ms). Multiple rapid calls coalesce into one write.
func (sm *SessionManager) markDirty() {
	sm.mu.Lock()
	sm.dirty = true
	if sm.saveTimer != nil {
		sm.saveTimer.Stop()
	}
	sm.saveTimer = time.AfterFunc(500*time.Millisecond, func() {
		sm.mu.Lock()
		sm.dirty = false
		sm.mu.Unlock()
		if err := sm.save(); err != nil {
			sm.logger.Warn("failed to persist sessions", "error", err)
		}
	})
	sm.mu.Unlock()
}

// Flush forces an immediate save if dirty, used for graceful shutdown.
func (sm *SessionManager) Flush() {
	sm.mu.Lock()
	if sm.saveTimer != nil {
		sm.saveTimer.Stop()
		sm.saveTimer = nil
	}
	wasDirty := sm.dirty
	sm.dirty = false
	sm.mu.Unlock()
	if wasDirty {
		if err := sm.save(); err != nil {
			sm.logger.Warn("failed to flush sessions", "error", err)
		}
	}
}

func (sm *SessionManager) withLock(fn func() error) error {
	if sm.lockTimeout > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), sm.lockTimeout)
		defer cancel()
		locked, err := sm.lockFile.TryLockContext(ctx, sm.lockTimeout)
		if err != nil {
			return err
		}
		if !locked {
			return fmt.Errorf("session lock timeout")
		}
		defer sm.lockFile.Unlock()
		return fn()
	}
	if err := sm.lockFile.Lock(); err != nil {
		return err
	}
	defer sm.lockFile.Unlock()
	return fn()
}

func (sm *SessionManager) load() error {
	return sm.withLock(func() error {

		data, err := os.ReadFile(sm.filePath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}

		var entries []*SessionEntry
		if err := json.Unmarshal(data, &entries); err != nil {
			return err
		}

		sm.mu.Lock()
		for _, entry := range entries {
			sm.sessions[entry.Hash] = entry
		}
		sm.mu.Unlock()

		sm.logger.Info("loaded sessions", "count", len(entries))
		return nil
	})
}

func (sm *SessionManager) save() error {
	return sm.withLock(func() error {
		sm.mu.RLock()
		entries := make([]*SessionEntry, 0, len(sm.sessions))
		for _, entry := range sm.sessions {
			entries = append(entries, entry)
		}
		sm.mu.RUnlock()

		data, err := json.Marshal(entries)
		if err != nil {
			return err
		}

		return os.WriteFile(sm.filePath, data, 0600)
	})
}
