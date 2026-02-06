package ldap

import (
	"strings"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/go-ldap/ldap/v3"
)

type mapper struct {
	config *mapperConfig
}

type mapperConfig struct {
	UIDAttribute         string
	UsernameAttribute    string
	EmailAttribute       string
	GroupsAttribute      string
	DisplayNameAttribute string

	GIDAttribute              string
	GroupNameAttribute        string
	GroupMembersAttribute     string
	GroupDescriptionAttribute string

	ExtractDomainFromDN bool
}

func newMapper(config *mapperConfig) *mapper {
	setDefaults(config)
	return &mapper{config: config}
}

func setDefaults(c *mapperConfig) {
	if c.UIDAttribute == "" {
		c.UIDAttribute = "uid"
	}
	if c.UsernameAttribute == "" {
		c.UsernameAttribute = "cn"
	}
	if c.EmailAttribute == "" {
		c.EmailAttribute = "mail"
	}
	if c.GroupsAttribute == "" {
		c.GroupsAttribute = "memberOf"
	}
	if c.DisplayNameAttribute == "" {
		c.DisplayNameAttribute = "displayName"
	}
	if c.GIDAttribute == "" {
		c.GIDAttribute = "gidNumber"
	}
	if c.GroupNameAttribute == "" {
		c.GroupNameAttribute = "cn"
	}
	if c.GroupMembersAttribute == "" {
		c.GroupMembersAttribute = "member"
	}
	if c.GroupDescriptionAttribute == "" {
		c.GroupDescriptionAttribute = "description"
	}
}

func (m *mapper) mapIdentity(entry *ldap.Entry) source.Identity {
	identity := source.Identity{
		UID:         entry.GetAttributeValue(m.config.UIDAttribute),
		Username:    entry.GetAttributeValue(m.config.UsernameAttribute),
		Email:       entry.GetAttributeValue(m.config.EmailAttribute),
		DisplayName: entry.GetAttributeValue(m.config.DisplayNameAttribute),
		Groups:      entry.GetAttributeValues(m.config.GroupsAttribute),
		Attributes:  make(map[string]any),
	}

	for _, attr := range entry.Attributes {
		if len(attr.Values) == 1 {
			identity.Attributes[attr.Name] = attr.Values[0]
		} else {
			identity.Attributes[attr.Name] = attr.Values
		}
	}

	identity.Attributes["dn"] = entry.DN

	if m.config.ExtractDomainFromDN {
		if domain := m.extractDomainFromDN(entry.DN); domain != "" {
			identity.Attributes["domain"] = domain
		}
	}

	for i, group := range identity.Groups {
		identity.Groups[i] = m.extractCNFromDN(group)
	}

	return identity
}

func (m *mapper) mapGroup(entry *ldap.Entry) source.Group {
	group := source.Group{
		GID:         entry.GetAttributeValue(m.config.GIDAttribute),
		Name:        entry.GetAttributeValue(m.config.GroupNameAttribute),
		Description: entry.GetAttributeValue(m.config.GroupDescriptionAttribute),
		Members:     entry.GetAttributeValues(m.config.GroupMembersAttribute),
		Attributes:  make(map[string]any),
	}

	for _, attr := range entry.Attributes {
		if len(attr.Values) == 1 {
			group.Attributes[attr.Name] = attr.Values[0]
		} else {
			group.Attributes[attr.Name] = attr.Values
		}
	}

	group.Attributes["dn"] = entry.DN

	for i, member := range group.Members {
		group.Members[i] = m.extractCNFromDN(member)
	}

	return group
}

func (m *mapper) extractDomainFromDN(dn string) string {
	parts := strings.Split(dn, ",")
	var domainParts []string
	for _, part := range parts {
		part = strings.ToLower(strings.TrimSpace(part))
		if strings.HasPrefix(part, "dc=") {
			domainParts = append(domainParts, part[3:])
		}
	}
	return strings.Join(domainParts, ".")
}

func (m *mapper) extractCNFromDN(dn string) string {
	parts := strings.Split(dn, ",")
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(trimmed), "cn=") {
			return trimmed[3:]
		}
	}
	return dn
}
