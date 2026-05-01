package domain

import (
	"errors"
	"testing"
)

const errMustNotBeEmpty = "Error() must not return empty string"

func TestValidationError_Error(t *testing.T) {
	e := &ValidationError{Code: ErrUnknownAppTag, Message: "not found"}
	want := "UNKNOWN_APP_TAG: not found"
	if e.Error() != want {
		t.Errorf("want %q, got %q", want, e.Error())
	}
}

func TestQuotaError_Error(t *testing.T) {
	e := &QuotaError{Limit: 100, Current: 95, Requested: 10}
	got := e.Error()
	if got == "" {
		t.Fatal(errMustNotBeEmpty)
	}
}

func TestQuotaStateError_Error(t *testing.T) {
	cause := errors.New("nats down")
	e := &QuotaStateError{Cause: cause}
	if e.Error() == "" {
		t.Fatal(errMustNotBeEmpty)
	}
	if !errors.Is(e, cause) {
		t.Error("Unwrap must expose cause for errors.Is")
	}
}

func TestNatsPublishError_Error(t *testing.T) {
	cause := errors.New("connection refused")
	e := &NatsPublishError{Cause: cause}
	if e.Error() == "" {
		t.Fatal(errMustNotBeEmpty)
	}
	if !errors.Is(e, cause) {
		t.Error("Unwrap must expose cause for errors.Is")
	}
}
