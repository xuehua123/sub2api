package geoip

import (
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

// fakeDB implements db with an in-memory record map so Lookup can be tested
// without shipping a licensed MMDB fixture.
type fakeDB struct {
	records map[string]*mmdbRecord
	calls   int
	closed  bool
}

type blockingDB struct {
	lookupStarted chan struct{}
	releaseLookup chan struct{}
	closed        atomic.Bool
}

func (b *blockingDB) Lookup(_ net.IP, result any) error {
	close(b.lookupStarted)
	<-b.releaseLookup
	if b.closed.Load() {
		return os.ErrClosed
	}
	if dst, ok := result.(*mmdbRecord); ok {
		*dst = *zhRecord("CN")
	}
	return nil
}

func (b *blockingDB) Close() error {
	b.closed.Store(true)
	return nil
}

func (f *fakeDB) Lookup(ip net.IP, result any) error {
	if f == nil || f.closed {
		return os.ErrClosed
	}
	f.calls++
	if record, ok := f.records[ip.String()]; ok {
		dst, ok := result.(*mmdbRecord)
		if !ok {
			return os.ErrInvalid
		}
		*dst = *record
	} else {
		// maxminddb returns a zero-valued record for a miss.
		if dst, ok := result.(*mmdbRecord); ok {
			*dst = mmdbRecord{}
		}
	}
	return nil
}

func (f *fakeDB) Close() error {
	if f != nil {
		f.closed = true
	}
	return nil
}

func zhRecord(code string) *mmdbRecord {
	record := &mmdbRecord{}
	record.Country.ISOCode = code
	record.Country.Names = map[string]string{"zh-CN": "中国", "en": "China"}
	record.Subdivisions = []struct {
		Names map[string]string `maxminddb:"names"`
	}{{Names: map[string]string{"zh-CN": "广东", "en": "Guangdong"}}}
	record.City.Names = map[string]string{"zh-CN": "深圳", "en": "Shenzhen"}
	return record
}

func newTestReader(records map[string]*mmdbRecord) (*Reader, *fakeDB) {
	db := &fakeDB{records: records}
	return newReader(db, "zh-CN"), db
}

func TestGeoIPLookupReturnsLocation(t *testing.T) {
	addr := netip.MustParseAddr("113.110.12.34")
	reader, db := newTestReader(map[string]*mmdbRecord{addr.String(): zhRecord("CN")})
	defer func() { _ = reader.Close() }()

	location, err := reader.Lookup(addr)
	require.NoError(t, err)
	require.NotNil(t, location)
	require.Equal(t, "CN", location.CountryCode)
	require.Equal(t, "中国", location.Country)
	require.Equal(t, "广东", location.Region)
	require.Equal(t, "深圳", location.City)
	require.Equal(t, 1, db.calls)
}

func TestGeoIPLookupIPv6(t *testing.T) {
	addr := netip.MustParseAddr("2001:4860:4860::8888")
	record := zhRecord("US")
	reader, db := newTestReader(map[string]*mmdbRecord{addr.String(): record})
	defer func() { _ = reader.Close() }()

	location, err := reader.Lookup(addr)
	require.NoError(t, err)
	require.NotNil(t, location)
	require.Equal(t, "US", location.CountryCode)
	require.Equal(t, 1, db.calls)
}

func TestGeoIPLookupIPv4MappedIPv6(t *testing.T) {
	mapped := netip.MustParseAddr("::ffff:113.110.12.34")
	reader, db := newTestReader(map[string]*mmdbRecord{netip.MustParseAddr("113.110.12.34").String(): zhRecord("CN")})
	defer func() { _ = reader.Close() }()

	location, err := reader.Lookup(mapped)
	require.NoError(t, err)
	require.NotNil(t, location)
	require.Equal(t, "CN", location.CountryCode)
	require.Equal(t, 1, db.calls)
}

func TestGeoIPLookupNameFallback(t *testing.T) {
	addr := netip.MustParseAddr("8.8.8.8")
	record := &mmdbRecord{}
	record.Country.ISOCode = "US"
	record.Country.Names = map[string]string{"en": "United States", "ja": "アメリカ"}
	reader, _ := newTestReader(map[string]*mmdbRecord{addr.String(): record})
	defer func() { _ = reader.Close() }()

	location, err := reader.Lookup(addr)
	require.NoError(t, err)
	require.NotNil(t, location)
	// zh-CN absent → falls back to en (preferred list includes "en").
	require.Equal(t, "United States", location.Country)
}

func TestGeoIPLookupNoMatchReturnsNil(t *testing.T) {
	addr := netip.MustParseAddr("1.1.1.1")
	reader, _ := newTestReader(nil) // fake db returns zero record for misses
	defer func() { _ = reader.Close() }()

	location, err := reader.Lookup(addr)
	require.NoError(t, err)
	require.Nil(t, location)
}

func TestGeoIPLookupRejectsNonPublicWithoutQuery(t *testing.T) {
	for _, raw := range []string{"10.0.0.1", "127.0.0.1", "192.168.1.1", "::1", "fe80::1", "100.64.0.1"} {
		addr := netip.MustParseAddr(raw)
		reader, db := newTestReader(map[string]*mmdbRecord{addr.String(): zhRecord("XX")})
		location, err := reader.Lookup(addr)
		require.Error(t, err, "raw=%s", raw)
		require.Nil(t, location)
		require.Equal(t, 0, db.calls, "database must not be queried for raw=%s", raw)
		_ = reader.Close()
	}
}

func TestGeoIPLookupRejectsInvalidAddress(t *testing.T) {
	reader, db := newTestReader(nil)
	defer func() { _ = reader.Close() }()
	location, err := reader.Lookup(netip.Addr{})
	require.Error(t, err)
	require.Nil(t, location)
	require.Equal(t, 0, db.calls)
}

func TestGeoIPLookupNotReady(t *testing.T) {
	reader, _ := newTestReader(nil)
	require.NoError(t, reader.Close())
	location, err := reader.Lookup(netip.MustParseAddr("8.8.8.8"))
	require.Error(t, err)
	require.Nil(t, location)
	require.False(t, reader.Ready())
}

func TestGeoIPCloseIdempotent(t *testing.T) {
	reader, db := newTestReader(nil)
	require.True(t, reader.Ready())
	require.NoError(t, reader.Close())
	require.False(t, reader.Ready())
	require.NoError(t, reader.Close())
	require.True(t, db.closed)
}

func TestGeoIPCloseWaitsForInFlightLookup(t *testing.T) {
	db := &blockingDB{
		lookupStarted: make(chan struct{}),
		releaseLookup: make(chan struct{}),
	}
	reader := newReader(db, "zh-CN")

	lookupDone := make(chan error, 1)
	go func() {
		_, err := reader.Lookup(netip.MustParseAddr("8.8.8.8"))
		lookupDone <- err
	}()
	<-db.lookupStarted

	closeDone := make(chan error, 1)
	go func() { closeDone <- reader.Close() }()

	select {
	case <-closeDone:
		t.Fatal("Close returned while Lookup was still using the database")
	case <-time.After(50 * time.Millisecond):
	}

	close(db.releaseLookup)
	require.NoError(t, <-lookupDone)
	require.NoError(t, <-closeDone)
	require.False(t, reader.Ready())
}

func TestGeoIPOpenEmptyPath(t *testing.T) {
	reader, err := Open("  ", "zh-CN")
	require.Error(t, err)
	require.Nil(t, reader)
}

func TestGeoIPOpenMissingFile(t *testing.T) {
	reader, err := Open(filepath.Join(t.TempDir(), "does-not-exist.mmdb"), "zh-CN")
	require.Error(t, err)
	require.Nil(t, reader)
}

func TestGeoIPOpenCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.mmdb")
	require.NoError(t, os.WriteFile(path, []byte("this is not a maxmind database"), 0o600))
	reader, err := Open(path, "zh-CN")
	require.Error(t, err)
	require.Nil(t, reader)
}

