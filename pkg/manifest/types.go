package manifest

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type IdentitySourceSpec struct {
	Type       string         `yaml:"type" json:"type"` // ldap, okta, etc.
	Config     map[string]any `yaml:"config" json:"config"`
	SyncPeriod string         `yaml:"syncPeriod" json:"syncPeriod"`
}

type IdentitySource struct {
	metav1.TypeMeta   `yaml:",inline"`
	metav1.ObjectMeta `yaml:"metadata"`
	Spec              IdentitySourceSpec `yaml:"spec"`
}

type SyncTargetSpec struct {
	SourceRef    string              `yaml:"sourceRef" json:"sourceRef"`
	Operator     string              `yaml:"operator" json:"operator"`
	Transformers []TransformerConfig `yaml:"transformers" json:"transformers"`
	Config       map[string]any      `yaml:"config" json:"config"`
	DryRun       bool                `yaml:"dryRun" json:"dryRun"`
}

type TransformerConfig struct {
	Name   string         `yaml:"name" json:"name"`
	Type   string         `yaml:"type" json:"type"` // filter, map, template
	Config map[string]any `yaml:"config" json:"config"`
}

type SyncTarget struct {
	metav1.TypeMeta   `yaml:",inline"`
	metav1.ObjectMeta `yaml:"metadata"`
	Spec              SyncTargetSpec   `yaml:"spec"`
	Status            SyncTargetStatus `yaml:"status,omitempty"`
}

type SyncTargetStatus struct {
	LastSync      metav1.Time `yaml:"lastSync" json:"lastSync"`
	Status        string      `yaml:"status" json:"status"`
	Message       string      `yaml:"message" json:"message"`
	IdentityCount int         `yaml:"identityCount" json:"identityCount"`
	GroupCount    int         `yaml:"groupCount" json:"groupCount"`
}
