package manifest

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type IdentitySourceSpec struct {
	Type       string         `json:"type"` // ldap, okta, etc.
	Config     map[string]any `json:"config"`
	SyncPeriod string         `json:"syncPeriod"`
}

type IdentitySource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              IdentitySourceSpec `json:"spec"`
}

type SyncTargetSpec struct {
	SourceRef    string              `json:"sourceRef"`
	Operator     string              `json:"operator"`
	Transformers []TransformerConfig `json:"transformers"`
	Config       map[string]any      `json:"config"`
	DryRun       bool                `json:"dryRun"`
}

type TransformerConfig struct {
	Name   string         `json:"name"`
	Type   string         `json:"type"` // filter, map, template
	Config map[string]any `json:"config"`
}

type SyncTarget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              SyncTargetSpec   `json:"spec"`
	Status            SyncTargetStatus `json:"status"`
}

type SyncTargetStatus struct {
	LastSync      metav1.Time `json:"lastSync"`
	Status        string      `json:"status"`
	Message       string      `json:"message"`
	IdentityCount int         `json:"identityCount"`
	GroupCount    int         `json:"groupCount"`
}
