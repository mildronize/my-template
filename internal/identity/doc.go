// Package identity owns the users and api_keys tables and the
// actor-resolution middleware (API key -> JWT -> reject). Unlike
// internal/todo, keep this directory on fork — every service built from
// this template needs its own identity/auth seam.
package identity
