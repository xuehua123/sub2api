// Package geoip provides a local MaxMind GeoLite2 City (MMDB) reader used to
// enrich a verified public egress IP with a coarse-grained location (country,
// first-level subdivision, and city). All lookups are local; user IPs are never
// sent to a third party. The database file is opened once at startup and is not
// part of the request path's I/O.
package geoip

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"github.com/oschwald/maxminddb-golang"

	clientip "github.com/Wei-Shaw/sub2api/internal/pkg/ip"
)

const (
	// maxCountryCodeBytes bounds the ISO country code field.
	maxCountryCodeBytes = 8
	// maxLocationFieldBytes bounds each localized name field by UTF-8 bytes so
	// the serialized probe response stays well below the 1 KiB hard cap even
	// with four multi-byte fields.
	maxLocationFieldBytes = 96
)

// Location is the coarse-grained geographic estimate exposed to the browser.
// It intentionally excludes latitude, longitude, postal code, timezone, ASN,
// ISP, internal nodes, and any proxy-chain information.
type Location struct {
	CountryCode string
	Country     string
	Region      string
	City        string
}

// Resolver is the small interface the service layer depends on.
type Resolver interface {
	// Lookup returns a coarse location for a public address. Non-public or
	// invalid addresses return an error without touching the database. A
	// database miss returns (nil, nil); read failures return an error.
	Lookup(addr netip.Addr) (*Location, error)
	// Ready reports whether the underlying database is usable.
	Ready() bool
	// Close releases the database handle.
	Close() error
}

var _ Resolver = (*Reader)(nil)

// db is the subset of the maxminddb Reader used by this package. Abstracting it
// lets unit tests inject a fake database without shipping a real MMDB fixture.
type db interface {
	Lookup(ip net.IP, result any) error
	Close() error
}

// Reader is a Resolver backed by an opened MMDB file.
type Reader struct {
	mu     sync.RWMutex
	db     db
	locale string
}

// mmdbRecord decodes only the fields this feature needs. The maxminddb decoder
// uses reflection over struct tags, so extra database keys are ignored.
type mmdbRecord struct {
	Country struct {
		ISOCode string            `maxminddb:"iso_code"`
		Names   map[string]string `maxminddb:"names"`
	} `maxminddb:"country"`
	Subdivisions []struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"subdivisions"`
	City struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"city"`
}

// Open opens the MMDB file once. A configured-but-missing or corrupt file
// returns an error so the caller can log a clear warning and fail closed
// without aborting application startup.
func Open(path string, locale string) (*Reader, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("geoip database path is empty")
	}
	realDB, err := maxminddb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open geoip database %q: %w", path, err)
	}
	reader, err := newCityReader(realDB, realDB.Metadata.DatabaseType, locale)
	if err != nil {
		return nil, fmt.Errorf("open geoip database %q: %w", path, err)
	}
	return reader, nil
}

func newCityReader(database db, databaseType, locale string) (*Reader, error) {
	if !isCityDatabaseType(databaseType) {
		typeErr := fmt.Errorf("database type %q is not City-compatible", strings.TrimSpace(databaseType))
		if closeErr := database.Close(); closeErr != nil {
			return nil, errors.Join(typeErr, fmt.Errorf("close incompatible geoip database: %w", closeErr))
		}
		return nil, typeErr
	}
	return newReader(database, locale), nil
}

func isCityDatabaseType(databaseType string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(databaseType)), "city")
}

func newReader(db db, locale string) *Reader {
	if locale = strings.TrimSpace(locale); locale == "" {
		locale = "zh-CN"
	}
	return &Reader{db: db, locale: locale}
}

// Ready reports whether the database is usable.
func (r *Reader) Ready() bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.db != nil
}

// Close releases the database handle. It is safe to call multiple times.
func (r *Reader) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.db == nil {
		return nil
	}
	d := r.db
	r.db = nil
	return d.Close()
}

