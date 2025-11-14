// Package main implements a minimal, production-ready HTTP web server
// demonstrating best practices for building simple web services with
// essential features while maintaining zero external dependencies.
//
// Features:
//   - Graceful shutdown with signal handling
//   - Request ID tracking and propagation
//   - Structured logging with request context
//   - Security headers and input validation
//   - Rate limiting and panic recovery
//   - Health check endpoint for orchestration
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type key int

const (
	requestIDKey key = 0
)

const (
	maxRequestIDLength = 128
	maxBodySize        = 10 * 1024 * 1024 // 10MB
)

var (
	Version      string
	GitTag       string
	GitCommit    string
	GitTreeState string
	listenAddr   string
	healthy      int32 // atomic access only - indicates server health status (1=healthy, 0=unhealthy)
)

func main() {
	flag.StringVar(&listenAddr, "listen-addr", ":5000", "server listen address")
	flag.Parse()

	logger := log.New(os.Stdout, "http: ", log.LstdFlags)

	logger.Println("Simple go server")
	logger.Println("Version:", Version)
	logger.Println("GitTag:", GitTag)
	logger.Println("GitCommit:", GitCommit)
	logger.Println("GitTreeState:", GitTreeState)

	logger.Println("Server is starting...")

	router := http.NewServeMux()
	router.Handle("/", index())
	router.Handle("/healthz", healthz())

	// Wrap handlers with middleware chain: recovery -> rate limiting -> security headers -> tracing -> logging
	handler := recovery(logger)(
		rateLimit(100, time.Second)(
			securityHeaders(
				tracing(nextRequestID)(
					logging(logger)(router)))))

	server := &http.Server{
		Addr:         listenAddr,
		Handler:      handler,
		ErrorLog:     logger,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	done := make(chan bool)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-quit
		logger.Println("Server is shutting down...")
		atomic.StoreInt32(&healthy, 0)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		server.SetKeepAlivesEnabled(false)
		if err := server.Shutdown(ctx); err != nil {
			logger.Printf("Could not gracefully shutdown the server: %v\n", err)
		}
		close(done)
	}()

	logger.Println("Server is ready to handle requests at", listenAddr)
	atomic.StoreInt32(&healthy, 1)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Printf("Could not listen on %s: %v\n", listenAddr, err)
		os.Exit(1)
	}

	<-done
	logger.Println("Server stopped")
}

func index() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
		// Limit request body size
		r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "Hello, World!")
	})
}

func healthz() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadInt32(&healthy) == 1 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code and response size
type responseWriter struct {
	http.ResponseWriter
	status int
	size   int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.status == 0 {
		rw.status = http.StatusOK
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.size += n
	return n, err
}

// sanitize removes newlines and control characters from strings to prevent log injection
func sanitize(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	// Remove other control characters
	var result strings.Builder
	for _, r := range s {
		if r >= 32 && r != 127 {
			result.WriteRune(r)
		}
	}
	return result.String()
}

func logging(logger *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Limit request body size
			r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

			start := time.Now()
			wrapped := &responseWriter{ResponseWriter: w, status: 0, size: 0}

			defer func() {
				requestID, ok := r.Context().Value(requestIDKey).(string)
				if !ok {
					requestID = "unknown"
				}
				// Sanitize user-controlled input to prevent log injection
				method := sanitize(r.Method)
				path := sanitize(r.URL.Path)
				remoteAddr := sanitize(r.RemoteAddr)
				userAgent := sanitize(r.UserAgent())
				duration := time.Since(start)

				logger.Printf("%s %s %s %s %s status=%d size=%d duration=%v",
					requestID, method, path, remoteAddr, userAgent,
					wrapped.status, wrapped.size, duration)
			}()
			next.ServeHTTP(wrapped, r)
		})
	}
}

// nextRequestID generates a cryptographically random request ID
func nextRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func tracing(nextRequestID func() string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get("X-Request-Id")
			if requestID == "" {
				requestID = nextRequestID()
			}
			// Validate and limit request ID length to prevent header bombs
			if len(requestID) > maxRequestIDLength {
				requestID = requestID[:maxRequestIDLength]
			}
			// Sanitize request ID
			requestID = sanitize(requestID)

			ctx := context.WithValue(r.Context(), requestIDKey, requestID)
			w.Header().Set("X-Request-Id", requestID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// securityHeaders adds security-related HTTP headers to all responses
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		next.ServeHTTP(w, r)
	})
}

// recovery recovers from panics and logs them, preventing server crashes
func recovery(logger *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					requestID, _ := r.Context().Value(requestIDKey).(string)
					logger.Printf("panic recovered: requestID=%s error=%v", requestID, err)
					http.Error(w, http.StatusText(http.StatusInternalServerError),
						http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// rateLimiter implements a simple token bucket rate limiter
type rateLimiter struct {
	tokens   int
	capacity int
	refill   time.Duration
	mu       sync.Mutex
	lastTime time.Time
}

func newRateLimiter(capacity int, refill time.Duration) *rateLimiter {
	return &rateLimiter{
		tokens:   capacity,
		capacity: capacity,
		refill:   refill,
		lastTime: time.Now(),
	}
}

func (rl *rateLimiter) allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastTime)

	// Refill tokens based on elapsed time
	tokensToAdd := int(elapsed / rl.refill)
	if tokensToAdd > 0 {
		rl.tokens = min(rl.capacity, rl.tokens+tokensToAdd)
		rl.lastTime = now
	}

	if rl.tokens > 0 {
		rl.tokens--
		return true
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// rateLimit applies a simple per-server rate limit
func rateLimit(capacity int, refill time.Duration) func(http.Handler) http.Handler {
	limiter := newRateLimiter(capacity, refill)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.allow() {
				http.Error(w, http.StatusText(http.StatusTooManyRequests),
					http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
