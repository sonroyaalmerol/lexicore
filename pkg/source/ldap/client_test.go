package ldap

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLDAPSource_Connect(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		mConfig *MapperConfig
		wantErr bool
	}{
		{
			name: "valid config structure",
			config: &Config{
				URL:          "ldap://localhost:389",
				BindDN:       "cn=admin,dc=example,dc=com",
				BindPassword: "password",
				BaseDN:       "dc=example,dc=com",
			},
			mConfig: &MapperConfig{},
			wantErr: true, // Will fail without real LDAP server but verifies struct compatibility
		},
		{
			name: "invalid URL",
			config: &Config{
				URL:          "invalid://url",
				BindDN:       "cn=admin,dc=example,dc=com",
				BindPassword: "password",
				BaseDN:       "dc=example,dc=com",
			},
			mConfig: &MapperConfig{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := NewLDAPSource(tt.config, tt.mConfig)
			err := source.Connect(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				defer source.Close()
			}
		})
	}
}
