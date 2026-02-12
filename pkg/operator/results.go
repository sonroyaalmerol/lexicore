package operator

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/xuri/excelize/v2"
)

type ActionType string

const (
	ActionCreate   ActionType = "CREATE"
	ActionUpdate   ActionType = "UPDATE"
	ActionDelete   ActionType = "DELETE"
	ActionNoOp     ActionType = "NOOP"
	ActionFailed   ActionType = "FAILED"
	ActionGroupAdd ActionType = "GROUP_ADD"
	ActionGroupRem ActionType = "GROUP_REMOVE"
)

type ResourceType string

const (
	ResourceIdentity ResourceType = "IDENTITY"
	ResourceGroup    ResourceType = "GROUP"
)

type AuditEntry struct {
	Timestamp    time.Time         `json:"timestamp"`
	ResourceType ResourceType      `json:"resourceType"`
	Action       ActionType        `json:"action"`
	Target       string            `json:"target"`
	ResourceID   string            `json:"resourceId"`     // The unique ID from source
	ResourceName string            `json:"resourceName"`   // Human readable name (e.g. email or group name)
	Diff         map[string]string `json:"diff,omitempty"` // Key: "AttributeName", Value: "Old -> New"
	Message      string            `json:"message,omitempty"`
	Error        error             `json:"error,omitempty"`
}

type SyncResult struct {
	mu      sync.Mutex
	Entries []AuditEntry
	Target  string
}

func (sr *SyncResult) AddEntry(entry AuditEntry) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	sr.Entries = append(sr.Entries, entry)
}

func (sr *SyncResult) RecordIdentityCreate(id source.Identity) {
	diff := make(map[string]string)
	for k, v := range id.Attributes {
		if vStr, ok := v.(string); ok {
			diff[k] = vStr
		}
	}

	sr.AddEntry(AuditEntry{
		ResourceType: ResourceIdentity,
		Action:       ActionCreate,
		Target:       sr.Target,
		ResourceID:   id.UID,
		ResourceName: id.Username,
		Diff:         diff,
		Message:      fmt.Sprintf("Created user with username %s", id.Username),
	})
}

func (sr *SyncResult) RecordIdentityUpdate(id source.Identity, diff map[string]string) {
	sr.AddEntry(AuditEntry{
		ResourceType: ResourceIdentity,
		Action:       ActionUpdate,
		Target:       sr.Target,
		ResourceID:   id.UID,
		ResourceName: id.Username,
		Diff:         diff,
		Message:      fmt.Sprintf("Updated user with %d attributes", len(diff)),
	})
}

func (sr *SyncResult) RecordIdentityUpdateManual(uid, username string, diff map[string]string) {
	sr.AddEntry(AuditEntry{
		ResourceType: ResourceIdentity,
		Action:       ActionUpdate,
		Target:       sr.Target,
		ResourceID:   uid,
		ResourceName: username,
		Diff:         diff,
		Message:      fmt.Sprintf("Updated user with %d attributes", len(diff)),
	})
}

func (sr *SyncResult) RecordIdentityDelete(username string) {
	sr.AddEntry(AuditEntry{
		ResourceType: ResourceIdentity,
		Action:       ActionDelete,
		Target:       sr.Target,
		ResourceID:   username,
		ResourceName: username,
	})
}

func (sr *SyncResult) RecordIdentityError(id source.Identity, action ActionType, err error) {
	sr.AddEntry(AuditEntry{
		ResourceType: ResourceIdentity,
		Action:       action,
		Target:       sr.Target,
		ResourceID:   id.UID,
		ResourceName: id.Username,
		Error:        err,
		Message:      err.Error(),
	})
}

func (sr *SyncResult) RecordIdentityErrorManual(id string, action ActionType, err error) {
	sr.AddEntry(AuditEntry{
		ResourceType: ResourceIdentity,
		Action:       action,
		Target:       sr.Target,
		ResourceID:   id,
		ResourceName: id,
		Error:        err,
		Message:      err.Error(),
	})
}

func (sr *SyncResult) RecordGroupMembership(id source.Identity, groupName string, action ActionType) {
	sr.AddEntry(AuditEntry{
		ResourceType: ResourceIdentity,
		Action:       action,
		Target:       sr.Target,
		ResourceID:   id.UID,
		ResourceName: id.Username,
		Message:      fmt.Sprintf("%s group: %s", action, groupName),
		Diff:         map[string]string{"group": groupName},
	})
}

func (sr *SyncResult) SummaryCounts() map[string]int {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	counts := make(map[string]int)
	for _, e := range sr.Entries {
		key := fmt.Sprintf("%s_%s", e.ResourceType, e.Action)
		counts[key]++
		if e.Error != nil {
			counts["TOTAL_ERRORS"]++
		}
	}
	return counts
}

func GenerateCSV(file *os.File, entries []AuditEntry) {
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Sheet1"

	headers := []string{"Timestamp", "Action", "ResourceType", "ID", "Name", "Diff", "Error"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	for idx, e := range entries {
		row := idx + 2
		diffStr := make([]string, 0, len(e.Diff))
		for k, v := range e.Diff {
			diffStr = append(diffStr, fmt.Sprintf("%s:%s", k, v))
		}

		errorStr := ""
		if e.Error != nil {
			errorStr = e.Error.Error()
		}

		values := []any{
			e.Timestamp.Format(time.RFC3339),
			e.Action,
			e.ResourceType,
			e.ResourceID,
			e.ResourceName,
			strings.Join(diffStr, "|"),
			errorStr,
		}

		for i, v := range values {
			cell, _ := excelize.CoordinatesToCellName(i+1, row)
			f.SetCellValue(sheet, cell, v)
		}
	}

	f.Write(file)
}
