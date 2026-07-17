package admin

import (
	"testing"
)

func TestNewHTTPHandler_ParsesSchemaAndReturnsHandler(t *testing.T) {
	handler, err := NewHTTPHandler(&Resolver{
		senders: nil,
		js:      nil,
	})
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}
	if handler == nil {
		t.Fatal("handler must not be nil")
	}
}
