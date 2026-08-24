package application

import (
	"errors"
	"fmt"
)

type ErrorKind string

const (
	KindValidation   ErrorKind = "validation"
	KindUnauthorized ErrorKind = "unauthorized"
	KindNotFound     ErrorKind = "not_found"
	KindConflict     ErrorKind = "conflict"
	KindState        ErrorKind = "invalid_state"
	KindPersistence  ErrorKind = "persistence"
)

type AppError struct {
	Kind    ErrorKind
	Code    string
	Message string
	Cause   error
}

func (e *AppError) Error() string { return e.Message }
func (e *AppError) Unwrap() error { return e.Cause }

func fail(kind ErrorKind, code, format string, args ...any) error {
	return &AppError{Kind: kind, Code: code, Message: fmt.Sprintf(format, args...)}
}

func Classify(err error) *AppError {
	var app *AppError
	if errors.As(err, &app) {
		return app
	}
	return &AppError{Kind: KindPersistence, Code: "internal_error", Message: "服务内部错误", Cause: err}
}
