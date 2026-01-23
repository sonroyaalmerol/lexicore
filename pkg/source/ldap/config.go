package ldap

import (
	"fmt"
	"strings"

	"codeberg.org/lexicore/lexicore/pkg/source"
)

func init() {
	source.Register("ldap", func(rawConfig map[string]any) (source.Source, error) {
		cfg, mCfg, err := ParseConfig(rawConfig)
		if err != nil {
			return nil, err
		}
		return NewLDAPSource(cfg, mCfg), nil
	})
}

func ParseConfig(raw map[string]any) (*Config, *MapperConfig, error) {
	cfg := &Config{
		URL:           getString(raw, "url"),
		BindDN:        getString(raw, "bindDN"),
		BindPassword:  getString(raw, "bindPassword"),
		BaseDN:        getString(raw, "baseDN"),
		UserSelector:  getStringOrDefault(raw, "userSelector", "(objectClass=person)"),
		GroupSelector: getStringOrDefault(raw, "groupSelector", "(objectClass=groupOfNames)"),
	}

	if cfg.URL == "" || cfg.BaseDN == "" {
		return nil, nil, fmt.Errorf("ldap config: 'url' and 'baseDN' are required fields")
	}

	cfg.UserAttributes = getStringSlice(raw, "userAttributes")
	cfg.GroupAttributes = getStringSlice(raw, "groupAttributes")

	ensureAttribute(&cfg.UserAttributes, "uid")
	ensureAttribute(&cfg.UserAttributes, "cn")
	ensureAttribute(&cfg.GroupAttributes, "gidNumber")
	ensureAttribute(&cfg.GroupAttributes, "cn")

	if tlsRaw, ok := raw["tls"].(map[string]any); ok {
		cfg.TLSConfig = &TLSConfig{
			Enabled:            getBool(tlsRaw, "enabled"),
			InsecureSkipVerify: getBool(tlsRaw, "insecureSkipVerify"),
			CAFile:             getString(tlsRaw, "caFile"),
		}
	}

	mCfg, _ := ParseMapperConfig(raw)
	return cfg, mCfg, nil
}

func ensureAttribute(list *[]string, attr string) {
	for _, a := range *list {
		if strings.EqualFold(a, attr) {
			return
		}
	}
	*list = append(*list, attr)
}

func ParseMapperConfig(c map[string]any) (*MapperConfig, error) {
	return &MapperConfig{
		UIDAttribute:              getString(c, "uidAttribute"),
		UsernameAttribute:         getString(c, "usernameAttribute"),
		EmailAttribute:            getString(c, "emailAttribute"),
		GroupsAttribute:           getString(c, "groupsAttribute"),
		DisplayNameAttribute:      getString(c, "displayNameAttribute"),
		GIDAttribute:              getString(c, "gidAttribute"),
		GroupNameAttribute:        getString(c, "groupNameAttribute"),
		GroupMembersAttribute:     getString(c, "groupMembersAttribute"),
		GroupDescriptionAttribute: getString(c, "groupDescriptionAttribute"),
		ExtractDomainFromDN:       getBool(c, "extractDomainFromDN"),
	}, nil
}

func getString(m map[string]any, k string) string {
	v, _ := m[k].(string)
	return v
}

func getStringOrDefault(m map[string]any, k, d string) string {
	if v, ok := m[k].(string); ok && v != "" {
		return v
	}
	return d
}

func getBool(m map[string]any, k string) bool {
	v, _ := m[k].(bool)
	return v
}

func getStringSlice(m map[string]any, k string) []string {
	if raw, ok := m[k].([]any); ok {
		s := make([]string, 0, len(raw))
		for _, v := range raw {
			if str, ok := v.(string); ok {
				s = append(s, str)
			}
		}
		return s
	}
	return nil
}
