package ldap

import (
	"context"
	"fmt"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/go-ldap/ldap/v3"
)

type Config struct {
	URL             string
	BindDN          string
	BindPassword    string
	BaseDN          string
	UserSelector    string
	GroupSelector   string
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
	mapper *Mapper
	conn   *ldap.Conn
}

func NewLDAPSource(config *Config, mapperConfig *MapperConfig) *LDAPSource {
	return &LDAPSource{
		config: config,
		mapper: NewMapper(mapperConfig),
	}
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

func (l *LDAPSource) GetIdentities(ctx context.Context) (map[string]source.Identity, error) {
	searchRequest := ldap.NewSearchRequest(
		l.config.BaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0, 0, false,
		l.config.UserSelector,
		l.config.UserAttributes,
		nil,
	)

	sr, err := l.conn.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("LDAP search failed: %w", err)
	}

	identities := make(map[string]source.Identity)
	for _, entry := range sr.Entries {
		identities[entry.DN] = l.mapper.MapIdentity(entry)
	}
	return identities, nil
}

func (l *LDAPSource) GetGroups(ctx context.Context) (map[string]source.Group, error) {
	searchRequest := ldap.NewSearchRequest(
		l.config.BaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0, 0, false,
		l.config.GroupSelector,
		l.config.GroupAttributes,
		nil,
	)

	sr, err := l.conn.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("LDAP group search failed: %w", err)
	}

	groups := make(map[string]source.Group)
	for _, entry := range sr.Entries {
		groups[entry.DN] = l.mapper.MapGroup(entry)
	}
	return groups, nil
}

func (l *LDAPSource) Watch(ctx context.Context) (<-chan source.Event, error) {
	events := make(chan source.Event)
	combinedFilter := fmt.Sprintf("(|%s%s)", l.config.UserSelector, l.config.GroupSelector)
	allAttributes := append(l.config.UserAttributes, l.config.GroupAttributes...)

	go func() {
		defer close(events)

		// Attempt RFC 4533 (SyncRepl)
		syncControl := ldap.NewControlSyncRequest(ldap.SyncRequestModeRefreshAndPersist, nil, true)
		searchRequest := ldap.NewSearchRequest(
			l.config.BaseDN,
			ldap.ScopeWholeSubtree,
			ldap.NeverDerefAliases,
			0, 0, false,
			combinedFilter,
			allAttributes,
			[]ldap.Control{syncControl},
		)

		sr, err := l.conn.Search(searchRequest)

		if err != nil && ldap.IsErrorWithCode(err, ldap.LDAPResultUnavailableCriticalExtension) {
			l.runPollingWatch(ctx, events, combinedFilter, allAttributes)
			return
		}

		if err == nil {
			for _, entry := range sr.Entries {
				l.sendEvent(ctx, events, entry)
			}
		}

		for {
			select {
			case <-ctx.Done():
				return
			default:
				if err != nil {
					time.Sleep(10 * time.Second)
					_ = l.Connect(ctx)
				}
				sr, err = l.conn.Search(searchRequest)
				if err == nil {
					for _, entry := range sr.Entries {
						l.sendEvent(ctx, events, entry)
					}
				}
			}
		}
	}()

	return events, nil
}

func (l *LDAPSource) runPollingWatch(ctx context.Context, events chan<- source.Event, filter string, attrs []string) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	searchRequest := ldap.NewSearchRequest(
		l.config.BaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0, 0, false,
		filter,
		attrs,
		nil,
	)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sr, err := l.conn.Search(searchRequest)
			if err != nil {
				_ = l.Connect(ctx)
				continue
			}

			for _, entry := range sr.Entries {
				l.sendEvent(ctx, events, entry)
			}
		}
	}
}

func (l *LDAPSource) sendEvent(ctx context.Context, events chan<- source.Event, entry *ldap.Entry) {
	event := l.processSyncEntry(entry)
	select {
	case events <- event:
	case <-ctx.Done():
	}
}

func (l *LDAPSource) processSyncEntry(entry *ldap.Entry) source.Event {
	isGroup := entry.GetAttributeValue(l.mapper.config.GIDAttribute) != ""

	now := time.Now().Unix()

	if isGroup {
		g := l.mapper.MapGroup(entry)
		return source.Event{
			Type:      source.EventUpdate,
			Group:     &g,
			Timestamp: now,
		}
	}

	i := l.mapper.MapIdentity(entry)
	return source.Event{
		Type:      source.EventUpdate,
		Identity:  &i,
		Timestamp: now,
	}
}

func (l *LDAPSource) Close() error {
	if l.conn != nil {
		return l.conn.Close()
	}
	return nil
}
