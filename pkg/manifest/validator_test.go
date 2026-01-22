package manifest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidator_ValidateIdentitySource(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name    string
		source  *IdentitySource
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid source",
			source: &IdentitySource{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: IdentitySourceSpec{
					Type:       "ldap",
					SyncPeriod: "5m",
					Config: map[string]any{
						"url":          "ldap://localhost",
						"bindDN":       "cn=admin",
						"bindPassword": "pass",
						"baseDN":       "dc=example",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			source: &IdentitySource{
				Spec: IdentitySourceSpec{
					Type: "ldap",
				},
			},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name: "missing type",
			source: &IdentitySource{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec:       IdentitySourceSpec{},
			},
			wantErr: true,
			errMsg:  "type is required",
		},
		{
			name: "invalid sync period",
			source: &IdentitySource{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: IdentitySourceSpec{
					Type:       "ldap",
					SyncPeriod: "invalid",
				},
			},
			wantErr: true,
			errMsg:  "invalid syncPeriod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.source)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateSyncTarget(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name    string
		target  *SyncTarget
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid target",
			target: &SyncTarget{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: SyncTargetSpec{
					SourceRef: "test-source",
					Operator:  "dovecot",
				},
			},
			wantErr: false,
		},
		{
			name: "missing sourceRef",
			target: &SyncTarget{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: SyncTargetSpec{
					Operator: "dovecot",
				},
			},
			wantErr: true,
			errMsg:  "sourceRef is required",
		},
		{
			name: "missing operator",
			target: &SyncTarget{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: SyncTargetSpec{
					SourceRef: "test-source",
				},
			},
			wantErr: true,
			errMsg:  "operator is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.target)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
