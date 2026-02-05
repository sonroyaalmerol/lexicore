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

type PluginSource struct {
	// Type specifies how to fetch the plugin (file, git)
	Type string `json:"type"`

	// File-based plugin configuration
	File *FilePluginSource `json:"file,omitempty"`

	// Git-based plugin configuration
	Git *GitPluginSource `json:"git,omitempty"`
}

type FilePluginSource struct {
	// Path to the Starlark script file
	Path string `json:"path"`
}

type GitPluginSource struct {
	// Repository URL
	URL string `json:"url"`

	// Ref to checkout (branch, tag, or commit SHA)
	// +optional
	Ref string `json:"ref,omitempty"`

	// Path to the Starlark script within the repository
	Path string `json:"path"`

	// Authentication credentials
	// +optional
	Auth *GitAuth `json:"auth,omitempty"`
}

type GitAuth struct {
	// SecretRef references a Secret containing auth credentials
	SecretRef string `json:"secretRef"`
}

type SyncTargetSpec struct {
	SourceRef    string              `json:"sourceRef"`
	Operator     string              `json:"operator"`
	Transformers []TransformerConfig `json:"transformers"`
	Config       map[string]any      `json:"config"`
	DryRun       bool                `json:"dryRun"`

	// Plugin source for Starlark-based operators
	// When specified, the operator field should be set to "starlark"
	// +optional
	PluginSource *PluginSource `json:"pluginSource,omitempty"`
}

type TransformerConfig struct {
	Name   string         `json:"name"`
	Type   string         `json:"type"`
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

	// PluginStatus contains information about the loaded plugin
	// +optional
	PluginStatus *PluginStatus `json:"pluginStatus,omitempty"`
}

type PluginStatus struct {
	// Loaded indicates if the plugin was successfully loaded
	Loaded bool `json:"loaded"`

	// Source tracking for Git-based plugins
	// +optional
	GitCommit string `json:"gitCommit,omitempty"`

	// LastUpdated tracks when the plugin was last fetched
	// +optional
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`

	// Error message if plugin failed to load
	// +optional
	Error string `json:"error,omitempty"`
}
