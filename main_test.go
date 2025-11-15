package main

import (
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestIndex tests the index handler
func TestIndex(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "root path returns hello world",
			path:           "/",
			expectedStatus: http.StatusOK,
			expectedBody:   "Hello, World!\n",
		},
		{
			name:           "non-root path returns 404",
			path:           "/notfound",
			expectedStatus: http.StatusNotFound,
			expectedBody:   "Not Found\n",
		},
		{
			name:           "arbitrary path returns 404",
			path:           "/some/other/path",
			expectedStatus: http.StatusNotFound,
			expectedBody:   "Not Found\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()

			handler := index()
			handler.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if w.Body.String() != tt.expectedBody {
				t.Errorf("expected body %q, got %q", tt.expectedBody, w.Body.String())
			}
		})
	}
}

// TestHealthz tests the health check handler
func TestHealthz(t *testing.T) {
	tests := []struct {
		name           string
		healthy        int32
		expectedStatus int
	}{
		{
			name:           "healthy returns 204",
			healthy:        1,
			expectedStatus: http.StatusNoContent,
		},
		{
			name:           "unhealthy returns 503",
			healthy:        0,
			expectedStatus: http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atomic.StoreInt32(&healthy, tt.healthy)

			req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
			w := httptest.NewRecorder()

			handler := healthz()
			handler.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

// TestResponseWriter tests the response writer wrapper
func TestResponseWriter(t *testing.T) {
	w := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: w, status: 0, size: 0}

	// Test WriteHeader
	rw.WriteHeader(http.StatusOK)
	if rw.status != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rw.status)
	}

	// Test Write
	data := []byte("test response")
	n, err := rw.Write(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(data) {
		t.Errorf("expected to write %d bytes, wrote %d", len(data), n)
	}
	if rw.size != len(data) {
		t.Errorf("expected size %d, got %d", len(data), rw.size)
	}

	// Test Write without explicit WriteHeader (should default to 200)
	w2 := httptest.NewRecorder()
	rw2 := &responseWriter{ResponseWriter: w2, status: 0, size: 0}
	rw2.Write([]byte("test"))
	if rw2.status != http.StatusOK {
		t.Errorf("expected default status %d, got %d", http.StatusOK, rw2.status)
	}
}

// TestSanitize tests the sanitize function
func TestSanitize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "removes newlines",
			input:    "hello\nworld",
			expected: "hello world",
		},
		{
			name:     "removes carriage returns",
			input:    "hello\rworld",
			expected: "hello world",
		},
		{
			name:     "removes tabs",
			input:    "hello\tworld",
			expected: "hello world",
		},
		{
			name:     "removes control characters",
			input:    "hello\x00\x01\x02world",
			expected: "helloworld",
		},
		{
			name:     "preserves normal text",
			input:    "hello world 123",
			expected: "hello world 123",
		},
		{
			name:     "removes DEL character",
			input:    "hello\x7fworld",
			expected: "helloworld",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitize(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestLoggingMiddleware tests the logging middleware
func TestLoggingMiddleware(t *testing.T) {
	logger := log.New(os.Stdout, "test: ", log.LstdFlags)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(req.Context(), requestIDKey, "test-request-id")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	middleware := logging(logger)(handler)
	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// TestNextRequestID tests the request ID generation
func TestNextRequestID(t *testing.T) {
	id1 := nextRequestID()
	id2 := nextRequestID()

	// IDs should not be empty
	if id1 == "" {
		t.Error("request ID should not be empty")
	}
	if id2 == "" {
		t.Error("request ID should not be empty")
	}

	// IDs should be unique
	if id1 == id2 {
		t.Error("request IDs should be unique")
	}

	// ID should be hex encoded (32 characters for 16 bytes)
	if len(id1) != 32 {
		t.Errorf("expected ID length 32, got %d", len(id1))
	}
}

// TestTracingMiddleware tests the tracing middleware
func TestTracingMiddleware(t *testing.T) {
	tests := []struct {
		name              string
		incomingRequestID string
		expectGenerated   bool
	}{
		{
			name:              "generates request ID when missing",
			incomingRequestID: "",
			expectGenerated:   true,
		},
		{
			name:              "preserves existing request ID",
			incomingRequestID: "existing-id-123",
			expectGenerated:   false,
		},
		{
			name:              "truncates long request ID",
			incomingRequestID: strings.Repeat("a", 200),
			expectGenerated:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestID, ok := r.Context().Value(requestIDKey).(string)
				if !ok || requestID == "" {
					t.Error("request ID should be set in context")
				}

				// Check request ID is not too long
				if len(requestID) > maxRequestIDLength {
					t.Errorf("request ID too long: %d", len(requestID))
				}
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.incomingRequestID != "" {
				req.Header.Set("X-Request-Id", tt.incomingRequestID)
			}
			w := httptest.NewRecorder()

			middleware := tracing(nextRequestID)(handler)
			middleware.ServeHTTP(w, req)

			// Check X-Request-Id header is set in response
			responseRequestID := w.Header().Get("X-Request-Id")
			if responseRequestID == "" {
				t.Error("X-Request-Id header should be set in response")
			}

			if tt.expectGenerated && responseRequestID == tt.incomingRequestID {
				t.Error("expected generated request ID, got incoming ID")
			}

			if !tt.expectGenerated && tt.incomingRequestID != "" && len(tt.incomingRequestID) <= maxRequestIDLength {
				// For short IDs, should preserve the original
				if !strings.HasPrefix(responseRequestID, tt.incomingRequestID[:min(len(tt.incomingRequestID), 10)]) {
					t.Errorf("expected preserved request ID prefix, got %s", responseRequestID)
				}
			}
		})
	}
}

// TestSecurityHeaders tests the security headers middleware
func TestSecurityHeaders(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	middleware := securityHeaders(handler)
	middleware.ServeHTTP(w, req)

	expectedHeaders := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"X-XSS-Protection":        "1; mode=block",
		"Referrer-Policy":         "strict-origin-when-cross-origin",
		"Content-Security-Policy": "default-src 'self'",
	}

	for header, expectedValue := range expectedHeaders {
		if got := w.Header().Get(header); got != expectedValue {
			t.Errorf("header %s: expected %q, got %q", header, expectedValue, got)
		}
	}
}

// TestRecoveryMiddleware tests the panic recovery middleware
func TestRecoveryMiddleware(t *testing.T) {
	logger := log.New(os.Stdout, "test: ", log.LstdFlags)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(req.Context(), requestIDKey, "test-request-id")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	middleware := recovery(logger)(handler)

	// Should not panic
	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

// TestRateLimiter tests the rate limiter
func TestRateLimiter(t *testing.T) {
	limiter := newRateLimiter(3, 10*time.Millisecond)

	// Should allow first 3 requests
	for i := 0; i < 3; i++ {
		if !limiter.allow() {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// Should deny 4th request
	if limiter.allow() {
		t.Error("4th request should be denied")
	}

	// Wait for refill
	time.Sleep(15 * time.Millisecond)

	// Should allow request after refill
	if !limiter.allow() {
		t.Error("request after refill should be allowed")
	}
}

// TestRateLimitMiddleware tests the rate limit middleware
func TestRateLimitMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := rateLimit(2, 100*time.Millisecond)(handler)

	// First 2 requests should succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		middleware.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
		}
	}

	// 3rd request should be rate limited
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected status %d, got %d", http.StatusTooManyRequests, w.Code)
	}

	// Wait for refill
	time.Sleep(150 * time.Millisecond)

	// Request should succeed after refill
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	w = httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d after refill, got %d", http.StatusOK, w.Code)
	}
}

