package ui

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/puremachinery/gemini-cli-go/internal/llm"
	"github.com/puremachinery/gemini-cli-go/internal/session"
)

// autoSessionSaver persists session snapshots at a throttled interval.
type autoSessionSaver struct {
	id          string
	startedAt   time.Time
	authType    string
	store       session.Store
	now         func() time.Time
	lastSavedAt time.Time
	lastCount   int
	minInterval time.Duration
}

const autoSessionPrefix = "session-"
const autoSessionMinInterval = time.Second

// newAutoSessionSaver builds a throttled saver for auto sessions.
func newAutoSessionSaver(store session.Store, authType string, now func() time.Time) *autoSessionSaver {
	if store == nil {
		return nil
	}
	if now == nil {
		now = time.Now
	}
	ts := now()
	suffix := randomSessionSuffix(4)
	return &autoSessionSaver{
		id:          fmt.Sprintf("%s%d-%s", autoSessionPrefix, ts.UnixNano(), suffix),
		startedAt:   ts,
		authType:    authType,
		store:       store,
		now:         now,
		minInterval: autoSessionMinInterval,
	}
}

func (s *autoSessionSaver) SetAuthType(authType string) {
	if s == nil {
		return
	}
	s.authType = authType
}

// isAutoSessionID reports whether an ID belongs to an auto-saved session.
func isAutoSessionID(id string) bool {
	return strings.HasPrefix(id, autoSessionPrefix)
}

// randomSessionSuffix returns a short hex suffix for collision avoidance.
func randomSessionSuffix(n int) string {
	if n <= 0 {
		return "0000"
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "0000"
	}
	return hex.EncodeToString(buf)
}

// Save writes the latest session snapshot unless throttled.
func (s *autoSessionSaver) Save(messages []llm.Message) error {
	if s == nil || s.store == nil {
		return nil
	}
	if len(messages) == 0 {
		return nil
	}
	if s.minInterval > 0 && s.now != nil {
		if !s.lastSavedAt.IsZero() && s.now().Sub(s.lastSavedAt) < s.minInterval {
			return nil
		}
	}
	if s.lastSavedAt != (time.Time{}) && len(messages) == s.lastCount {
		return nil
	}
	sess := &session.Session{
		ID:        s.id,
		StartedAt: s.startedAt,
		AuthType:  s.authType,
		Messages:  messages,
	}
	if deltaSaver, ok := s.store.(interface {
		SaveDelta(*session.Session) error
	}); ok {
		if err := deltaSaver.SaveDelta(sess); err != nil {
			return err
		}
	} else {
		if err := s.store.Save(sess); err != nil {
			return err
		}
	}
	if s.now != nil {
		s.lastSavedAt = s.now()
	}
	s.lastCount = len(messages)
	return nil
}
