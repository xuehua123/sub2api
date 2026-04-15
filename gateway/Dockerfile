ARG GOLANG_IMAGE=golang:1.26.1-alpine
ARG ALPINE_IMAGE=alpine:3.21
ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn

FROM ${GOLANG_IMAGE} AS builder

ARG GOPROXY
ARG GOSUMDB

ENV GOPROXY=${GOPROXY}
ENV GOSUMDB=${GOSUMDB}

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app/gateway

COPY gateway/go.mod ./
RUN go mod download

COPY gateway/ ./

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /app/lobehub-gateway \
    ./cmd/lobehub-gateway

FROM ${ALPINE_IMAGE}

RUN apk add --no-cache ca-certificates tzdata wget && \
    addgroup -g 1000 lobehub-gateway && \
    adduser -u 1000 -G lobehub-gateway -s /bin/sh -D lobehub-gateway

WORKDIR /app

COPY --from=builder /app/lobehub-gateway /app/lobehub-gateway

RUN chown -R lobehub-gateway:lobehub-gateway /app

USER lobehub-gateway

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget -q -T 3 -O /dev/null http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/app/lobehub-gateway"]
