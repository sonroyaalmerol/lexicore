package iredadmin

import (
	"strconv"
	"strings"

	"codeberg.org/lexicore/lexicore/pkg/source"
)

func (o *IRedAdminOperator) GetAttributePrefix() string {
	if o.attrPrefix != nil {
		return *o.attrPrefix
	}

	if prefix, err := o.BaseOperator.GetStringConfig("attributePrefix"); err == nil {
		o.attrPrefix = &prefix
		return prefix
	}

	return ""
}
func (o *IRedAdminOperator) getConcurrency() int {
	workers := 10
	if w, ok := o.GetConfig("concurrency"); ok {
		if wInt, ok := w.(int); ok && wInt > 0 {
			workers = wInt
		} else if wFloat, ok := w.(float64); ok && wFloat > 0 {
			workers = int(wFloat)
		}
	}
	return workers
}

// manifest to iredadmin userdata
func (o *IRedAdminOperator) identityToUser(identity source.Identity) UserData {
	u := UserData{}
	u.CN = []string{identity.DisplayName}
	for k, v := range identity.Attributes {
		fieldName, hasPrefix := strings.CutPrefix(k, o.GetAttributePrefix())
		if !hasPrefix {
			continue
		}
		if vString, ok := v.(string); ok {
			switch fieldName {
			case AttributeGivenName:
				u.GivenName = []string{vString}
			case AttributeSN:
				u.SN = []string{vString}
			case AttributeLanguage:
				u.PreferredLanguage = []string{vString}
			case AttributeQuota:
				u.MailQuota = []string{vString}
			case AttributeStatus:
				u.AccountStatus = []string{vString}
			}
		}
		if vStrArray, ok := v.([]string); ok {
			switch fieldName {
			case AttributeForwardingAddresses:
				u.MailForwardingAddress = vStrArray
			case AttributeEnabledServices:
				u.EnabledService = vStrArray
			case AttributeMailingLists:
				u.MailingLists = vStrArray
			case AttributeAliases:
				u.MailingAliases = vStrArray
			}
		}
		if vInt, ok := v.(int); ok {
			switch fieldName {
			case AttributeQuota:
				u.MailQuota = []string{strconv.Itoa(vInt)}
			}
		}
	}

	return u
}

func stringIsEqual(a []string, b []string) bool {
	return getStringFromArray(a) == getStringFromArray(b)
}

func getStringFromArray(arr []string) string {
	if len(arr) > 0 {
		return arr[0]
	}

	return ""
}
