FROM alpine:3.19 as builder

RUN apk update && apk add ca-certificates && rm -rf /var/cache/apk/*

RUN adduser -D -g '' app

# ---
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /etc/passwd /etc/passwd

COPY simple-go-server /simple-go-server

USER app

# Note: HEALTHCHECK is not included because this is a scratch container.
# External orchestrators (Kubernetes, Docker Compose, etc.) should use
# the /healthz endpoint for health checks: GET http://container:5000/healthz

ENTRYPOINT ["/simple-go-server"]