// TestMinFunction tests the min helper function
func TestMinFunction(t *testing.T) {
	tests := []struct {
		name     string
		a        int
		b        int
		expected int
	}{
		{
			name:     "a less than b",
			a:        5,
			b:        10,
			expected: 5,
		},
		{
			name:     "b less than a",
			a:        10,
			b:        5,
			expected: 5,
		},
		{
			name:     "a equals b",
			a:        7,
			b:        7,
			expected: 7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := min(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// BenchmarkNextRequestID benchmarks the request ID generation
func BenchmarkNextRequestID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		nextRequestID()
	}
}

// BenchmarkSanitize benchmarks the sanitize function
func BenchmarkSanitize(b *testing.B) {
	input := "hello\nworld\ttest\rdata"
	for i := 0; i < b.N; i++ {
		sanitize(input)
	}
}

// BenchmarkRateLimiter benchmarks the rate limiter
func BenchmarkRateLimiter(b *testing.B) {
	limiter := newRateLimiter(1000, time.Millisecond)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.allow()
	}
}

// TestIntegrationFullStack tests the full middleware stack
func TestIntegrationFullStack(t *testing.T) {
	logger := log.New(os.Stdout, "test: ", log.LstdFlags)

	router := http.NewServeMux()
	router.Handle("/", index())
	router.Handle("/healthz", healthz())

	handler := recovery(logger)(
		rateLimit(100, time.Second)(
			securityHeaders(
				tracing(nextRequestID)(
					logging(logger)(router)))))

	atomic.StoreInt32(&healthy, 1)

	tests := []struct {
		name           string
		path           string
		expectedStatus int
	}{
		{
			name:           "index handler",
			path:           "/",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "health check",
			path:           "/healthz",
			expectedStatus: http.StatusNoContent,
		},
		{
			name:           "404 page",
			path:           "/notfound",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			// Check security headers are present
			if w.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Error("security headers not set")
			}

			// Check request ID is set
			if w.Header().Get("X-Request-Id") == "" {
				t.Error("X-Request-Id header not set")
			}
		})
	}
}
