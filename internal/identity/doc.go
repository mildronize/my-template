// Package identity owns the users and api_keys tables and the
// actor-resolution logic (API key -> JWT -> reject, service.go's
// ResolveActor) behind it — the gin middleware that calls ResolveActor on
// every request (RequireActor/RejectActorFields) lives in
// internal/transport/publicapi instead, since this package holds no
// transport code (ARCHITECTURE.md — "Why identity is not under domain/",
// which explains why identity is its own top-level layer even though it,
// like a domain module, keeps no HTTP-facing code of its own). Unlike
// internal/domain/todo, keep this directory on fork — every service built
// from this template needs its own identity/auth seam.
package identity
