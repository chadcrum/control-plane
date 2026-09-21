package service

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/dcm-project/control-plane/internal/placement/policy"
	"github.com/dcm-project/control-plane/internal/placement/sprm"
)

// Error codes returned by service operations.
const (
	ErrCodeNotFound            = "https://dcm.example.com/errors/not-found"
	ErrCodeConflict            = "https://dcm.example.com/errors/conflict"
	ErrCodeValidation          = "https://dcm.example.com/errors/validation"
	ErrCodeProvisioningError   = "https://dcm.example.com/errors/provisioning-error"
	ErrCodeInternal            = "https://dcm.example.com/errors/internal-error"
	ErrCodePolicyError         = "https://dcm.example.com/errors/policy-error"
	ErrCodePolicyInternalError = "https://dcm.example.com/errors/policy-internal-error"
	ErrCodePolicyRejected      = "https://dcm.example.com/errors/policy-rejected"
	ErrCodePolicyConflict      = "https://dcm.example.com/errors/policy-conflict"
	ErrCodeSPRMError           = "https://dcm.example.com/errors/sprm-error"
	ErrCodeUnavailable         = "https://dcm.example.com/errors/unavailable"
)

// ServiceError represents a business logic error with a code for HTTP mapping.
type ServiceError struct {
	Code    string
	Message string
}

func (e *ServiceError) Error() string {
	return e.Message
}

// Helper functions for creating ServiceErrors

func NewNotFoundError(message string) *ServiceError {
	return &ServiceError{
		Code:    ErrCodeNotFound,
		Message: message,
	}
}

func NewValidationError(message string) *ServiceError {
	return &ServiceError{
		Code:    ErrCodeValidation,
		Message: message,
	}
}

func NewInternalError(message string) *ServiceError {
	return &ServiceError{
		Code:    ErrCodeInternal,
		Message: message,
	}
}

func NewConflictError(message string) *ServiceError {
	return &ServiceError{
		Code:    ErrCodeConflict,
		Message: message,
	}
}

func NewPolicyError(message string) *ServiceError {
	return &ServiceError{
		Code:    ErrCodePolicyError,
		Message: message,
	}
}

func NewPolicyInternalError(message string) *ServiceError {
	return &ServiceError{
		Code:    ErrCodePolicyInternalError,
		Message: message,
	}
}

func NewPolicyRejectedError(message string) *ServiceError {
	return &ServiceError{
		Code:    ErrCodePolicyRejected,
		Message: message,
	}
}

func NewPolicyConflictError(message string) *ServiceError {
	return &ServiceError{
		Code:    ErrCodePolicyConflict,
		Message: message,
	}
}

func NewSPRMError(message string) *ServiceError {
	return &ServiceError{
		Code:    ErrCodeSPRMError,
		Message: message,
	}
}

func NewUnavailableError(message string) *ServiceError {
	return &ServiceError{
		Code:    ErrCodeUnavailable,
		Message: message,
	}
}

// IsCallbackRetryable reports whether a placement callback error should cause
// the status consumer to NAK the message for redelivery.
func IsCallbackRetryable(err error) bool {
	var svcErr *ServiceError
	if !errors.As(err, &svcErr) {
		return false
	}
	switch svcErr.Code {
	case ErrCodeInternal, ErrCodeUnavailable, ErrCodeSPRMError, ErrCodePolicyInternalError:
		return true
	default:
		return false
	}
}

// IsClientError returns true if err is a ServiceError representing a client-side
// (4xx) problem. If svcErr is non-nil it is populated with the unwrapped error.
func IsClientError(err error, svcErr **ServiceError) bool {
	if !errors.As(err, svcErr) {
		return false
	}
	switch (*svcErr).Code {
	case ErrCodeValidation, ErrCodeNotFound, ErrCodeConflict,
		ErrCodePolicyRejected, ErrCodePolicyConflict, ErrCodeProvisioningError:
		return true
	}
	return false
}

// handlePolicyError maps policy client errors to service errors by checking
// the error type and extracting the HTTP status code.
func handlePolicyError(err error) *ServiceError {
	// Try to unwrap and get the actual error
	var httpErr *policy.HTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.StatusCode {
		case http.StatusBadRequest:
			return NewValidationError(httpErr.Body)
		case http.StatusNotAcceptable:
			return NewPolicyRejectedError(httpErr.Body)
		case http.StatusConflict:
			return NewPolicyConflictError(httpErr.Body)
		case http.StatusInternalServerError:
			return NewPolicyInternalError(httpErr.Body)
		default:
			return NewPolicyError(fmt.Sprintf("policy evaluation failed with status %d: %s", httpErr.StatusCode, httpErr.Body))
		}
	}

	// Network or client communication error - treat as transient for callback retry.
	return NewPolicyInternalError("policy client communication error: " + err.Error())
}

// handleSPRMError maps SPRM client errors to service errors by checking
// the error type and extracting the HTTP status code. 5xx bodies are logged
// server-side and replaced with a generic client-facing message so internal
// error detail (DB errors, NATS errors) never leaks into API responses.
func handleSPRMError(err error) *ServiceError {
	var httpErr *sprm.HTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.StatusCode {
		case http.StatusBadRequest:
			return NewValidationError(fmt.Sprintf("invalid request format for SPRM: %s", httpErr.Body))
		case http.StatusNotFound:
			return NewNotFoundError(fmt.Sprintf("resource not found in SPRM: %s", httpErr.Body))
		case http.StatusConflict:
			return NewConflictError(fmt.Sprintf("resource conflict in SPRM: %s", httpErr.Body))
		case http.StatusUnprocessableEntity:
			return &ServiceError{
				Code:    ErrCodeProvisioningError,
				Message: fmt.Sprintf("SPRM provisioning error: %s", httpErr.Body),
			}
		case http.StatusServiceUnavailable:
			slog.Error("sprm request failed", "status", httpErr.StatusCode, "detail", httpErr.Body)
			return NewUnavailableError("service temporarily unavailable")
		case http.StatusInternalServerError:
			slog.Error("sprm request failed", "status", httpErr.StatusCode, "detail", httpErr.Body)
			return NewSPRMError("internal server error")
		default:
			slog.Error("sprm request failed", "status", httpErr.StatusCode, "detail", httpErr.Body)
			return NewSPRMError("internal server error")
		}
	}

	slog.Error("sprm request failed", "error", err)
	return NewSPRMError("internal server error")
}
