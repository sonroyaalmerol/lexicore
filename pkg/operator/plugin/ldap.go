package starlarklib

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/go-ldap/ldap/v3"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func makeLDAPModule(ctx context.Context) *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "ldap",
		Members: starlark.StringDict{
			"connect": starlark.NewBuiltin("ldap.connect", ldapConnect),
		},
	}
}

type ldapConn struct {
	conn *ldap.Conn
}

func (l *ldapConn) String() string        { return "ldap.Connection" }
func (l *ldapConn) Type() string          { return "ldap.Connection" }
func (l *ldapConn) Freeze()               {}
func (l *ldapConn) Truth() starlark.Bool  { return starlark.True }
func (l *ldapConn) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: ldap.Connection") }

func (l *ldapConn) Attr(name string) (starlark.Value, error) {
	switch name {
	case "bind":
		return starlark.NewBuiltin("bind", l.bind), nil
	case "search":
		return starlark.NewBuiltin("search", l.search), nil
	case "add":
		return starlark.NewBuiltin("add", l.add), nil
	case "modify":
		return starlark.NewBuiltin("modify", l.modify), nil
	case "delete":
		return starlark.NewBuiltin("delete", l.delete), nil
	case "close":
		return starlark.NewBuiltin("close", l.close), nil
	}
	return nil, nil
}

func (l *ldapConn) AttrNames() []string {
	return []string{"bind", "search", "add", "modify", "delete", "close"}
}

func ldapConnect(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var url string
	var useTLS, insecureSkipVerify bool

	if err := starlark.UnpackArgs(
		"ldap.connect",
		args,
		kwargs,
		"url", &url,
		"use_tls?", &useTLS,
		"insecure_skip_verify?", &insecureSkipVerify,
	); err != nil {
		return nil, err
	}

	var conn *ldap.Conn
	var err error

	if useTLS {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: insecureSkipVerify,
		}
		conn, err = ldap.DialTLS("tcp", url, tlsConfig)
	} else {
		conn, err = ldap.Dial("tcp", url)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to LDAP: %w", err)
	}

	return &ldapConn{conn: conn}, nil
}

func (l *ldapConn) bind(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var username, password string

	if err := starlark.UnpackArgs(
		"bind",
		args,
		kwargs,
		"username", &username,
		"password", &password,
	); err != nil {
		return nil, err
	}

	err := l.conn.Bind(username, password)
	if err != nil {
		return nil, fmt.Errorf("bind failed: %w", err)
	}

	return starlark.None, nil
}

func (l *ldapConn) search(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var baseDN, filter string
	var attributes *starlark.List
	var scope int = ldap.ScopeWholeSubtree

	if err := starlark.UnpackArgs(
		"search",
		args,
		kwargs,
		"base_dn", &baseDN,
		"filter", &filter,
		"attributes?", &attributes,
		"scope?", &scope,
	); err != nil {
		return nil, err
	}

	var attrs []string
	if attributes != nil {
		attrs = make([]string, attributes.Len())
		for i := 0; i < attributes.Len(); i++ {
			if str, ok := attributes.Index(i).(starlark.String); ok {
				attrs[i] = string(str)
			}
		}
	}

	searchRequest := ldap.NewSearchRequest(
		baseDN,
		scope,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		filter,
		attrs,
		nil,
	)

	sr, err := l.conn.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Convert entries to Starlark list
	entries := make([]starlark.Value, len(sr.Entries))
	for i, entry := range sr.Entries {
		entryDict := starlark.NewDict(len(entry.Attributes) + 1)
		entryDict.SetKey(starlark.String("dn"), starlark.String(entry.DN))

		for _, attr := range entry.Attributes {
			values := make([]starlark.Value, len(attr.Values))
			for j, v := range attr.Values {
				values[j] = starlark.String(v)
			}
			entryDict.SetKey(starlark.String(attr.Name), starlark.NewList(values))
		}

		entries[i] = entryDict
	}

	return starlark.NewList(entries), nil
}

func (l *ldapConn) add(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var dn string
	var attributes *starlark.Dict

	if err := starlark.UnpackArgs(
		"add",
		args,
		kwargs,
		"dn", &dn,
		"attributes", &attributes,
	); err != nil {
		return nil, err
	}

	addRequest := ldap.NewAddRequest(dn, nil)

	for _, item := range attributes.Items() {
		attrName, ok := item[0].(starlark.String)
		if !ok {
			continue
		}

		var values []string
		switch v := item[1].(type) {
		case starlark.String:
			values = []string{string(v)}
		case *starlark.List:
			values = make([]string, v.Len())
			for i := 0; i < v.Len(); i++ {
				if str, ok := v.Index(i).(starlark.String); ok {
					values[i] = string(str)
				}
			}
		}

		addRequest.Attribute(string(attrName), values)
	}

	err := l.conn.Add(addRequest)
	if err != nil {
		return nil, fmt.Errorf("add failed: %w", err)
	}

	return starlark.None, nil
}

func (l *ldapConn) modify(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var dn string
	var changes *starlark.Dict

	if err := starlark.UnpackArgs(
		"modify",
		args,
		kwargs,
		"dn", &dn,
		"changes", &changes,
	); err != nil {
		return nil, err
	}

	modifyRequest := ldap.NewModifyRequest(dn, nil)

	for _, item := range changes.Items() {
		attrName, ok := item[0].(starlark.String)
		if !ok {
			continue
		}

		changeDict, ok := item[1].(*starlark.Dict)
		if !ok {
			continue
		}

		opVal, found, _ := changeDict.Get(starlark.String("op"))
		if !found {
			continue
		}
		operation, _ := opVal.(starlark.String)

		valuesVal, found, _ := changeDict.Get(starlark.String("values"))
		if !found {
			continue
		}

		var values []string
		switch v := valuesVal.(type) {
		case starlark.String:
			values = []string{string(v)}
		case *starlark.List:
			values = make([]string, v.Len())
			for i := 0; i < v.Len(); i++ {
				if str, ok := v.Index(i).(starlark.String); ok {
					values[i] = string(str)
				}
			}
		}

		var op uint
		switch string(operation) {
		case "add":
			op = ldap.AddAttribute
		case "delete":
			op = ldap.DeleteAttribute
		case "replace":
			op = ldap.ReplaceAttribute
		default:
			return nil, fmt.Errorf("unknown operation: %s", operation)
		}

		modifyRequest.Replace(string(attrName), values)
		modifyRequest.Changes = append(modifyRequest.Changes, ldap.Change{
			Operation:    op,
			Modification: ldap.PartialAttribute{Type: string(attrName), Vals: values},
		})
	}

	err := l.conn.Modify(modifyRequest)
	if err != nil {
		return nil, fmt.Errorf("modify failed: %w", err)
	}

	return starlark.None, nil
}

func (l *ldapConn) delete(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var dn string

	if err := starlark.UnpackArgs("delete", args, kwargs, "dn", &dn); err != nil {
		return nil, err
	}

	deleteRequest := ldap.NewDelRequest(dn, nil)
	err := l.conn.Del(deleteRequest)
	if err != nil {
		return nil, fmt.Errorf("delete failed: %w", err)
	}

	return starlark.None, nil
}

func (l *ldapConn) close(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	l.conn.Close()
	return starlark.None, nil
}
