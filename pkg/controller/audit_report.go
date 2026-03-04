package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/operator"
	"go.uber.org/zap"
)

func (m *Manager) generateAuditReport(
	target *ActiveOperator,
	targetName string,
	result *operator.SyncResult,
) {
	if target.manifest.Spec.Config["generateAuditReport"].Value() != true {
		return
	}

	counts := result.Counts()
	if counts[string(operator.ActionUpdate)] == 0 {
		m.logger.Info(
			"Skipping audit report - no changes detected",
			zap.String("target", targetName),
		)
		return
	}

	switch m.cfg.Audit.Mode {
	case "email":
		cfg := m.cfg.Audit.Email
		sender := operator.NewEmailAuditSender(
			cfg.SMTP.Host,
			cfg.SMTP.Port,
			cfg.SMTP.Username,
			cfg.SMTP.Password,
			cfg.SMTP.TLSMode,
			cfg.From,
			cfg.To,
			cfg.SubjectFmt,
		)
		if err := sender.Send(targetName, result.Entries()); err != nil {
			m.logger.Error(
				"Failed to send audit report email",
				zap.String("target", targetName),
				zap.Error(err),
			)
			return
		}
		m.logger.Info(
			"Audit report sent via email",
			zap.String("target", targetName),
			zap.Strings("to", cfg.To),
		)
	default:
		if err := os.MkdirAll(m.cfg.Audit.XLSDir, 0755); err != nil {
			m.logger.Error(
				"Failed to create audit report directory",
				zap.Error(err),
			)
			return
		}
		filename := fmt.Sprintf(
			"audit_log_%s_%d.xlsx", targetName, time.Now().Unix(),
		)
		fullPath := filepath.Join(m.cfg.Audit.XLSDir, filename)
		file, err := os.Create(fullPath)
		if err != nil {
			m.logger.Error(
				"Failed to create audit report file",
				zap.Error(err),
			)
			return
		}
		defer file.Close()
		if err := operator.ExportToExcel(
			file, result.Entries(),
		); err != nil {
			m.logger.Error(
				"Failed to write audit report",
				zap.Error(err),
			)
		}
	}
}
