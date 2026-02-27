package manifest

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ValueFromSource struct {
	Env  string `yaml:"env,omitempty"  json:"env,omitempty"`
	File string `yaml:"file,omitempty" json:"file,omitempty"`
}

type SecretValue struct {
	Value     *string          `yaml:"value,omitempty"     json:"value,omitempty"`
	ValueFrom *ValueFromSource `yaml:"valueFrom,omitempty" json:"valueFrom,omitempty"`
}

type IdentitySourceSpec struct {
	Type         string                 `yaml:"type"       json:"type"`
	Config       map[string]ConfigValue `yaml:"config"     json:"config"`
	SyncPeriod   string                 `yaml:"syncPeriod" json:"syncPeriod"`
	PluginSource *PluginSource          `yaml:"pluginSource,omitempty" json:"pluginSource,omitempty"`
}

type IdentitySource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `yaml:"metadata" json:"metadata"`
	Spec              IdentitySourceSpec `yaml:"spec" json:"spec"`
}

type PluginSource struct {
	Type string            `yaml:"type" json:"type"`
	File *FilePluginSource `yaml:"file,omitempty" json:"file,omitempty"`
	Git  *GitPluginSource  `yaml:"git,omitempty"  json:"git,omitempty"`
}

type FilePluginSource struct {
	Path string `yaml:"path" json:"path"`
}

type GitPluginSource struct {
	URL  string   `yaml:"url"            json:"url"`
	Ref  string   `yaml:"ref,omitempty"  json:"ref,omitempty"`
	Path string   `yaml:"path"           json:"path"`
	Auth *GitAuth `yaml:"auth,omitempty" json:"auth,omitempty"`
}

type GitAuth struct {
	SecretRef string `yaml:"secretRef" json:"secretRef"`
}

type SyncTargetSpec struct {
	SourceRef    string                 `yaml:"sourceRef"              json:"sourceRef"`
	Operator     string                 `yaml:"operator"               json:"operator"`
	Transformers []TransformerConfig    `yaml:"transformers"           json:"transformers"`
	Config       map[string]ConfigValue `yaml:"config"                 json:"config"`
	DryRun       bool                   `yaml:"dryRun"                 json:"dryRun"`
	PluginSource *PluginSource          `yaml:"pluginSource,omitempty" json:"pluginSource,omitempty"`
}

type TransformerConfig struct {
	Name   string                 `yaml:"name"   json:"name"`
	Type   string                 `yaml:"type"   json:"type"`
	Config map[string]ConfigValue `yaml:"config" json:"config"`
}

type SyncTarget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `yaml:"metadata" json:"metadata"`
	Spec              SyncTargetSpec   `yaml:"spec"   json:"spec"`
	Status            SyncTargetStatus `yaml:"status" json:"status"`
}

type SyncTargetStatus struct {
	LastSync      metav1.Time   `yaml:"lastSync"      json:"lastSync"`
	Status        string        `yaml:"status"        json:"status"`
	Message       string        `yaml:"message"       json:"message"`
	IdentityCount int           `yaml:"identityCount" json:"identityCount"`
	GroupCount    int           `yaml:"groupCount"    json:"groupCount"`
	PluginStatus  *PluginStatus `yaml:"pluginStatus,omitempty" json:"pluginStatus,omitempty"`
}

type PluginStatus struct {
	Loaded      bool        `yaml:"loaded"               json:"loaded"`
	GitCommit   string      `yaml:"gitCommit,omitempty"  json:"gitCommit,omitempty"`
	LastUpdated metav1.Time `yaml:"lastUpdated,omitempty" json:"lastUpdated,omitempty"`
	Error       string      `yaml:"error,omitempty"      json:"error,omitempty"`
}
