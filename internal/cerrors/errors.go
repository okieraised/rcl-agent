package cerrors

import (
	"errors"
	"fmt"
	"net/http"
)

type AppError struct {
	Code       string
	Message    string
	HTTPStatus int
	Cause      error
}

func (e *AppError) Error() string {
	if e == nil {
		return "OK"
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

func (e *AppError) Unwrap() error { return e.Cause }

// WithCause returns a shallow copy of e with Cause.
func (e *AppError) WithCause(err error) *AppError {
	if e == nil {
		return nil
	}
	c := *e
	c.Cause = err
	return &c
}

// WithMessage returns a shallow copy with an overridden message.
func (e *AppError) WithMessage(msg string, a ...any) *AppError {
	if e == nil {
		return nil
	}
	c := *e
	if len(a) > 0 {
		c.Message = fmt.Sprintf(msg, a...)
	} else {
		c.Message = msg
	}
	return &c
}

// CodeOf returns the code if err is *AppError; "UNKNOWN" otherwise; "OK" for nil.
func CodeOf(err error) string {
	switch e := err.(type) {
	case nil:
		return "OK"
	case *AppError:
		return e.Code
	default:
		return "UNKNOWN"
	}
}

// MessageOf returns the message if err is *AppError; err.Error() otherwise; "OK" for nil.
func MessageOf(err error) string {
	switch e := err.(type) {
	case nil:
		return "OK"
	case *AppError:
		if e.Message != "" {
			return e.Message
		}
		return e.Code
	default:
		return e.Error()
	}
}

// HTTPStatusOf returns the HTTP status if err is *AppError; otherwise 500; 200 for nil.
func HTTPStatusOf(err error) int {
	switch e := err.(type) {
	case nil:
		return http.StatusOK
	case *AppError:
		if e.HTTPStatus != 0 {
			return e.HTTPStatus
		}
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// IsCode reports whether err is an *AppError with the given code.
func IsCode(err error, code string) bool {
	var e *AppError
	if errors.As(err, &e) {
		return e.Code == code
	}
	return false
}

// def is a small constructor for sentinels.
func def(code, msg string, httpStatus int) *AppError {
	return &AppError{Code: code, Message: msg, HTTPStatus: httpStatus}
}

var (
	OK = def("OK", "OK", http.StatusOK)
)

var (
	ErrGenericBadRequest      = def("400000", "bad request error", http.StatusBadRequest)
	ErrGenericUnauthorized    = def("400001", "unauthorized request error", http.StatusUnauthorized)
	ErrGenericPermission      = def("400003", "invalid permission error", http.StatusForbidden)
	ErrGenericUnknownAPIPath  = def("400004", "unknown api path", http.StatusNotFound)
	ErrGenericInternalServer  = def("500000", "internal server error", http.StatusInternalServerError)
	ErrGenericRequestTimedOut = def("500004", "request timeout error", http.StatusGatewayTimeout)
	ErrInvalidDatabaseClient  = def("500005", "invalid database client", http.StatusInternalServerError)
)

var (
	ErrMissingAuthenticationHeader = def("410000", "missing authentication header", http.StatusUnauthorized)
	ErrInvalidAuthenticationHeader = def("410001", "invalid authentication header", http.StatusUnauthorized)
)

var (
	ErrMissingWebRTCOffer = def("420000", "missing webrtc offer", http.StatusBadRequest)
)
