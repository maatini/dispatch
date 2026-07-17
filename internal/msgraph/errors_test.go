package msgraph

import (
	"errors"
	"testing"
	"time"
)

func TestGraphTransientError_Error(t *testing.T) {
	te := &GraphTransientError{StatusCode: 429, RetryAfter: 5 * time.Second}
	s := te.Error()
	if s == "" {
		t.Fatal("Error() must return non-empty string")
	}
}

func TestGraphTransientError_ErrorWithCause(t *testing.T) {
	cause := errors.New("connection refused")
	te := &GraphTransientError{Cause: cause}
	s := te.Error()
	if s == "" {
		t.Fatal("Error() must return non-empty string")
	}
}

func TestGraphTransientError_Unwrap(t *testing.T) {
	cause := errors.New("timeout")
	te := &GraphTransientError{Cause: cause}
	if !errors.Is(te, cause) {
		t.Error("Unwrap must return Cause")
	}
}

func TestGraphTransientError_UnwrapNil(t *testing.T) {
	te := &GraphTransientError{}
	if te.Unwrap() != nil {
		t.Error("Unwrap of nil Cause must return nil")
	}
}

func TestGraphPermanentError_Error(t *testing.T) {
	pe := &GraphPermanentError{StatusCode: 400, Body: "bad request"}
	s := pe.Error()
	if s == "" {
		t.Fatal("Error() must return non-empty string")
	}
}

func TestGraphPermanentError_ErrorWithoutBody(t *testing.T) {
	pe := &GraphPermanentError{StatusCode: 404}
	s := pe.Error()
	if s == "" {
		t.Fatal("Error() must return non-empty string")
	}
}
