package session

import (
	"sync"
	"time"
)

// Entry maps a client conversation to an ACP session.
type Entry struct {
	SessionID      string
	AgentID        string
	Model          string
	Mode           string
	Workspace      string
	LastPromptHash string
	LastSentAt     time.Time
	MessageCount   int
}

// Pool tracks conversation_id → ACP session with LRU eviction.
type Pool struct {
	mu      sync.Mutex
	entries map[string]*Entry
	order   []string
	ttl     time.Duration
	max     int
}

func NewPool(ttl time.Duration, maxEntries int) *Pool {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	if maxEntries <= 0 {
		maxEntries = 256
	}
	return &Pool{
		entries: make(map[string]*Entry),
		ttl:     ttl,
		max:     maxEntries,
	}
}

func (p *Pool) Get(conversationID, agentID string) (*Entry, bool) {
	if conversationID == "" {
		return nil, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.evictLocked()
	e, ok := p.entries[conversationID]
	if !ok {
		return nil, false
	}
	if time.Since(e.LastSentAt) > p.ttl {
		delete(p.entries, conversationID)
		return nil, false
	}
	if agentID != "" && e.AgentID != "" && e.AgentID != agentID {
		delete(p.entries, conversationID)
		return nil, false
	}
	return e, true
}

func (p *Pool) Put(conversationID, agentID, sessionID, model, mode, workspace, promptHash string, msgCount int) {
	if conversationID == "" || sessionID == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.evictLocked()
	e := &Entry{
		SessionID:      sessionID,
		AgentID:        agentID,
		Model:          model,
		Mode:           mode,
		Workspace:      workspace,
		LastPromptHash: promptHash,
		LastSentAt:     time.Now(),
		MessageCount:   msgCount,
	}
	p.entries[conversationID] = e
	p.touchLocked(conversationID)
	if len(p.entries) > p.max {
		p.evictOldestLocked()
	}
}

func (p *Pool) Delete(conversationID string) {
	if conversationID == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.entries, conversationID)
	for i, v := range p.order {
		if v == conversationID {
			p.order = append(p.order[:i], p.order[i+1:]...)
			break
		}
	}
}

func (p *Pool) InvalidateAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.entries = make(map[string]*Entry)
	p.order = nil
}

func (p *Pool) touchLocked(id string) {
	for i, v := range p.order {
		if v == id {
			p.order = append(append(p.order[:i], p.order[i+1:]...), id)
			return
		}
	}
	p.order = append(p.order, id)
}

func (p *Pool) evictLocked() {
	now := time.Now()
	for id, e := range p.entries {
		if now.Sub(e.LastSentAt) > p.ttl {
			delete(p.entries, id)
		}
	}
}

func (p *Pool) evictOldestLocked() {
	if len(p.order) == 0 {
		return
	}
	oldest := p.order[0]
	delete(p.entries, oldest)
	p.order = p.order[1:]
}

func (p *Pool) Active() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.entries)
}
