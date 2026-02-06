package controller

import (
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (m *Manager) updateTargetStatus(
	targetName string,
	success bool,
	message string,
	identityCount int,
	groupCount int,
) {
	m.activeOperators.Compute(targetName, func(t *ActiveOperator, loaded bool) (*ActiveOperator, xsync.ComputeOp) {
		if !loaded {
			return t, xsync.CancelOp
		}

		t.manifest.Status.LastSync = v1.NewTime(time.Now())
		if success {
			t.manifest.Status.Status = "Success"
		} else {
			t.manifest.Status.Status = "Failed"
		}
		t.manifest.Status.Message = message
		t.manifest.Status.IdentityCount = identityCount
		t.manifest.Status.GroupCount = groupCount

		return t, xsync.UpdateOp
	})
}
