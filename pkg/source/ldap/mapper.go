package ldap

import (
	"strings"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/go-ldap/ldap/v3"
)

type Mapper struct {
	config *MapperConfig
}

type MapperConfig struct {
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

func NewMapper(config *MapperConfig) *Mapper {
	if config.UIDAttribute == "" {
		config.UIDAttribute = "uid"
	}
	if config.UsernameAttribute == "" {
		config.UsernameAttribute = "cn"
	}
	if config.EmailAttribute == "" {
		config.EmailAttribute = "mail"
	}
	if config.GroupsAttribute == "" {
		config.GroupsAttribute = "memberOf"
	}
	if config.DisplayNameAttribute == "" {
		config.DisplayNameAttribute = "displayName"
	}
	if config.GIDAttribute == "" {
		config.GIDAttribute = "gidNumber"
	}
	if config.GroupNameAttribute == "" {
		config.GroupNameAttribute = "cn"
	}
	if config.GroupMembersAttribute == "" {
		config.GroupMembersAttribute = "member"
	}
	if config.GroupDescriptionAttribute == "" {
		config.GroupDescriptionAttribute = "description"
	}

	return &Mapper{config: config}
}

func (m *Mapper) ConstantIdentity(entry *ldap.Entry) source.Identity {
	identity := source.Identity{
		UID:         m.getAttributeValue(entry, m.config.UIDAttribute),
		Username:    m.getAttributeValue(entry, m.config.UsernameAttribute),
		Email:       m.getAttributeValue(entry, m.config.EmailAttribute),
		DisplayName: m.getAttributeValue(entry, m.config.DisplayNameAttribute),
		Groups:      m.getAttributeValues(entry, m.config.GroupsAttribute),
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
		domain := m.extractDomainFromDN(entry.DN)
		if domain != "" {
			identity.Attributes["domain"] = domain
		}
	}

	normalizedGroups := make([]string, 0, len(identity.Groups))
	for _, group := range identity.Groups {
		normalizedGroups = append(normalizedGroups, m.extractCNFromDN(group))
	}
	identity.Groups = normalizedGroups

	return identity
}

func (m *Mapper) ConstantGroup(entry *ldap.Entry) source.Group {
	group := source.Group{
		GID:         m.getAttributeValue(entry, m.config.GIDAttribute),
		Name:        m.getAttributeValue(entry, m.config.GroupNameAttribute),
		Description: m.getAttributeValue(entry, m.config.GroupDescriptionAttribute),
		Members:     m.getAttributeValues(entry, m.config.GroupMembersAttribute),
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

	normalizedMembers := make([]string, 0, len(group.Members))
	for _, member := range group.Members {
		normalizedMembers = append(normalizedMembers, m.extractCNFromDN(member))
	}
	group.Members = normalizedMembers

	return group
}

func (m *Mapper) getAttributeValue(entry *ldap.Entry, attribute string) string {
	return entry.GetAttributeValue(attribute)
}

func (m *Mapper) getAttributeValues(entry *ldap.Entry, attribute string) []string {
	return entry.GetAttributeValues(attribute)
}

func (m *Mapper) extractDomainFromDN(dn string) string {
	parts := strings.Split(dn, ",")
	var domainParts []string

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(part), "dc=") {
			domainParts = append(domainParts, strings.TrimPrefix(part[3:], ""))
		}
	}

	return strings.Join(domainParts, ".")
}

func (m *Mapper) extractCNFromDN(dn string) string {
	parts := strings.Split(dn, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(part), "cn=") {
			return part[3:]
		}
	}
	return dn
}

func ParseMapperConfig(config map[string]any) (*MapperConfig, error) {
	mapperConfig := &MapperConfig{}

	if v, ok := config["uidAttribute"].(string); ok {
		mapperConfig.UIDAttribute = v
	}
	if v, ok := config["usernameAttribute"].(string); ok {
		mapperConfig.UsernameAttribute = v
	}
	if v, ok := config["emailAttribute"].(string); ok {
		mapperConfig.EmailAttribute = v
	}
	if v, ok := config["groupsAttribute"].(string); ok {
		mapperConfig.GroupsAttribute = v
	}
	if v, ok := config["displayNameAttribute"].(string); ok {
		mapperConfig.DisplayNameAttribute = v
	}
	if v, ok := config["gidAttribute"].(string); ok {
		mapperConfig.GIDAttribute = v
	}
	if v, ok := config["groupNameAttribute"].(string); ok {
		mapperConfig.GroupNameAttribute = v
	}
	if v, ok := config["groupMembersAttribute"].(string); ok {
		mapperConfig.GroupMembersAttribute = v
	}
	if v, ok := config["groupDescriptionAttribute"].(string); ok {
		mapperConfig.GroupDescriptionAttribute = v
	}
	if v, ok := config["extractDomainFromDN"].(bool); ok {
		mapperConfig.ExtractDomainFromDN = v
	}

	return mapperConfig, nil
}
