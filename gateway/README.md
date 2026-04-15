# LobeHub Chat Gateway

This directory contains the standalone chat-domain gateway used by the Sub2API <-> official LobeHub integration.

Its job is to sit in front of an unmodified LobeHub deployment and handle:

- direct-open zero-click SSO bootstrapping
- `refresh-target` redirects back to Sub2API
- bootstrap ticket exchange / consume flow
- reverse proxying normal requests to official LobeHub

## Current Sync Strategy

The gateway now uses an actual `ConfigProbe` flow for existing authenticated LobeHub sessions:

- if there is no LobeHub session, it auto-posts to LobeHub Generic OIDC
- if there is a session but no valid target token, it redirects to `sub2api /auth/lobehub-sso?mode=refresh-target`
- if there is a session and a valid target token, it calls official LobeHub `GET /trpc/lambda/user.getUserState`
- the gateway extracts current `keyVaults` + `languageModel` settings and sends them to `POST /api/v1/lobehub/config-probe/compare`
- only when Sub2API confirms the current persisted settings already match the desired config does it proxy straight through
- otherwise it calls `POST /api/v1/lobehub/bootstrap-exchange` and sends the browser to `__lobehub_bootstrap`

The gateway still writes a shared sync cookie during bootstrap completion, but request-time allow/skip decisions are now driven by the persistent-settings probe rather than that browser-local hint.

## Environment Variables

Copy `gateway/.env.example` and set:

- `LOBEHUB_UPSTREAM_URL`
  The official LobeHub service behind the gateway, for example `http://lobehub:3210`.
- `SUB2API_API_BASE_URL`
  The Sub2API backend API base URL, including `/api/v1`.
- `SUB2API_FRONTEND_URL`
  The user-facing Sub2API frontend URL used for `refresh-target` redirects.
- `LISTEN_ADDR`
  Gateway listen address, default `:8080`.
- `LOBEHUB_PROVIDER_ID`
  Generic OIDC provider id, default `generic-oidc`.
- `LOBEHUB_BOOTSTRAP_PATH`
  Chat-domain bootstrap endpoint, default `/__lobehub_bootstrap`.
- `LOBEHUB_USER_STATE_PATH`
  Official LobeHub route used by ConfigProbe, default `/trpc/lambda/user.getUserState`.

## Run Locally

```bash
cd gateway
go test ./...
go run ./cmd/lobehub-gateway
```

## Build Image

```bash
docker build -f gateway/Dockerfile -t sub2api/lobehub-gateway:local .
```

## Required Topology

Recommended domains:

- `app.example.com` for Sub2API frontend
- `api.example.com` for Sub2API backend
- `chat.example.com` for this gateway

The gateway expects:

- official LobeHub configured for `Generic OIDC`
- `AUTH_DISABLE_EMAIL_PASSWORD=1`
- `AUTH_SSO_PROVIDERS=generic-oidc`
- official LobeHub `user.getUserState` route available at `LOBEHUB_USER_STATE_PATH` (default `/trpc/lambda/user.getUserState`)
- shared-domain cookies to work across your chosen subdomains

## Entry Points

- normal page requests: gateway decides whether to proxy, refresh target, or bootstrap
- `/__lobehub_bootstrap`: consumes the latest bootstrap ticket and redirects to the final `?settings=...` URL
- `/healthz`: lightweight readiness/liveness endpoint returning `200 ok`
- API/static/auth callback routes: proxied directly to official LobeHub
