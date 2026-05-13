//go:build unit

package service

import (
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

func TestSupportIssueSearchParseIDNumber(t *testing.T) {
	t.Parallel()

	parsed := mustParseSupportIssueSearch(t, "id:123")
	if parsed.IssueID == nil || *parsed.IssueID != 123 {
		t.Fatalf("IssueID = %v, want 123", parsed.IssueID)
	}
	if parsed.PublicID != "" {
		t.Fatalf("PublicID = %q, want empty", parsed.PublicID)
	}
}

func TestSupportIssueSearchParsePublicID(t *testing.T) {
	t.Parallel()

	parsed := mustParseSupportIssueSearch(t, "id:ISS-000123")
	if parsed.PublicID != "ISS-000123" {
		t.Fatalf("PublicID = %q, want ISS-000123", parsed.PublicID)
	}
	if parsed.IssueID != nil {
		t.Fatalf("IssueID = %v, want nil", *parsed.IssueID)
	}
}

func TestSupportIssueSearchParseFullEmail(t *testing.T) {
	t.Parallel()

	parsed := mustParseSupportIssueSearch(t, "email:user@example.com")
	if parsed.Email != "user@example.com" {
		t.Fatalf("Email = %q, want user@example.com", parsed.Email)
	}
	if parsed.EmailIsDomain {
		t.Fatal("EmailIsDomain = true, want false")
	}
}

func TestSupportIssueSearchParseEmailDomain(t *testing.T) {
	t.Parallel()

	parsed := mustParseSupportIssueSearch(t, "email:gmail.com")
	if parsed.Email != "gmail.com" {
		t.Fatalf("Email = %q, want gmail.com", parsed.Email)
	}
	if !parsed.EmailIsDomain {
		t.Fatal("EmailIsDomain = false, want true")
	}
}

func TestSupportIssueSearchParseStructuredFilters(t *testing.T) {
	t.Parallel()

	parsed := mustParseSupportIssueSearch(t, "status:open category:payment code:429")
	if parsed.Status != domain.SupportIssueStatusOpen {
		t.Fatalf("Status = %q, want open", parsed.Status)
	}
	if parsed.Category != domain.SupportIssueCategoryPayment {
		t.Fatalf("Category = %q, want payment", parsed.Category)
	}
	if parsed.HTTPStatus == nil || *parsed.HTTPStatus != 429 {
		t.Fatalf("HTTPStatus = %v, want 429", parsed.HTTPStatus)
	}
}

func TestSupportIssueSearchParseTimeRange(t *testing.T) {
	t.Parallel()

	parsed := mustParseSupportIssueSearch(t, "time:2026-05-01..2026-05-13")
	wantFrom := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	wantTo := time.Date(2026, 5, 13, 23, 59, 59, int(time.Second-time.Nanosecond), time.Local)

	if parsed.OccurredFrom == nil || !parsed.OccurredFrom.Equal(wantFrom) {
		t.Fatalf("OccurredFrom = %v, want %v", parsed.OccurredFrom, wantFrom)
	}
	if parsed.OccurredTo == nil || !parsed.OccurredTo.Equal(wantTo) {
		t.Fatalf("OccurredTo = %v, want %v", parsed.OccurredTo, wantTo)
	}
}

func TestSupportIssueSearchParsePhraseAndFilters(t *testing.T) {
	t.Parallel()

	parsed := mustParseSupportIssueSearch(t, `"rate limit" code:429 status:open`)
	if len(parsed.Phrases) != 1 || parsed.Phrases[0] != "rate limit" {
		t.Fatalf("Phrases = %#v, want rate limit", parsed.Phrases)
	}
	if parsed.HTTPStatus == nil || *parsed.HTTPStatus != 429 {
		t.Fatalf("HTTPStatus = %v, want 429", parsed.HTTPStatus)
	}
	if parsed.Status != domain.SupportIssueStatusOpen {
		t.Fatalf("Status = %q, want open", parsed.Status)
	}
}

func TestSupportIssueSearchParseTitlePhrase(t *testing.T) {
	t.Parallel()

	parsed := mustParseSupportIssueSearch(t, `title:"余额未到账"`)
	if parsed.TitlePhrase != "余额未到账" {
		t.Fatalf("TitlePhrase = %q, want 余额未到账", parsed.TitlePhrase)
	}
}

func TestSupportIssueSearchParseErrorPhrase(t *testing.T) {
	t.Parallel()

	parsed := mustParseSupportIssueSearch(t, `error:"Your account is temporarily unavailable"`)
	if parsed.ErrorPhrase != "Your account is temporarily unavailable" {
		t.Fatalf("ErrorPhrase = %q, want exact error phrase", parsed.ErrorPhrase)
	}
}

func TestSupportIssueSearchParseHasImageAndLanguage(t *testing.T) {
	t.Parallel()

	parsed := mustParseSupportIssueSearch(t, "has:image lang:en")
	if parsed.HasImage == nil || !*parsed.HasImage {
		t.Fatalf("HasImage = %v, want true", parsed.HasImage)
	}
	if parsed.ScreenshotLanguage != domain.SupportIssueScreenshotLanguageEN || parsed.Language != domain.SupportIssueScreenshotLanguageEN {
		t.Fatalf("language = %q/%q, want en", parsed.Language, parsed.ScreenshotLanguage)
	}
}

func TestSupportIssueSearchParseTermsWithANDSemantics(t *testing.T) {
	t.Parallel()

	parsed := mustParseSupportIssueSearch(t, "rate limit balance")
	want := []string{"rate", "limit", "balance"}
	if len(parsed.Terms) != len(want) {
		t.Fatalf("Terms = %#v, want %#v", parsed.Terms, want)
	}
	for i := range want {
		if parsed.Terms[i] != want[i] {
			t.Fatalf("Terms[%d] = %q, want %q", i, parsed.Terms[i], want[i])
		}
	}
	if len(parsed.Phrases) != 0 {
		t.Fatalf("Phrases = %#v, want empty", parsed.Phrases)
	}
}

func TestSupportIssueSearchUnclosedQuoteReturnsError(t *testing.T) {
	t.Parallel()

	_, err := ParseSupportIssueSearch(`title:"余额未到账`)
	if err == nil {
		t.Fatal("expected unclosed quote error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unclosed quote") {
		t.Fatalf("error = %v, want unclosed quote", err)
	}
}

func TestSupportIssueSearchUnknownFieldReturnsError(t *testing.T) {
	t.Parallel()

	_, err := ParseSupportIssueSearch("unknown:value")
	if err == nil {
		t.Fatal("expected unknown field error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unknown search field") {
		t.Fatalf("error = %v, want unknown search field", err)
	}
}

func TestSupportIssueSearchParseAPIKeySuffix(t *testing.T) {
	t.Parallel()

	parsed := mustParseSupportIssueSearch(t, "key:ab12cd")
	if parsed.APIKeySuffix != "ab12cd" {
		t.Fatalf("APIKeySuffix = %q, want ab12cd", parsed.APIKeySuffix)
	}
}

func TestSupportIssueEmailMasking(t *testing.T) {
	t.Parallel()

	masked, err := domain.MaskSupportIssueEmail("User@Example.COM")
	if err != nil {
		t.Fatalf("MaskSupportIssueEmail returned error: %v", err)
	}
	if masked != "u***@example.com" {
		t.Fatalf("masked email = %q, want u***@example.com", masked)
	}
}

func TestSupportIssueStatusTransitions(t *testing.T) {
	t.Parallel()

	allowed := []struct {
		from  string
		to    string
		actor string
	}{
		{domain.SupportIssueStatusOpen, domain.SupportIssueStatusNeedsInfo, domain.SupportIssueTransitionActorAdmin},
		{domain.SupportIssueStatusOpen, domain.SupportIssueStatusInProgress, domain.SupportIssueTransitionActorAdmin},
		{domain.SupportIssueStatusOpen, domain.SupportIssueStatusResolved, domain.SupportIssueTransitionActorReporter},
		{domain.SupportIssueStatusOpen, domain.SupportIssueStatusClosed, domain.SupportIssueTransitionActorAdmin},
		{domain.SupportIssueStatusNeedsInfo, domain.SupportIssueStatusOpen, domain.SupportIssueTransitionActorReporter},
		{domain.SupportIssueStatusNeedsInfo, domain.SupportIssueStatusInProgress, domain.SupportIssueTransitionActorAdmin},
		{domain.SupportIssueStatusNeedsInfo, domain.SupportIssueStatusResolved, domain.SupportIssueTransitionActorAdmin},
		{domain.SupportIssueStatusNeedsInfo, domain.SupportIssueStatusClosed, domain.SupportIssueTransitionActorAdmin},
		{domain.SupportIssueStatusInProgress, domain.SupportIssueStatusNeedsInfo, domain.SupportIssueTransitionActorAdmin},
		{domain.SupportIssueStatusInProgress, domain.SupportIssueStatusResolved, domain.SupportIssueTransitionActorReporter},
		{domain.SupportIssueStatusInProgress, domain.SupportIssueStatusClosed, domain.SupportIssueTransitionActorAdmin},
		{domain.SupportIssueStatusResolved, domain.SupportIssueStatusOpen, domain.SupportIssueTransitionActorAdmin},
		{domain.SupportIssueStatusClosed, domain.SupportIssueStatusOpen, domain.SupportIssueTransitionActorAdmin},
	}

	for _, tt := range allowed {
		if err := domain.ValidateSupportIssueStatusTransition(tt.from, tt.to, tt.actor); err != nil {
			t.Fatalf("transition %s -> %s by %s returned error: %v", tt.from, tt.to, tt.actor, err)
		}
	}

	blocked := []struct {
		from  string
		to    string
		actor string
	}{
		{domain.SupportIssueStatusOpen, domain.SupportIssueStatusNeedsInfo, domain.SupportIssueTransitionActorReporter},
		{domain.SupportIssueStatusResolved, domain.SupportIssueStatusInProgress, domain.SupportIssueTransitionActorAdmin},
		{domain.SupportIssueStatusClosed, domain.SupportIssueStatusOpen, domain.SupportIssueTransitionActorReporter},
		{domain.SupportIssueStatusOpen, domain.SupportIssueStatusOpen, domain.SupportIssueTransitionActorAdmin},
	}

	for _, tt := range blocked {
		if err := domain.ValidateSupportIssueStatusTransition(tt.from, tt.to, tt.actor); err == nil {
			t.Fatalf("transition %s -> %s by %s succeeded, want error", tt.from, tt.to, tt.actor)
		}
	}
}

func mustParseSupportIssueSearch(t *testing.T, raw string) ParsedSupportIssueSearch {
	t.Helper()

	parsed, err := ParseSupportIssueSearch(raw)
	if err != nil {
		t.Fatalf("ParseSupportIssueSearch(%q) returned error: %v", raw, err)
	}
	return parsed
}
