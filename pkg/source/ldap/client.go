package ldap

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/go-ldap/ldap/v3"
)

type config struct {
	URL             string
	BindDN          string
	BindPassword    string
	BaseDN          string
	UserSelector    string
	GroupSelector   string
	UserAttributes  []string
	GroupAttributes []string
	TLSConfig       *tlsConfig

	EnableChangeDetection bool
	ModifyTimestampAttr   string // e.g., "modifyTimestamp"
	CreateTimestampAttr   string // e.g., "createTimestamp"
}

type tlsConfig struct {
	Enabled            bool
	InsecureSkipVerify bool
	CertFile           string
	KeyFile            string
	CAFile             string
}

type LDAPSource struct {
	*source.BaseSource

	mu     sync.Mutex
	config *config
	mapper *mapper
	conn   *ldap.Conn

	supportsChangeDetection bool
	lastSyncTime            time.Time
}

func (l *LDAPSource) Initialize(ctx context.Context, config map[string]any) error {
	l.SetConfig(config)

	return l.Validate(ctx)
}

func (l *LDAPSource) Validate(ctx context.Context) error {
	cfg, mCfg, err := parseConfig(l.GetRawConfig())
	if err != nil {
		return err
	}

	l.mu.Lock()
	l.config = cfg
	l.mapper = newMapper(mCfg)

	if cfg.EnableChangeDetection {
		if cfg.ModifyTimestampAttr == "" {
			cfg.ModifyTimestampAttr = "modifyTimestamp"
		}
		if cfg.CreateTimestampAttr == "" {
			cfg.CreateTimestampAttr = "createTimestamp"
		}
		l.supportsChangeDetection = true
	}
	l.mu.Unlock()

	return nil
}

func (l *LDAPSource) Connect(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	conn, err := ldap.DialURL(l.config.URL)
	if err != nil {
		return fmt.Errorf("failed to dial LDAP: %w", err)
	}

	if l.config.TLSConfig != nil && l.config.TLSConfig.Enabled {
		if err := conn.StartTLS(nil); err != nil {
			conn.Close()
			return fmt.Errorf("failed to start TLS: %w", err)
		}
	}

	if err := conn.Bind(l.config.BindDN, l.config.BindPassword); err != nil {
		conn.Close()
		return fmt.Errorf("failed to bind: %w", err)
	}

	l.conn = conn
	return nil
}

func (l *LDAPSource) GetIdentities(ctx context.Context) (map[string]source.Identity, error) {
	l.mu.Lock()
	config := l.config
	conn := l.conn
	l.mu.Unlock()

	if conn == nil {
		return nil, fmt.Errorf("connection not established")
	}

	searchRequest := ldap.NewSearchRequest(
		config.BaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0, 0, false,
		config.UserSelector,
		config.UserAttributes,
		nil,
	)

	sr, err := conn.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("LDAP search failed: %w", err)
	}

	identities := make(map[string]source.Identity, len(sr.Entries))
	for _, entry := range sr.Entries {
		identity := l.mapper.mapIdentity(entry)
		key := l.identityKey(identity)
		identities[key] = identity
	}

	return identities, nil
}

func (l *LDAPSource) GetGroups(ctx context.Context) (map[string]source.Group, error) {
	l.mu.Lock()
	config := l.config
	conn := l.conn
	l.mu.Unlock()

	if conn == nil {
		return nil, fmt.Errorf("connection not established")
	}

	searchRequest := ldap.NewSearchRequest(
		config.BaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0, 0, false,
		config.GroupSelector,
		config.GroupAttributes,
		nil,
	)

	sr, err := conn.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("LDAP group search failed: %w", err)
	}

	groups := make(map[string]source.Group, len(sr.Entries))
	for _, entry := range sr.Entries {
		group := l.mapper.mapGroup(entry)
		key := l.groupKey(group)
		groups[key] = group
	}

	return groups, nil
}

func (l *LDAPSource) Close() error {
	l.mu.Lock()
	conn := l.conn
	l.conn = nil
	l.mu.Unlock()

	if conn != nil {
		return conn.Close()
	}
	return nil
}

