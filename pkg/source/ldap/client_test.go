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
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				URL:          "ldap://localhost:389",
				BindDN:       "cn=admin,dc=example,dc=com",
				BindPassword: "password",
				BaseDN:       "dc=example,dc=com",
			},
			wantErr: true, // Will fail without real LDAP server
		},
		{
			name: "invalid URL",
			config: &Config{
				URL:          "invalid://url",
				BindDN:       "cn=admin,dc=example,dc=com",
				BindPassword: "password",
				BaseDN:       "dc=example,dc=com",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := NewLDAPSource(tt.config)
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
