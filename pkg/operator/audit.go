package operator

import (
	"encoding/json"
	"sync"
	"time"
)

type Action string

const (
	ActionCreate Action = "CREATE"
	ActionUpdate Action = "UPDATE"
	ActionDelete Action = "DELETE"
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
	entries []AuditEntry
}

func NewSyncResult(target string) *SyncResult {
	return &SyncResult{
		target: target,
	}
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
	r.entries = append(r.entries, entry)
	r.mu.Unlock()
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
