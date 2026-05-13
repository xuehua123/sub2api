package service

import (
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type ParsedSupportIssueSearch struct {
	IssueID            *int64
	PublicID           string
	Email              string
	EmailIsDomain      bool
	Status             string
	Category           string
	Severity           string
	Model              string
	ModelName          string
	Client             string
	ClientName         string
	HTTPStatus         *int
	ErrorCode          string
	APIKeySuffix       string
	Language           string
	ScreenshotLanguage string
	HasImage           *bool
	OccurredFrom       *time.Time
	OccurredTo         *time.Time
	TitlePhrase        string
	ErrorPhrase        string
	Phrases            []string
	Terms              []string
}

func ParseSupportIssueSearch(rawQuery string) (ParsedSupportIssueSearch, error) {
	tokens, err := tokenizeSupportIssueSearch(rawQuery)
	if err != nil {
		return ParsedSupportIssueSearch{}, err
	}

	var parsed ParsedSupportIssueSearch
	for _, token := range tokens {
		if err := parsed.applySupportIssueSearchToken(token); err != nil {
			return ParsedSupportIssueSearch{}, err
		}
	}
	return parsed, nil
}

func ParseSupportIssueSearchQuery(rawQuery string) (ParsedSupportIssueSearch, error) {
	return ParseSupportIssueSearch(rawQuery)
}

func (p *ParsedSupportIssueSearch) applySupportIssueSearchToken(rawToken string) error {
	field, rawValue, structured := splitSupportIssueSearchField(rawToken)
	if !structured {
		value, quoted, err := unquoteSupportIssueSearchValue(rawToken)
		if err != nil {
			return err
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return nil
		}
		if quoted {
			p.Phrases = append(p.Phrases, value)
			return nil
		}
		p.Terms = append(p.Terms, value)
		return nil
	}

	value, quoted, err := unquoteSupportIssueSearchValue(rawValue)
	if err != nil {
		return err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return domain.NewSupportIssueSearchInvalidError("empty value for search field: " + field)
	}

	switch field {
	case "id":
		return p.applySupportIssueIDFilter(value)
	case "email":
		return p.applySupportIssueEmailFilter(value)
	case "status":
		status, err := domain.ValidateSupportIssueStatus(value)
		if err != nil {
			return err
		}
		p.Status = status
	case "category", "type":
		category, err := domain.ValidateSupportIssueCategory(value)
		if err != nil {
			return err
		}
		p.Category = category
	case "severity":
		severity, err := domain.ValidateSupportIssueSeverity(value)
		if err != nil {
			return err
		}
		p.Severity = severity
	case "model":
		p.Model = strings.ToLower(value)
		p.ModelName = p.Model
	case "client":
		p.Client = strings.ToLower(value)
		p.ClientName = p.Client
	case "code":
		code, err := parseSupportIssueHTTPStatusFilter(value)
		if err != nil {
			return err
		}
		p.HTTPStatus = &code
	case "error":
		if quoted {
			p.ErrorPhrase = value
			return nil
		}
		p.ErrorCode = strings.ToLower(value)
	case "key":
		suffix, err := domain.NormalizeSupportIssueAPIKeySuffix(value)
		if err != nil {
			return err
		}
		p.APIKeySuffix = suffix
	case "lang":
		language, err := domain.ValidateSupportIssueScreenshotLanguage(value)
		if err != nil {
			return err
		}
		p.Language = language
		p.ScreenshotLanguage = language
	case "has":
		if strings.ToLower(value) != "image" {
			return domain.NewSupportIssueSearchInvalidError("unsupported has filter: " + value)
		}
		hasImage := true
		p.HasImage = &hasImage
	case "time":
		from, to, err := parseSupportIssueTimeFilter(value)
		if err != nil {
			return err
		}
		p.OccurredFrom = &from
		p.OccurredTo = &to
	case "title":
		if !quoted {
			return domain.NewSupportIssueSearchInvalidError("title search requires a quoted phrase")
		}
		p.TitlePhrase = value
	default:
		return domain.NewSupportIssueSearchInvalidError("unknown search field: " + field)
	}

	return nil
}

func tokenizeSupportIssueSearch(rawQuery string) ([]string, error) {
	rawQuery = strings.TrimSpace(rawQuery)
	if rawQuery == "" {
		return nil, nil
	}

	var tokens []string
	start := -1
	inQuote := false
	escaped := false

	for i, r := range rawQuery {
		if start == -1 {
			if unicode.IsSpace(r) {
				continue
			}
			start = i
		}

		if escaped {
			escaped = false
			continue
		}
		if inQuote && r == '\\' {
			escaped = true
			continue
		}
		if r == '"' {
			inQuote = !inQuote
			continue
		}
		if !inQuote && unicode.IsSpace(r) {
			tokens = append(tokens, strings.TrimSpace(rawQuery[start:i]))
			start = -1
		}
	}

	if inQuote {
		return nil, domain.NewSupportIssueSearchInvalidError("unclosed quote in search query")
	}
	if start != -1 {
		tokens = append(tokens, strings.TrimSpace(rawQuery[start:]))
	}

	return tokens, nil
}

func splitSupportIssueSearchField(rawToken string) (string, string, bool) {
	colonIndex := strings.Index(rawToken, ":")
	if colonIndex <= 0 {
		return "", "", false
	}
	quoteIndex := strings.Index(rawToken, "\"")
	if quoteIndex >= 0 && quoteIndex < colonIndex {
		return "", "", false
	}

	field := strings.ToLower(strings.TrimSpace(rawToken[:colonIndex]))
	if field == "" {
		return "", "", false
	}
	return field, rawToken[colonIndex+1:], true
}

func unquoteSupportIssueSearchValue(rawValue string) (string, bool, error) {
	rawValue = strings.TrimSpace(rawValue)
	if rawValue == "" {
		return "", false, nil
	}

	if strings.HasPrefix(rawValue, "\"") {
		if !strings.HasSuffix(rawValue, "\"") || len(rawValue) == 1 {
			return "", false, domain.NewSupportIssueSearchInvalidError("unclosed quote in search query")
		}
		value := rawValue[1 : len(rawValue)-1]
		value = strings.ReplaceAll(value, `\"`, `"`)
		value = strings.ReplaceAll(value, `\\`, `\`)
		return value, true, nil
	}

	if strings.Contains(rawValue, "\"") {
		return "", false, domain.NewSupportIssueSearchInvalidError("unexpected quote in search query")
	}
	return rawValue, false, nil
}

func (p *ParsedSupportIssueSearch) applySupportIssueIDFilter(value string) error {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	if strings.HasPrefix(normalized, "ISS-") {
		digits := strings.TrimPrefix(normalized, "ISS-")
		if digits == "" || !supportIssueSearchAllDigits(digits) {
			return domain.NewSupportIssueSearchInvalidError("invalid issue public id: " + value)
		}
		p.PublicID = normalized
		return nil
	}

	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return domain.NewSupportIssueSearchInvalidError("invalid issue id: " + value)
	}
	p.IssueID = &id
	return nil
}

func (p *ParsedSupportIssueSearch) applySupportIssueEmailFilter(value string) error {
	value = strings.ToLower(strings.TrimSpace(value))
	if strings.Contains(value, "@") {
		email, err := domain.NormalizeSupportIssueEmail(value)
		if err != nil {
			return err
		}
		p.Email = email
		p.EmailIsDomain = false
		return nil
	}
	if value == "" || strings.ContainsAny(value, " \t\r\n") {
		return domain.NewSupportIssueSearchInvalidError("invalid email search value: " + value)
	}
	p.Email = value
	p.EmailIsDomain = true
	return nil
}

func parseSupportIssueHTTPStatusFilter(value string) (int, error) {
	status, err := strconv.Atoi(value)
	if err != nil {
		return 0, domain.NewSupportIssueSearchInvalidError("invalid http status: " + value)
	}
	if err := domain.ValidateSupportIssueHTTPStatus(&status); err != nil {
		return 0, err
	}
	return status, nil
}

func parseSupportIssueTimeFilter(value string) (time.Time, time.Time, error) {
	if strings.Contains(value, "..") {
		parts := strings.Split(value, "..")
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return time.Time{}, time.Time{}, domain.NewSupportIssueSearchInvalidError("invalid time range: " + value)
		}
		from, err := parseSupportIssueSearchDateStart(parts[0])
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		to, err := parseSupportIssueSearchDateEnd(parts[1])
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		if to.Before(from) {
			return time.Time{}, time.Time{}, domain.NewSupportIssueSearchInvalidError("time range end is before start")
		}
		return from, to, nil
	}

	from, err := parseSupportIssueSearchDateStart(value)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	to, err := parseSupportIssueSearchDateEnd(value)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return from, to, nil
}

func parseSupportIssueSearchDateStart(value string) (time.Time, error) {
	date, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(value), time.Local)
	if err != nil {
		return time.Time{}, domain.NewSupportIssueSearchInvalidError("invalid time date: " + value)
	}
	return date, nil
}

func parseSupportIssueSearchDateEnd(value string) (time.Time, error) {
	date, err := parseSupportIssueSearchDateStart(value)
	if err != nil {
		return time.Time{}, err
	}
	return date.AddDate(0, 0, 1).Add(-time.Nanosecond), nil
}

func supportIssueSearchAllDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
