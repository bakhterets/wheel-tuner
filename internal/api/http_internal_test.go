package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatusRecorderUnwrap(t *testing.T) {
	inner := httptest.NewRecorder()
	rec := &statusRecorder{ResponseWriter: inner, status: http.StatusOK}
	if err := http.NewResponseController(rec).Flush(); err != nil {
		t.Fatalf("flush through the recorder: %v", err)
	}
	if !inner.Flushed {
		t.Error("flush did not reach the underlying writer")
	}
}
