package services

import "strings"

func choice(fields map[string]string, field, value, fallback string, allowed ...string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	for _, ok := range allowed {
		if value == ok {
			return value
		}
	}
	fields[field] = field + " must be " + englishList(allowed)
	return value
}

func englishList(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " or " + items[1]
	default:
		return strings.Join(items[:len(items)-1], ", ") + ", or " + items[len(items)-1]
	}
}

func timeoutSeconds(fields map[string]string, value *int) int {
	if value == nil {
		return 120
	}
	if *value < 1 {
		fields["acceptance_timeout_seconds"] = "acceptance_timeout_seconds must be at least 1"
		return 0
	}
	return *value
}

func activeOrDefault(value *bool) bool {
	if value == nil {
		return true
	}
	return *value
}
