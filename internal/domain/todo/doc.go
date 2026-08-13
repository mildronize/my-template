// Package todo is the example domain module for this template. It holds
// the todo business logic (service.go) and data access (repo.go) that
// exist to demonstrate the identity/auth seam working end to end — the
// domain itself is deliberately minimal. It holds no transport code: the
// HTTP handlers that expose this domain over the public API live in
// internal/transport/publicapi/todo_handler.go instead, outside every
// domain module (ARCHITECTURE.md — "Why transport is not inside a domain
// module anymore").
//
// On fork: this directory is eventually deleted whole (`rm -rf
// internal/domain/todo`) and replaced with your own domain module — but
// not as step one. Study it first (that's what reading this file is),
// then copy it into your new module and get it wired in alongside the
// original; deleting internal/domain/todo is the second-to-last step, not
// the first. See docs/GETTING-STARTED.md Step 4, which spells out why and
// in what order.
package todo
