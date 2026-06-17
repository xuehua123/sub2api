package service

import "strings"

const (
	AccountExtraTagsKey = "tags"

	accountTagMaxLength = 32
	accountTagsMaxCount = 20
)

// NormalizeAccountTags cleans account tags before they are stored or returned.
func NormalizeAccountTags(input []string) []string {
	if len(input) == 0 {
		return []string{}
	}

	tags := make([]string, 0, min(len(input), accountTagsMaxCount))
	seen := make(map[string]struct{}, len(input))
	for _, raw := range input {
		tag := truncateAccountTag(strings.TrimSpace(raw))
		if tag == "" {
			continue
		}

		key := strings.ToLower(tag)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		tags = append(tags, tag)
		if len(tags) >= accountTagsMaxCount {
			break
		}
	}
	return tags
}

// AccountTagsFromExtra extracts normalized tags from account.extra.
func AccountTagsFromExtra(extra map[string]any) []string {
	if len(extra) == 0 {
		return []string{}
	}

	raw, ok := extra[AccountExtraTagsKey]
	if !ok || raw == nil {
		return []string{}
	}

	switch value := raw.(type) {
	case []string:
		return NormalizeAccountTags(value)
	case []any:
		tags := make([]string, 0, len(value))
		for _, item := range value {
			tag, ok := item.(string)
			if ok {
				tags = append(tags, tag)
			}
		}
		return NormalizeAccountTags(tags)
	case string:
		return NormalizeAccountTags(splitAccountTagString(value))
	default:
		return []string{}
	}
}

func splitAccountTagString(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '；' || r == '\n' || r == '\t'
	})
}

func truncateAccountTag(tag string) string {
	runes := []rune(tag)
	if len(runes) <= accountTagMaxLength {
		return tag
	}
	return string(runes[:accountTagMaxLength])
}