// Lookup resolves a public address. Non-public, invalid, or scoped addresses
// are rejected before any database access, so malformed input never triggers a
// query. A database miss (no record) returns (nil, nil); read failures return
// an error.
func (r *Reader) Lookup(addr netip.Addr) (*Location, error) {
	if r == nil {
		return nil, errors.New("geoip database is not ready")
	}
	if !addr.IsValid() || addr.Zone() != "" {
		return nil, errors.New("geoip lookup requires a valid address")
	}
	addr = addr.Unmap()
	if !clientip.IsPublicInternetAddr(addr) {
		return nil, errors.New("geoip lookup requires a public address")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.db == nil {
		return nil, errors.New("geoip database is not ready")
	}

	var record mmdbRecord
	if err := r.db.Lookup(addr.AsSlice(), &record); err != nil {
		return nil, fmt.Errorf("geoip lookup failed: %w", err)
	}
	if strings.TrimSpace(record.Country.ISOCode) == "" &&
		len(record.Country.Names) == 0 && len(record.Subdivisions) == 0 &&
		len(record.City.Names) == 0 {
		return nil, nil // No matching record.
	}

	// Sanitize every field before it leaves the reader: MMDB content is untrusted
	// data and must stay within the probe protocol's bounds (bounded length, no
	// abnormal control characters). A malformed database must never produce a
	// response the strict frontend would reject as protocol_error.
	location := &Location{
		CountryCode: sanitizeCountryCode(record.Country.ISOCode),
		Country:     sanitizeLocationField(r.pickName(record.Country.Names)),
	}
	if len(record.Subdivisions) > 0 {
		location.Region = sanitizeLocationField(r.pickName(record.Subdivisions[0].Names))
	}
	location.City = sanitizeLocationField(r.pickName(record.City.Names))
	return location, nil
}

// sanitizeCountryCode keeps at most 8 ASCII alphanumerics, uppercased. Anything
// else is dropped so a corrupt database cannot smuggle control characters into
// the probe response.
func sanitizeCountryCode(value string) string {
	out := make([]byte, 0, maxCountryCodeBytes)
	for _, r := range strings.ToUpper(strings.TrimSpace(value)) {
		if r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			out = append(out, byte(r))
			if len(out) >= maxCountryCodeBytes {
				break
			}
		}
	}
	return string(out)
}

// sanitizeLocationField strips abnormal control characters (C0/C1 and
// bidirectional controls) and truncates to a byte budget so the serialized
// probe response stays far below the 1 KiB hard cap.
func sanitizeLocationField(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	cleaned := strings.Map(func(r rune) rune {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) || isAbnormalControlRune(r) {
			return -1
		}
		return r
	}, value)
	cleaned = strings.TrimSpace(cleaned)
	if len(cleaned) <= maxLocationFieldBytes {
		return cleaned
	}
	// Truncate by runes to stay under the byte budget without splitting UTF-8.
	runes := []rune(cleaned)
	kept := 0
	for i, r := range runes {
		if kept+utf8.RuneLen(r) > maxLocationFieldBytes {
			return string(runes[:i])
		}
		kept += utf8.RuneLen(r)
	}
	return string(runes)
}

func isAbnormalControlRune(r rune) bool {
	switch r {
	case 0x061c, // Arabic Letter Mark
		0x200e, 0x200f, // LRE/RLE? (LRI? no: 200E LRM, 200F RLM)
		0x2028, 0x2029, // Line/Paragraph Separator
		0x202a, 0x202b, 0x202c, 0x202d, 0x202e, // LRE/RLE/PDF/LRO/RLO
		0x2066, 0x2067, 0x2068, 0x2069: // LRI/RLI/FSI/PDI
		return true
	}
	return false
}

// pickName resolves a localized name using the configured locale, then the
// locale's base language, then English, then any available key as a stable
// fallback. Matching is case-insensitive so an operator-set locale such as
// "zh-cn" still finds the canonical "zh-CN" database key. Empty names stay
// empty.
func (r *Reader) pickName(names map[string]string) string {
	if len(names) == 0 {
		return ""
	}
	preferred := []string{strings.ToLower(r.locale)}
	if base, _, ok := strings.Cut(r.locale, "-"); ok && base != "" {
		preferred = append(preferred, strings.ToLower(base))
	}
	preferred = append(preferred, "en")

	for _, preferredKey := range preferred {
		for key, value := range names {
			if strings.EqualFold(key, preferredKey) {
				if trimmed := strings.TrimSpace(value); trimmed != "" {
					return trimmed
				}
			}
		}
	}
	keys := make([]string, 0, len(names))
	for key := range names {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if value := strings.TrimSpace(names[key]); value != "" {
			return value
		}
	}
	return ""
}

// LogLookupFailure is a rate-limited sink used by callers to avoid spamming
// logs when lookups keep failing. The counter is atomic because probe requests
// are served concurrently.
type LogLookupFailure struct {
	limit   int
	counter atomic.Int64
}

// NewLogLookupFailure creates a limiter that reports at most every limit calls.
func NewLogLookupFailure(limit int) *LogLookupFailure {
	if limit < 1 {
		limit = 1
	}
	return &LogLookupFailure{limit: limit}
}

// Warn logs a warning at most once per limit invocations.
func (l *LogLookupFailure) Warn(msg string, args ...any) {
	if l == nil {
		return
	}
	n := l.counter.Add(1)
	if n == 1 || n%int64(l.limit) == 0 {
		slog.Warn(msg, args...)
	}
}
