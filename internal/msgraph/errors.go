package msgraph

import "fmt"

// GraphTransientError wraps 429 / 5xx / IO failures — worker must NOT ack, JetStream redelivers.
type GraphTransientError struct {
	StatusCode int
	Cause      error
}

func (e *GraphTransientError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("graph transient error (status=%d): %v", e.StatusCode, e.Cause)
	}
	return fmt.Sprintf("graph transient error (status=%d)", e.StatusCode)
}

func (e *GraphTransientError) Unwrap() error { return e.Cause }

// GraphPermanentError wraps 4xx (≠429) — worker must ack and write FAILED to audit.
type GraphPermanentError struct {
	StatusCode int
	Body       string
}

func (e *GraphPermanentError) Error() string {
	return fmt.Sprintf("graph permanent error (status=%d): %s", e.StatusCode, e.Body)
}