func TestGeoIPCityDatabaseType(t *testing.T) {
	for _, databaseType := range []string{
		"GeoLite2-City",
		"GeoIP2-City",
		"DBIP-City-Lite",
		" custom-city-database ",
	} {
		require.True(t, isCityDatabaseType(databaseType), "database_type=%q", databaseType)
	}
	for _, databaseType := range []string{"", "GeoLite2-Country", "GeoLite2-ASN", "DBIP-ISP"} {
		require.False(t, isCityDatabaseType(databaseType), "database_type=%q", databaseType)
	}
}

func TestGeoIPRejectsIncompatibleDatabaseAndClosesIt(t *testing.T) {
	database := &fakeDB{}
	reader, err := newCityReader(database, "GeoLite2-ASN", "zh-CN")
	require.Error(t, err)
	require.Nil(t, reader)
	require.True(t, database.closed)
}

func TestGeoIPLookupSanitizesFields(t *testing.T) {
	addr := netip.MustParseAddr("8.8.8.8")
	record := &mmdbRecord{}
	record.Country.ISOCode = "cn  "
	record.Country.Names = map[string]string{"zh-CN": "中" + "\x00" + "国\u2028\u2029" + strings.Repeat("长", 200)}
	reader, _ := newTestReader(map[string]*mmdbRecord{addr.String(): record})
	defer func() { _ = reader.Close() }()

	location, err := reader.Lookup(addr)
	require.NoError(t, err)
	require.NotNil(t, location)
	require.Equal(t, "CN", location.CountryCode)
	require.NotContains(t, location.Country, "\x00")
	require.NotContains(t, location.Country, "\u2028")
	require.NotContains(t, location.Country, "\u2029")
	require.LessOrEqual(t, len(location.Country), maxLocationFieldBytes)
}

