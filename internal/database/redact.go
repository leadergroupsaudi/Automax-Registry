package database

import (
	"fmt"
	"log"
	"strings"

	"github.com/automax/registry/pkg/utils"
)

// Redact strips credential hashes from text that might be written to a log.
func Redact(s string) string {
	return utils.Redact(s)
}

type redactingLogWriter struct{}

func (redactingLogWriter) Printf(format string, args ...any) {
	msg := Redact(fmt.Sprintf(format, args...))
	if strings.Contains(msg, "failed to initialize database") {
		return
	}
	log.Print(msg)
}
