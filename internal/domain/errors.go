package domain

import "fmt"

type RuleError struct {
	Code    string
	Message string
}

func (e *RuleError) Error() string { return e.Message }

func invalid(code, format string, args ...any) error {
	return &RuleError{Code: code, Message: fmt.Sprintf(format, args...)}
}
