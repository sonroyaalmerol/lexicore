package manifest

import (
	"fmt"
	"time"
)

type Validator struct{}

func NewValidator() *Validator {
	return &Validator{}
}

func (v *Validator) Validate(manifest any) error {
	switch m := manifest.(type) {
	case *IdentitySource:
		return v.validateIdentitySource(m)
	case *SyncTarget:
		return v.validateSyncTarget(m)
	default:
		return fmt.Errorf("unknown manifest type: %T", manifest)
	}
}

func (v *Validator) validateIdentitySource(source *IdentitySource) error {
	if err := v.validateAPIVersion(source.APIVersion); err != nil {
		return err
	}

	if source.Kind != "IdentitySource" {
		return fmt.Errorf("kind must be IdentitySource, got: %s", source.Kind)
	}

	if source.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}

	if source.Spec.Type == "" {
		return fmt.Errorf("spec.type is required")
	}

	if source.Spec.SyncPeriod != "" {
		if _, err := time.ParseDuration(source.Spec.SyncPeriod); err != nil {
			return fmt.Errorf("invalid syncPeriod: %w", err)
		}
	}

	switch source.Spec.Type {
	case "ldap":
		return v.validateLDAPConfig(source.Spec.Config)
	default:
		return nil
	}
}

func (v *Validator) validateLDAPConfig(config map[string]any) error {
	required := []string{"url", "bindDN", "bindPassword", "baseDN"}
	for _, field := range required {
		if _, ok := config[field]; !ok {
			return fmt.Errorf("LDAP config missing required field: %s", field)
		}
	}
	return nil
}

func (v *Validator) validateSyncTarget(target *SyncTarget) error {
	// Validate API version
	if err := v.validateAPIVersion(target.APIVersion); err != nil {
		return err
	}

	if target.Kind != "SyncTarget" {
		return fmt.Errorf("kind must be SyncTarget, got: %s", target.Kind)
	}

	if target.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}

	if target.Spec.SourceRef == "" {
		return fmt.Errorf("spec.sourceRef is required")
	}

	if target.Spec.Operator == "" {
		return fmt.Errorf("spec.operator is required")
	}

	for i, transformer := range target.Spec.Transformers {
		if err := v.validateTransformer(&transformer); err != nil {
			return fmt.Errorf("transformer[%d] invalid: %w", i, err)
		}
	}

	return nil
}

func (v *Validator) validateAPIVersion(apiVersion string) error {
	if apiVersion == "" {
		return fmt.Errorf("apiVersion is required")
	}

	if apiVersion != SupportedAPIVersion {
		return fmt.Errorf(
			"unsupported apiVersion: %s (supported: %s)",
			apiVersion,
			SupportedAPIVersion,
		)
	}

	return nil
}

func (v *Validator) validateTransformer(transformer *TransformerConfig) error {
	if transformer.Name == "" {
		return fmt.Errorf("transformer name is required")
	}

	if transformer.Type == "" {
		return fmt.Errorf("transformer type is required")
	}

	// Validate known transformer types
	switch transformer.Type {
	case "selector", "constant", "template", "sanitizers":
		// Valid types
	default:
		// Unknown types are warnings, not errors (for extensibility)
	}

	return nil
}
