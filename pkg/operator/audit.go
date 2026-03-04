package operator

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

type Action string

const (
	ActionUpdate Action = "UPDATE"
	ActionSkip   Action = "SKIP"
)

type ChangeKind string

const (
	KindAttribute  ChangeKind = "attribute"
	KindMembership ChangeKind = "membership"
)

type Change struct {
	Kind  ChangeKind `json:"kind"`
	Field string     `json:"field"`
	Old   string     `json:"old,omitempty"`
	New   string     `json:"new,omitempty"`
}

func AttrChange(field, old, new string) Change {
	return Change{Kind: KindAttribute, Field: field, Old: old, New: new}
}

func MembershipAdded(group string) Change {
	return Change{Kind: KindMembership, Field: group, New: "member"}
}

func MembershipRemoved(group string) Change {
	return Change{Kind: KindMembership, Field: group, Old: "member"}
}

type AuditEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Action    Action    `json:"action"`
	Target    string    `json:"target"`
	UID       string    `json:"uid"`
	Name      string    `json:"name"`
	Changes   []Change  `json:"changes,omitempty"`
	Error     error     `json:"-"`
}

func (e AuditEntry) MarshalJSON() ([]byte, error) {
	type Alias AuditEntry
	var errStr string
	if e.Error != nil {
		errStr = e.Error.Error()
	}
	return json.Marshal(&struct {
		Alias
		Error string `json:"error,omitempty"`
	}{
		Alias: Alias(e),
		Error: errStr,
	})
}

type SyncResult struct {
	mu      sync.Mutex
	target  string
	logger  *zap.Logger
	entries []AuditEntry
	index   map[string]int
}

func NewSyncResult(logger *zap.Logger, target string) *SyncResult {
	return &SyncResult{
		target: target,
		logger: logger,
		index:  make(map[string]int),
	}
}

func (r *SyncResult) key(entry AuditEntry) string {
	hasError := "noerror"
	if entry.Error != nil {
		hasError = "error"
	}
	return fmt.Sprintf("%s:%s:%s:%s", entry.Target, entry.Name, entry.Action, hasError)
}

func (r *SyncResult) Record(action Action, uid, name string, changes ...Change) {
	entry := AuditEntry{
		Timestamp: time.Now(),
		Action:    action,
		Target:    r.target,
		UID:       uid,
		Name:      name,
		Changes:   changes,
	}
	r.append(entry)
}

func (r *SyncResult) RecordError(action Action, uid, name string, err error) {
	entry := AuditEntry{
		Timestamp: time.Now(),
		Action:    action,
		Target:    r.target,
		UID:       uid,
		Name:      name,
		Error:     err,
	}
	r.append(entry)
}

func (r *SyncResult) append(entry AuditEntry) {
	r.mu.Lock()

	key := r.key(entry)

	if idx, exists := r.index[key]; exists && entry.Error == nil {
		if len(entry.Changes) > 0 {
			r.entries[idx].Changes = append(r.entries[idx].Changes, entry.Changes...)
		}
		r.mu.Unlock()
		return
	}

	index := len(r.entries)
	r.entries = append(r.entries, entry)
	r.index[key] = index
	r.mu.Unlock()

	fields := []zap.Field{
		zap.String("audit", "true"),
		zap.Time("timestamp", entry.Timestamp),
		zap.String("action", string(entry.Action)),
		zap.String("target", entry.Target),
		zap.String("uid", entry.UID),
		zap.String("name", entry.Name),
	}

	if entry.Error != nil {
		fields = append(fields, zap.Error(entry.Error))
		r.logger.Error("audit event", fields...)
		return
	}

	if len(entry.Changes) > 0 {
		fields = append(fields, zap.Any("changes", entry.Changes))
	}

	r.logger.Info("audit event", fields...)
}

func (r *SyncResult) Entries() []AuditEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]AuditEntry, len(r.entries))
	copy(out, r.entries)
	return out
}

func (r *SyncResult) Counts() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	counts := make(map[string]int)
	for _, e := range r.entries {
		counts[string(e.Action)]++
		if e.Error != nil {
			counts["ERRORS"]++
		}
	}
	return counts
}
