# simple-go-server

A minimal, production-ready HTTP web server written in pure Go — using **only the standard library** (zero external dependencies).

It demonstrates best practices for building simple web services by wiring together graceful shutdown, request tracing, structured logging, security headers, rate limiting, and panic recovery into a compact, readable server.

This repository originates from **[this gist](https://gist.github.com/enricofoltran/10b4a980cd07cb02836f70a4ab3e72d7)** (by the same author), which was featured on [Hacker News](https://news.ycombinator.com/item?id=16090977).

Contributions are welcome, but the goal is to keep the code as simple as possible and use only the Go standard library.

---

## Features

- **Graceful shutdown** — handles `SIGINT`/`SIGTERM`, drains in-flight requests (30s timeout), and flips the health flag to `unhealthy` before stopping.
- **Request ID tracking** — generates a cryptographically random `X-Request-Id` (or propagates one you supply), truncates overly long values, and echoes it in the response.
- **Structured request logging** — one line per request with request ID, method, path, remote address, user agent, status code, response size, and duration.
- **Security headers** — `X-Content-Type-Options`, `X-Frame-Options`, `X-XSS-Protection`, `Referrer-Policy`, and `Content-Security-Policy` on every response.
- **Rate limiting** — simple token-bucket limiter (default 100 req/s) that returns `429 Too Many Requests` when the bucket is empty.
- **Panic recovery** — any panic in a handler is recovered, logged, and returned as `500`, keeping the server alive.
- **Request body size limit** — bodies are capped at 10 MB.
- **Log injection protection** — user-controlled values are sanitized before being written to logs.
- **Health check endpoint** — `GET /healthz` for orchestrators (Kubernetes, Docker Compose, etc.), returning `204` when healthy and `503` when shutting down.
- **Zero dependencies & tiny Docker image** — scratched image based on `alpine:3.19`/`scratch`.

---

## Endpoints

| Method | Path      | Description                                                    |
|--------|-----------|----------------------------------------------------------------|
| `GET`  | `/`       | Responds `200 OK` with `Hello, World!`                         |
| `GET`  | `/healthz`| Readiness/liveness probe — `204 No Content` or `503 Unavailable` |

Any other path returns `404 Not Found`.

---

## Getting started

### Prerequisites

- Go **1.24+** (see `go.mod`)
- Docker (optional, for building images)

### Run locally

```bash
# Run directly
go run main.go

# Or use the Makefile
make run
```

By default the server listens on `:5000`:

```bash
curl http://localhost:5000/
curl http://localhost:5000/healthz
```

### Configuration

The only runtime flag is the listen address:

```bash
go run main.go -listen-addr :8080
```

Other knobs are currently hardcoded constants in `main.go`:

| Constant          | Value | Purpose                                   |
|-------------------|-------|-------------------------------------------|
| `maxBodySize`     | 10 MB | Maximum request body size                 |
| `maxRequestIDLength` | 128 | Max accepted `X-Request-Id` length        |
| Rate limit        | 100/s | Token-bucket capacity and refill rate     |
| `ReadTimeout`     | 5s    | Server read timeout                       |
| `WriteTimeout`    | 10s   | Server write timeout                      |
| `IdleTimeout`     | 15s   | Server idle timeout                       |

---

## Architecture

The server runs every request through a middleware chain:

```
request
  └─ recovery      (recover panics → 500)
      └─ rateLimit (token bucket → 429)
          └─ securityHeaders (inject security headers)
              └─ tracing     (resolve X-Request-Id)
                  └─ logging (structured access log)
                      └─ mux router
                          ├─ /        → index
                          └─ /healthz → healthz
```

All middleware is plain `func(http.Handler) http.Handler`, making the pipeline easy to read, reorder, or extend.

---

## Development

All common tasks are wrapped in the [`Makefile`](./Makefile).

```bash
make build        # compile and install into ./bin
make run          # run the server locally
make test         # run tests with -race and coverage
make test-short   # run tests without the race detector
make bench        # run benchmarks
make coverage     # run tests and open a coverage report (coverage.html)
make vet          # go vet
make fmt          # go fmt
make lint         # vet + fmt
make check        # lint + test (all checks)
make clean        # remove build artifacts
```

### Tests

The test suite (`main_test.go`) covers every handler and middleware function:

- `TestIndex` — root vs. 404 paths
- `TestHealthz` — 204 when healthy, 503 when unhealthy
- `TestResponseWriter` — status/size capture
- `TestSanitize` — log-injection sanitization
- `TestLoggingMiddleware` — access logging
- `TestNextRequestID` / `TestTracingMiddleware` — request ID generation and propagation
- `TestSecurityHeaders` — header injection
- `TestRecoveryMiddleware` — panic recovery
- `TestRateLimiter` / `TestRateLimitMiddleware` — token bucket behavior
- `TestIntegrationFullStack` — end-to-end through the full middleware stack

Plus benchmarks for `nextRequestID`, `sanitize`, and `rateLimiter`.

```bash
make test
```

---

## Docker

Build a tiny, static, rootless image from the `rootfs` context.

```bash
make docker-build
```

The image is named `${DOCKER_REGISTRY}/${IMAGE_PREFIX}/${SHORT_NAME}:latest` (defaults to `docker.io/enricofoltran/simple-go-server:latest`) — override with the corresponding Makefile variables.

```bash
# Push to a registry
make docker-push
```

Run it:

```bash
docker run --rm -p 5000:5000 docker.io/enricofoltran/simple-go-server:latest
```

> **Note:** The container has no `HEALTHCHECK` because it's built on `scratch`. External orchestrators should use the `/healthz` endpoint directly: `GET http://<container>:5000/healthz`.

---

## License

[MIT](LICENSE) © Enrico Foltran
