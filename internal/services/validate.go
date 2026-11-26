package services

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/automax/registry/internal/models"
)

// Letters from any language, numbers, spaces, and the marks used in real names.
var nameSpecial = regexp.MustCompile(`[^\p{L}\p{N} \-'./&]`)
var nameLetterOrNumber = regexp.MustCompile(`[\p{L}\p{N}]`)

func validateName(fields map[string]string, field, value string, max int) string {
	value = strings.TrimSpace(value)
	switch {
	case value == "":
		fields[field] = field + " is required"
	case utf8.RuneCountInString(value) > max:
		fields[field] = field + " must be at most " + itoa(max) + " characters"
	}
	return value
}

// rejectSpecialName records an error when a name uses a character outside letters,
// numbers, spaces, and - ' . / &. An empty value is left for the required check.
func rejectSpecialName(fields map[string]string, field, value string) {
	if fields[field] != "" || value == "" {
		return
	}
	if nameSpecial.MatchString(value) || !nameLetterOrNumber.MatchString(value) {
		fields[field] = field + " must not contain special characters"
	}
}

func validateMetadata(fields map[string]string, raw models.JSON) models.JSON {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return models.JSON(`{}`)
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 64*1024 {
		fields["metadata"] = "metadata must be at most 65536 bytes"
		return raw
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &obj); err != nil || obj == nil {
		fields["metadata"] = "metadata must be a JSON object"
		return raw
	}
	return models.JSON(trimmed)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
