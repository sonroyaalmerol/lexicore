package ldap

import (
	"fmt"
	"strings"

	"codeberg.org/lexicore/lexicore/pkg/manifest"
)

func parseConfig(raw map[string]manifest.ConfigValue) (*config, *mapperConfig, error) {
	cfg := &config{
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
	ensureAttribute(&cfg.GroupAttributes, "memberOf")

	if tlsRaw, ok := raw["tls"].Value().(map[string]manifest.ConfigValue); ok {
		cfg.TLSConfig = &tlsConfig{
			Enabled:            getBool(tlsRaw, "enabled"),
			InsecureSkipVerify: getBool(tlsRaw, "insecureSkipVerify"),
			CAFile:             getString(tlsRaw, "caFile"),
		}
	}

	mCfg, _ := parseMapperConfig(raw)
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

func parseMapperConfig(c map[string]manifest.ConfigValue) (*mapperConfig, error) {
	return &mapperConfig{
		UIDAttribute:              getString(c, "uidAttribute"),
		UsernameAttribute:         getString(c, "usernameAttribute"),
		EmailAttribute:            getString(c, "emailAttribute"),
		GroupsAttribute:           getString(c, "groupsAttribute"),
		DisplayNameAttribute:      getString(c, "displayNameAttribute"),
		GIDAttribute:              getString(c, "gidAttribute"),
		GroupNameAttribute:        getString(c, "groupNameAttribute"),
		GroupMembersAttribute:     getString(c, "groupMembersAttribute"),
		GroupDescriptionAttribute: getString(c, "groupDescriptionAttribute"),
		GroupParentsAttribute:     getString(c, "groupParentsAttribute"),
		ExtractDomainFromDN:       getBool(c, "extractDomainFromDN"),
	}, nil
}

func getString(m map[string]manifest.ConfigValue, k string) string {
	v, _ := m[k].Value().(string)
	return v
}

func getStringOrDefault(m map[string]manifest.ConfigValue, k, d string) string {
	if v, ok := m[k].Value().(string); ok && v != "" {
		return v
	}
	return d
}

func getBool(m map[string]manifest.ConfigValue, k string) bool {
	v, _ := m[k].Value().(bool)
	return v
}

func getStringSlice(m map[string]manifest.ConfigValue, k string) []string {
	if raw, ok := m[k].Value().([]any); ok {
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