func TestGeoIPLocationFieldUsesUTF8ByteBudget(t *testing.T) {
	require.Equal(t, strings.Repeat("a", 96), sanitizeLocationField(strings.Repeat("a", 96)))
	require.Equal(t, strings.Repeat("a", 96), sanitizeLocationField(strings.Repeat("a", 97)))
	require.Equal(t, strings.Repeat("中", 32), sanitizeLocationField(strings.Repeat("中", 32)))
	require.Equal(t, strings.Repeat("中", 32), sanitizeLocationField(strings.Repeat("中", 33)))
	require.True(t, utf8.ValidString(sanitizeLocationField(strings.Repeat("中", 33))))
}

func TestGeoIPLocaleCaseInsensitive(t *testing.T) {
	addr := netip.MustParseAddr("8.8.8.8")
	record := &mmdbRecord{}
	record.Country.ISOCode = "US"
	record.Country.Names = map[string]string{"zh-CN": "美国", "en": "United States"}
	reader := newReader(&fakeDB{records: map[string]*mmdbRecord{addr.String(): record}}, "zh-cn")
	defer func() { _ = reader.Close() }()

	location, err := reader.Lookup(addr)
	require.NoError(t, err)
	require.NotNil(t, location)
	require.Equal(t, "美国", location.Country)
}

func TestGeoIPDefaultLocale(t *testing.T) {
	reader, _ := newTestReader(nil)
	require.Equal(t, "zh-CN", reader.locale)
	reader2 := newReader(&fakeDB{}, "")
	require.Equal(t, "zh-CN", reader2.locale)
	_ = reader2.Close()
}

func TestGeoIPLookupFailureLimiter(t *testing.T) {
	limiter := NewLogLookupFailure(3)
	require.NotNil(t, limiter)
	// No panic on nil receiver.
	var nilLimiter *LogLookupFailure
	nilLimiter.Warn("nope")
	// Counter increments without panicking.
	for i := 0; i < 10; i++ {
		limiter.Warn("warn")
	}
	require.GreaterOrEqual(t, limiter.counter.Load(), int64(10))
}