func (l *LDAPSource) SupportsChangeDetection() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.supportsChangeDetection
}

func (l *LDAPSource) GetChangesSince(
	ctx context.Context,
	since time.Time,
) (*source.Changes, time.Time, error) {
	l.mu.Lock()
	config := l.config
	conn := l.conn
	l.mu.Unlock()

	if conn == nil {
		return nil, time.Time{}, fmt.Errorf("connection not established")
	}

	if !l.supportsChangeDetection {
		return nil, time.Time{}, fmt.Errorf("change detection not enabled")
	}

	currentTime := time.Now()
	changes := &source.Changes{
		ModifiedIdentities: make([]source.Identity, 0),
		DeletedIdentities:  make([]string, 0),
		ModifiedGroups:     make([]source.Group, 0),
		DeletedGroups:      make([]string, 0),
		FullSync:           false,
	}

	if since.IsZero() {
		changes.FullSync = true
		return changes, currentTime, nil
	}

	// LDAP uses format: YYYYMMDDHHMMSSz
	sinceStr := since.UTC().Format("20060102150405.0Z")

	userFilter := fmt.Sprintf(
		"(&%s(|(%s>=%s)(%s>=%s)))",
		config.UserSelector,
		config.ModifyTimestampAttr,
		sinceStr,
		config.CreateTimestampAttr,
		sinceStr,
	)

	userAttrs := append([]string{}, config.UserAttributes...)
	userAttrs = appendIfMissing(userAttrs, config.ModifyTimestampAttr)
	userAttrs = appendIfMissing(userAttrs, config.CreateTimestampAttr)

	userSearchRequest := ldap.NewSearchRequest(
		config.BaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0, 0, false,
		userFilter,
		userAttrs,
		nil,
	)

	userResults, err := conn.Search(userSearchRequest)
	if err != nil {
		// If the filter fails, fall back to full sync
		changes.FullSync = true
		return changes, currentTime, nil
	}

	for _, entry := range userResults.Entries {
		identity := l.mapper.mapIdentity(entry)
		changes.ModifiedIdentities = append(changes.ModifiedIdentities, identity)
	}

	// Search for modified/created groups
	groupFilter := fmt.Sprintf(
		"(&%s(|(%s>=%s)(%s>=%s)))",
		config.GroupSelector,
		config.ModifyTimestampAttr,
		sinceStr,
		config.CreateTimestampAttr,
		sinceStr,
	)

	groupAttrs := append([]string{}, config.GroupAttributes...)
	groupAttrs = appendIfMissing(groupAttrs, config.ModifyTimestampAttr)
	groupAttrs = appendIfMissing(groupAttrs, config.CreateTimestampAttr)

	groupSearchRequest := ldap.NewSearchRequest(
		config.BaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0, 0, false,
		groupFilter,
		groupAttrs,
		nil,
	)

	groupResults, err := conn.Search(groupSearchRequest)
	if err != nil {
		// If the filter fails, fall back to full sync
		changes.FullSync = true
		return changes, currentTime, nil
	}

	for _, entry := range groupResults.Entries {
		group := l.mapper.mapGroup(entry)
		changes.ModifiedGroups = append(changes.ModifiedGroups, group)
	}

	// Note: LDAP doesn't provide easy deletion detection without tombstone support
	// or persistent search. For basic change detection, we only track modifications.
	// Deletions will be detected by the cache diff when doing periodic full syncs.
	// - Persistent Search (RFC 3673)
	// - Content Sync Operation (RFC 4533)
	// - Tombstone tracking if available

	l.mu.Lock()
	l.lastSyncTime = currentTime
	l.mu.Unlock()

	return changes, currentTime, nil
}

func (l *LDAPSource) identityKey(identity source.Identity) string {
	if identity.UID != "" {
		return identity.UID
	}
	if identity.Username != "" {
		return identity.Username
	}
	return identity.Email
}

func (l *LDAPSource) groupKey(group source.Group) string {
	if group.GID != "" {
		return group.GID
	}
	return group.Name
}

func appendIfMissing(slice []string, item string) []string {
	if slices.Contains(slice, item) {
		return slice
	}

	return append(slice, item)
}
