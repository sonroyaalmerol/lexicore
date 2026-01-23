package ldap

import (
	"testing"

	"github.com/go-ldap/ldap/v3"
	"github.com/stretchr/testify/assert"
)

func TestMapper_MapIdentity(t *testing.T) {
	config := &MapperConfig{
		UIDAttribute:      "uid",
		UsernameAttribute: "cn",
		EmailAttribute:    "mail",
		GroupsAttribute:   "memberOf",
	}

	mapper := NewMapper(config)

	entry := ldap.NewEntry(
		"cn=john,ou=users,dc=example,dc=com",
		map[string][]string{
			"uid":      {"john123"},
			"cn":       {"john"},
			"mail":     {"john@example.com"},
			"memberOf": {"cn=admins,ou=groups,dc=example,dc=com"},
		},
	)

	identity := mapper.MapIdentity(entry)

	assert.Equal(t, "john123", identity.UID)
	assert.Equal(t, "john", identity.Username)
	assert.Equal(t, "john@example.com", identity.Email)
	assert.Contains(t, identity.Groups, "admins")
	assert.Equal(t, "cn=john,ou=users,dc=example,dc=com", identity.Attributes["dn"])
}

func TestMapper_ExtractCNFromDN(t *testing.T) {
	config := &MapperConfig{}
	mapper := NewMapper(config)

	tests := []struct {
		name string
		dn   string
		want string
	}{
		{
			name: "simple DN",
			dn:   "cn=admin,dc=example,dc=com",
			want: "admin",
		},
		{
			name: "complex DN",
			dn:   "cn=john doe,ou=users,ou=people,dc=example,dc=com",
			want: "john doe",
		},
		{
			name: "case insensitive CN",
			dn:   "CN=Jane,dc=example,dc=com",
			want: "Jane",
		},
		{
			name: "no CN",
			dn:   "uid=123,dc=example,dc=com",
			want: "uid=123,dc=example,dc=com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapper.extractCNFromDN(tt.dn)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestMapper_ExtractDomainFromDN(t *testing.T) {
	config := &MapperConfig{}
	mapper := NewMapper(config)

	tests := []struct {
		name string
		dn   string
		want string
	}{
		{
			name: "simple domain",
			dn:   "cn=admin,dc=example,dc=com",
			want: "example.com",
		},
		{
			name: "subdomain",
			dn:   "cn=admin,ou=users,dc=sub,dc=example,dc=com",
			want: "sub.example.com",
		},
		{
			name: "case insensitive DC",
			dn:   "CN=admin,DC=CORP,DC=LOCAL",
			want: "corp.local",
		},
		{
			name: "no domain",
			dn:   "cn=admin,ou=users",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapper.extractDomainFromDN(tt.dn)
			assert.Equal(t, tt.want, result)
		})
	}
}
