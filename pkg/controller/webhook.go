package controller

import (
	"context"
	"fmt"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"go.uber.org/zap"
)

func (m *Manager) ProcessWebhook(sourceName string, payload []byte) error {
	activeSource, ok := m.activeSources.Load(sourceName)
	if !ok {
		return fmt.Errorf("source %s not found", sourceName)
	}

	webhookCapable, ok := activeSource.Source.(source.WebhookCapable)
	if !ok {
		return fmt.Errorf("source %s does not support webhooks", sourceName)
	}

	event, err := webhookCapable.ProcessWebhookEvent(m.shutdownCtx, payload)
	if err != nil {
		return fmt.Errorf("failed to process webhook: %w", err)
	}

	select {
	case m.webhookQueue <- webhookReconcileTask{
		sourceName: sourceName,
		event:      event,
	}:
		return nil
	case <-m.shutdownCtx.Done():
		return fmt.Errorf("manager is shutting down")
	default:
		return fmt.Errorf("webhook queue is full")
	}
}

func (m *Manager) webhookProcessor(ctx context.Context) {
	debounceWindow := 5 * time.Second
	if m.cfg.WebhookDebounceWindow > 0 {
		debounceWindow = m.cfg.WebhookDebounceWindow
	}

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("Webhook processor stopped")
			return
		case task := <-m.webhookQueue:
			m.accumulateWebhookUpdate(task, debounceWindow)
		}
	}
}

func (m *Manager) accumulateWebhookUpdate(task webhookReconcileTask, debounceWindow time.Duration) {
	pending, _ := m.pendingWebhookUpdates.LoadOrCompute(task.sourceName, func() (*pendingUpdate, bool) {
		return &pendingUpdate{
			identities: make(map[string]bool),
			groups:     make(map[string]bool),
		}, false
	})

	pending.mu.Lock()
	defer pending.mu.Unlock()

	switch task.event.Type {
	case source.WebhookIdentityCreated, source.WebhookIdentityUpdated:
		if task.event.Identity != nil {
			pending.identities[task.event.Identity.UID] = true
		}
	case source.WebhookIdentityDeleted:
		if task.event.Identity != nil {
			pending.identities[task.event.Identity.UID] = true
		}
	case source.WebhookGroupCreated, source.WebhookGroupUpdated:
		if task.event.Group != nil {
			pending.groups[task.event.Group.GID] = true
		}
	case source.WebhookGroupDeleted:
		if task.event.Group != nil {
			pending.groups[task.event.Group.GID] = true
		}
	}

	if pending.timer != nil {
		pending.timer.Stop()
	}

	pending.timer = time.AfterFunc(debounceWindow, func() {
		m.flushWebhookUpdates(task.sourceName)
	})

	m.logger.Debug(
		"Accumulated webhook update",
		zap.String("source", task.sourceName),
		zap.String("eventType", string(task.event.Type)),
	)
}

func (m *Manager) flushWebhookUpdates(sourceName string) {
	pending, ok := m.pendingWebhookUpdates.LoadAndDelete(sourceName)
	if !ok {
		return
	}

	pending.mu.Lock()
	identityUIDs := make([]string, 0, len(pending.identities))
	for uid := range pending.identities {
		identityUIDs = append(identityUIDs, uid)
	}
	groupGIDs := make([]string, 0, len(pending.groups))
	for gid := range pending.groups {
		groupGIDs = append(groupGIDs, gid)
	}
	pending.mu.Unlock()

	if len(identityUIDs) == 0 && len(groupGIDs) == 0 {
		return
	}

	m.logger.Info(
		"Flushing webhook updates",
		zap.String("source", sourceName),
		zap.Int("identities", len(identityUIDs)),
		zap.Int("groups", len(groupGIDs)),
	)

	var targetNames []string
	m.activeOperators.Range(func(name string, target *ActiveOperator) bool {
		if target.manifest.Spec.SourceRef == sourceName {
			targetNames = append(targetNames, name)
		}
		return true
	})

	for _, targetName := range targetNames {
		select {
		case m.queue <- reconcileTask{
			targetName:   targetName,
			immediate:    true,
			partialSync:  true,
			identityUIDs: identityUIDs,
			groupGIDs:    groupGIDs,
		}:
		case <-m.shutdownCtx.Done():
			return
		default:
			m.logger.Warn(
				"Queue full, skipping webhook-triggered reconciliation",
				zap.String("target", targetName),
			)
		}
	}
}
