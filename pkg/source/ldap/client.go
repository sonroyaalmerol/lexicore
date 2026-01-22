package ldap

import (
	"context"
	"fmt"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/go-ldap/ldap/v3"
)

type Config struct {
	URL             string
	BindDN          string
	BindPassword    string
	BaseDN          string
	UserFilter      string
	GroupFilter     string
	UserAttributes  []string
	GroupAttributes []string
	TLSConfig       *TLSConfig
}

type TLSConfig struct {
	Enabled            bool
	InsecureSkipVerify bool
	CertFile           string
	KeyFile            string
	CAFile             string
}

type LDAPSource struct {
	config *Config
	conn   *ldap.Conn
}

func NewLDAPSource(config *Config) *LDAPSource {
	return &LDAPSource{config: config}
}

func (l *LDAPSource) Connect(ctx context.Context) error {
	conn, err := ldap.DialURL(l.config.URL)
	if err != nil {
		return fmt.Errorf("failed to dial LDAP: %w", err)
	}

	if l.config.TLSConfig != nil && l.config.TLSConfig.Enabled {
		if err := conn.StartTLS(nil); err != nil {
			return fmt.Errorf("failed to start TLS: %w", err)
		}
	}

	if err := conn.Bind(l.config.BindDN, l.config.BindPassword); err != nil {
		return fmt.Errorf("failed to bind: %w", err)
	}

	l.conn = conn
	return nil
}

func (l *LDAPSource) GetIdentities(ctx context.Context) ([]source.Identity, error) {
	searchRequest := ldap.NewSearchRequest(
		l.config.BaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		l.config.UserFilter,
		l.config.UserAttributes,
		nil,
	)

	sr, err := l.conn.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("LDAP search failed: %w", err)
	}

	identities := make([]source.Identity, 0, len(sr.Entries))
	for _, entry := range sr.Entries {
		identity := source.Identity{
			UID:        entry.GetAttributeValue("uid"),
			Username:   entry.GetAttributeValue("cn"),
			Email:      entry.GetAttributeValue("mail"),
			Groups:     entry.GetAttributeValues("memberOf"),
			Attributes: make(map[string]any),
		}

		for _, attr := range entry.Attributes {
			if len(attr.Values) == 1 {
				identity.Attributes[attr.Name] = attr.Values[0]
			} else {
				identity.Attributes[attr.Name] = attr.Values
			}
		}

		identities = append(identities, identity)
	}

	return identities, nil
}

func (l *LDAPSource) GetGroups(ctx context.Context) ([]source.Group, error) {
	searchRequest := ldap.NewSearchRequest(
		l.config.BaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		l.config.GroupFilter,
		l.config.GroupAttributes,
		nil,
	)

	sr, err := l.conn.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("LDAP group search failed: %w", err)
	}

	groups := make([]source.Group, 0, len(sr.Entries))
	for _, entry := range sr.Entries {
		group := source.Group{
			GID:         entry.GetAttributeValue("gidNumber"),
			Name:        entry.GetAttributeValue("cn"),
			Members:     entry.GetAttributeValues("member"),
			Description: entry.GetAttributeValue("description"),
			Attributes:  make(map[string]any),
		}

		for _, attr := range entry.Attributes {
			if len(attr.Values) == 1 {
				group.Attributes[attr.Name] = attr.Values[0]
			} else {
				group.Attributes[attr.Name] = attr.Values
			}
		}

		groups = append(groups, group)
	}

	return groups, nil
}

func (l *LDAPSource) Watch(ctx context.Context) (<-chan source.Event, error) {
	// TODO: implement LDAP persistent search or sync repl
	events := make(chan source.Event)
	return events, nil
}

func (l *LDAPSource) Close() error {
	if l.conn != nil {
		l.conn.Close()
	}
	return nil
}
