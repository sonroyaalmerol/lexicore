package iredadmin

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"codeberg.org/lexicore/lexicore/pkg/source"
)

// manifest to iredadmin userdata
func (o *IRedAdminOperator) identityToUser(identity source.Identity) UserData {
	u := UserData{}
	u.CN = []string{identity.DisplayName}
	prefix := o.GetAttributePrefix()

	for k, v := range identity.Attributes {
		fieldName, hasPrefix := strings.CutPrefix(k, prefix)
		if !hasPrefix {
			continue
		}

		if v == nil {
			continue
		}

		var values []string
		switch val := v.(type) {
		case string:
			values = []string{val}
		case int:
			if fieldName == AttributeQuota {
				values = []string{strconv.Itoa(val)}
			}
		case int32:
			if fieldName == AttributeQuota {
				values = []string{strconv.Itoa(int(val))}
			}
		case int64:
			if fieldName == AttributeQuota {
				values = []string{strconv.Itoa(int(val))}
			}
		case bool:
			if val {
				values = []string{"true"}
			} else {
				values = []string{"false"}
			}
		case []string:
			values = val
		case []any:
			for _, item := range val {
				switch val2 := item.(type) {
				case string:
					values = append(values, val2)
				case int:
					values = append(values, strconv.Itoa(val2))
				case int32:
					values = append(values, strconv.Itoa(int(val2)))
				case int64:
					values = append(values, strconv.Itoa(int(val2)))
				case bool:
					if val2 {
						values = append(values, "true")
					} else {
						values = append(values, "false")
					}
				default:
					o.LogError(fmt.Errorf("unsupported type for attribute k array element: %T", item))
					continue
				}
			}
		default:
			o.LogError(fmt.Errorf("unsupported type for attribute %s: %T", k, v))
			continue
		}

		if len(values) > 0 {
			slices.Sort(values)

			switch fieldName {
			case AttributeGivenName:
				u.GivenName = values
			case AttributeSN:
				u.SN = values
			case AttributeLanguage:
				u.PreferredLanguage = values
			case AttributeQuota:
				u.MailQuota = values
			case AttributeStatus:
				if len(values) > 0 && values[0] != "true" {
					u.AccountStatus = []string{"disabled"}
				} else {
					u.AccountStatus = []string{"active"}
				}
			case AttributeForwardingAddresses:
				u.MailForwardingAddress = values
			case AttributeEnabledServices:
				u.EnabledService = values
			case AttributeMailingLists:
				u.MailingLists = values
			case AttributeAliases:
				u.MailingAliases = values
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
