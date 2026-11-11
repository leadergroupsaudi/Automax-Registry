package services

import "github.com/automax/registry/internal/repository"

const (
	KindInvalid     = "invalid"
	KindNotFound    = "not_found"
	KindConflict    = "conflict"
	KindUnavailable = "unavailable"
	KindUpstream    = "upstream"
)

type Error struct {
	Kind    string
	Message string
	Fields  map[string]string
}

func (e *Error) Error() string { return e.Message }

func Invalid(message string, fields map[string]string) *Error {
	return &Error{Kind: KindInvalid, Message: message, Fields: fields}
}

func NotFound(message string) *Error {
	return &Error{Kind: KindNotFound, Message: message}
}

func Conflict(message string) *Error {
	return &Error{Kind: KindConflict, Message: message}
}

func Unavailable(message string) *Error {
	return &Error{Kind: KindUnavailable, Message: message}
}

func Upstream(message string) *Error {
	return &Error{Kind: KindUpstream, Message: message}
}

func asDuplicateName(err error, message string) error {
	if repository.IsUniqueViolation(err) {
		return Conflict(message)
	}
	return err
}
