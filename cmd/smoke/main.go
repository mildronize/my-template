// Command smoke is this template's end-to-end smoke test — the Go
// counterpart to my-task's smoke-api-v1.ts
// (~/gits/my-task/src/server/scripts/smoke-api-v1.ts). Same purpose, same
// reason it exists, ported to this project's own domain (todos, not
// my-task's tasks/projects/labels) and this project's own language (Go,
// matching every other cmd/ entrypoint here — my-task's version is
// TypeScript only because that's my-task's language, not because the
// pattern is TypeScript-specific).
//
// Why this exists: every test in this repo (internal/transport/publicapi's
// own handler tests, bff's, internal/invariants_test.go's I1-I21 tests
// included) either injects the actor directly on a gin context built
// in-process, or signs a session cookie directly with the same in-process
// signer under test — none of them go through the real
// Authorization: Bearer HTTP header -> internal/identity.Service.ResolveActor
// -> key-hash lookup -> database round trip, against a genuinely separate
// running process. That chain is this project's central security claim —
// I1 ("identity comes only from the resolved credential, never a request
// field") most of all — and it is the one thing no automated test in the
// suite touches. This program closes that gap the same way smoke-api-v1.ts
// closes it for my-task: real HTTP, against a real running server, with a
// real minted key, not a fabricated one.
//
// milestone-4: this file went untouched across the entire milestone-4
// branch — it still spoke the pre-milestone-4 wire shape (done: bool,
// DELETE /api/v1/todos/:id) with zero awareness of /events, the four new
// todo fields, or the removed DELETE, and nothing in `make test` (the
// milestone's own full-suite gate) ever ran `make smoke` to notice. This
// revision brings it onto the milestone-4 surface and, along the way,
// proves several of that milestone's invariants over a real network path
// for the first time: I16 (a `created` event is never client-specifiable,
// only ever POST /todos' own side effect), I19 (a repeated clientRequestId
// writes nothing twice, on both POST /todos and POST .../events), the
// removed DELETE's genuine 404 (no route, not a 405), and I18 (an agent
// key cannot move a todo to status: closed). See "What this program
// cannot check" below for what's deliberately out of reach.
//
// Usage:
//
//	go run ./cmd/server &                     # start a live instance first — not this script's job
//	go run ./cmd/smoke                        # SMOKE_BASE_URL defaults to http://localhost:8080
//	SMOKE_BASE_URL=https://host go run ./cmd/smoke
//	make smoke                                 # same thing, via the Makefile target
//
// Exits non-zero if any check fails, so it can gate a deploy.
//
// Assumptions this script makes (documented rather than hidden, per this
// milestone's own convention — see cmd/issue-key's header comment for the
// same style):
//
//   - A live instance of this service is already running and reachable at
//     SMOKE_BASE_URL. This script only ever calls out over HTTP to that
//     address — it never starts, stops, seeds, or otherwise manages the
//     server process. That's a human's or CI's job, run before this
//     script, exactly the way `go test ./...` never starts the server it
//     tests either. Point it at a genuinely throwaway instance/database —
//     every check below writes real rows nothing cleans up (see below) —
//     never at an instance something else depends on.
//   - This program itself runs from the repo root (`go run ./cmd/smoke`
//     and `make smoke` both satisfy this — every other Makefile target in
//     this repo assumes the same thing), in the same environment
//     (DATABASE_PATH / .env) as that running server. It mints its own two
//     disposable test keys by shelling out to
//     `go run ./cmd/issue-key <handle>` — the exact command
//     docs/DEPLOY-REQUIREMENTS.md tells an operator to run, the only path
//     in this codebase that ever mints a raw tpl_ key (_contract/API.md:
//     "no POST /api/v1/keys and no key-rotation HTTP endpoint... issuance
//     is CLI-only") — against that same database file. If this process's
//     DATABASE_PATH doesn't point at the database the running server
//     under test actually reads, the minted keys will be valid nowhere
//     reachable at SMOKE_BASE_URL and every authenticated check below
//     will fail with 401 — that is a misconfigured smoke run, not a
//     server bug, and is exactly what I5 ("401 never leaks why") should
//     make it look like from the HTTP side alone.
//   - Two disposable users get created as a side effect of the above
//     (handles "smoke-<ts>" and "smoke2-<ts>", one fresh pair per run),
//     plus every todo this run creates — nothing is cleaned up.
//     milestone-4 removed DELETE /api/v1/todos/:id entirely (GOAL.md,
//     mirroring my-task's own I12: this domain's history is not
//     supposed to be erasable), so unlike this file's pre-milestone-4
//     version — which deleted its own probe todo before exiting — there
//     is no code path left that could clean one up even if this script
//     wanted to. Left-behind rows are the expected, permanent shape of
//     running this against a real instance now, exactly like
//     smoke-api-v1.ts already accepts for my-task's own tasks; this is
//     the reason a throwaway instance/database is a hard requirement
//     above, not just a suggestion.
//
// What this program cannot check:
//
//   - GET /api/bff/activity (the cross-todo activity feed) and every
//     other /api/bff/* route are session-authenticated only
//     (bff.RequireJSONSession, a signed browser cookie from a real SSO
//     login) — there is no Bearer-key path onto that surface at all, by
//     design (_contract/API.md). This script authenticates exclusively
//     with cmd/issue-key-minted agent keys, the only credential kind an
//     agent can hold, so it structurally cannot drive a session and
//     structurally cannot reach that surface — not a gap in this
//     script's coverage so much as a boundary of what an agent-key
//     credential is allowed to reach at all. What this script proves
//     instead, on the publicapi surface it can reach: GET
//     /todos/:id/events (this todo's own timeline, oldest first) returns
//     the same events an owner's feed read would show for that todo —
//     same actor/type/payload/body shape — which is as close as an
//     agent-only credential can get to the feed's own data without ever
//     touching the feed endpoint itself.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// ============================================================================
// ==== EDIT THIS FOR YOUR DOMAIN ============================================
// ============================================================================
//
// Everything between this banner and the matching one below is specific to
// this template's example domain (todos: title/status/assignee/priority/
// dueDate, plus their event timeline) — the resource path, the
// request/response shapes, and the literal request bodies every check
// further down builds from. Forking this file onto a different domain
// (docs/GETTING-STARTED.md's fork checklist) means editing this block and
// nothing else — the checks in main() below call these names, not
// literals of their own, specifically so "did I update this file for my
// domain" is answerable by reading this one block, not the whole file.
//
// docs/GETTING-STARTED.md spells out the cost of skipping this: this
// program is the ONLY real-HTTP check of I1 (actor-field rejection) in
// the entire test suite — every other I1 test runs in-process. Fork
// without touching this block and every check below keeps compiling and
// keeps hitting /todos against a server that has no such endpoint
// anymore — but `go build ./...` and `go test ./...` both stay green
// regardless, because this program has no test file of its own and
// nothing else in the suite runs it. Skipping this update doesn't leave
// a stale test; it leaves zero working verification of I1 over a real
// network path, with nothing to say so.

// resourcePath is the collection endpoint under apiBase this smoke run
// exercises — every check below builds its URL from this, never a literal
// "/todos" of its own.
const resourcePath = "/todos"

// resource is this domain's response shape (_contract/API.md /
// openapi.yaml's Todo schema).
type resource struct {
	ID             string  `json:"id"`
	Title          string  `json:"title"`
	Status         string  `json:"status"`
	AssigneeID     *string `json:"assigneeId"`
	AssigneeHandle *string `json:"assigneeHandle"`
	Priority       *string `json:"priority"`
	DueDate        *string `json:"dueDate"`
	CreatedBy      string  `json:"createdBy"`
	CreatedAt      string  `json:"createdAt"`
	UpdatedAt      string  `json:"updatedAt"`
}

// resourceListResponse is GET resourcePath's list envelope.
type resourceListResponse struct {
	Items []resource `json:"todos"`
}

// resourceEvent is this domain's event shape (openapi.yaml's TodoEvent
// schema) — one row of resourcePath's own timeline.
type resourceEvent struct {
	ID              string                 `json:"id"`
	TodoID          string                 `json:"todoId"`
	Seq             int64                  `json:"seq"`
	ActorID         string                 `json:"actorId"`
	ActorHandle     string                 `json:"actorHandle"`
	Type            string                 `json:"type"`
	Payload         map[string]interface{} `json:"payload"`
	Body            *string                `json:"body"`
	ClientRequestID string                 `json:"clientRequestId"`
	CreatedAt       string                 `json:"createdAt"`
}

// resourceEventListResponse is GET resourcePath/:id/events's envelope.
type resourceEventListResponse struct {
	Items []resourceEvent `json:"events"`
}

// newCreateBody builds a POST resourcePath request body carrying this
// domain's one required field (title) plus I19's required idempotency
// key.
func newCreateBody(title, clientRequestID string) []byte {
	return []byte(fmt.Sprintf(`{"title":%q,"clientRequestId":%q}`, title, clientRequestID))
}

// newCreateBodyWithForbiddenField is newCreateBody plus one forbidden
// actor-shaped field, for the I1 rejection checks below.
func newCreateBodyWithForbiddenField(title, clientRequestID, field string) []byte {
	return []byte(fmt.Sprintf(`{"title":%q,"clientRequestId":%q,%q:"someone-else"}`, title, clientRequestID, field))
}

// newUpdateTitleBody builds a PATCH resourcePath/:id request body — the
// only field this endpoint still writes as of milestone-4 (status/
// assignee/priority/dueDate all moved to the event endpoint below).
func newUpdateTitleBody(title, clientRequestID string) []byte {
	return []byte(fmt.Sprintf(`{"title":%q,"clientRequestId":%q}`, title, clientRequestID))
}

// missingRequiredFieldBody is a create body missing this domain's
// required field(s) entirely — `{}` works for todos because title and
// clientRequestId are its only required fields; a domain with more
// required fields should still send a body missing at least one of them
// here.
var missingRequiredFieldBody = []byte(`{}`)

// --- event-body builders (POST resourcePath/:id/events) -------------------
// One shape covers every type (openapi.yaml's CreateTodoEventRequest) —
// these builders just fill in the fields each type actually reads,
// mirroring the handler's own per-type dispatch.

func newCreatedEventBody(clientRequestID string) []byte {
	return []byte(fmt.Sprintf(`{"type":"created","clientRequestId":%q}`, clientRequestID))
}

func newCommentedEventBody(body, clientRequestID string) []byte {
	return []byte(fmt.Sprintf(`{"type":"commented","body":%q,"clientRequestId":%q}`, body, clientRequestID))
}

func newStatusChangedEventBody(to, clientRequestID string) []byte {
	return []byte(fmt.Sprintf(`{"type":"status_changed","to":%q,"clientRequestId":%q}`, to, clientRequestID))
}

// newAssignedEventBody's to is a pointer: nil means "unassign" (an
// explicit `"to": null`, not an omitted key — matches the handler's own
// documented equivalence of the two).
func newAssignedEventBody(to *string, clientRequestID string) []byte {
	if to == nil {
		return []byte(fmt.Sprintf(`{"type":"assigned","to":null,"clientRequestId":%q}`, clientRequestID))
	}
	return []byte(fmt.Sprintf(`{"type":"assigned","to":%q,"clientRequestId":%q}`, *to, clientRequestID))
}

func newFieldChangedEventBody(field, to, clientRequestID string) []byte {
	return []byte(fmt.Sprintf(`{"type":"field_changed","field":%q,"to":%q,"clientRequestId":%q}`, field, to, clientRequestID))
}

// ============================================================================
// ==== END DOMAIN-SPECIFIC BLOCK ============================================
// ============================================================================

// --- check bookkeeping -------------------------------------------------

type check struct {
	name      string
	invariant string
	ok        bool
	detail    string
}

var checks []check

func record(name string, ok bool, detail, invariant string) {
	checks = append(checks, check{name: name, invariant: invariant, ok: ok, detail: detail})
}

// printReport prints every recorded check as PASS/FAIL, mirroring
// smoke-api-v1.ts's own report format, and returns how many failed.
func printReport() int {
	failed := 0
	for _, c := range checks {
		if !c.ok {
			failed++
		}
		mark := "PASS"
		if !c.ok {
			mark = "FAIL"
		}
		tag := ""
		if c.invariant != "" {
			tag = fmt.Sprintf(" (%s)", c.invariant)
		}
		fmt.Printf("  %s  %s%s\n        %s\n", mark, c.name, tag, c.detail)
	}
	fmt.Printf("\n%d/%d passed.\n", len(checks)-failed, len(checks))
	return failed
}

// fatalErr prints whatever was recorded so far (so a hard failure midway
// still shows what passed before it), then the fatal error, then exits
// non-zero. Mirrors smoke-api-v1.ts's top-level `.catch()`.
func fatalErr(err error) {
	if len(checks) > 0 {
		printReport()
	}
	fmt.Fprintf(os.Stderr, "\nSmoke run failed: %v\n", err)
	os.Exit(1)
}

// --- HTTP helpers --------------------------------------------------------

func doRequest(client *http.Client, method, url string, headers map[string]string, body []byte) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return 0, nil, fmt.Errorf("building %s %s: %w", method, url, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("%s %s: %w", method, url, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("reading response body for %s %s: %w", method, url, err)
	}
	return resp.StatusCode, respBody, nil
}

func decode[T any](body []byte) T {
	var v T
	if len(body) == 0 {
		return v
	}
	_ = json.Unmarshal(body, &v) // best-effort — a malformed body just leaves v zero-valued, which fails the check that reads it
	return v
}

func authHeader(key string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + key}
}

func mergeHeaders(a, b map[string]string) map[string]string {
	out := make(map[string]string, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func strPtr(s string) *string { return &s }

// --- response shapes (_contract/API.md) -----------------------------------
// meResponse and errorEnvelope are identity-shaped/generic, not this
// domain's — they stay here rather than in the edit-zone banner above.
// This domain's own response shapes (resource, resourceListResponse,
// resourceEvent, resourceEventListResponse) live in that banner, alongside
// the rest of what a fork needs to touch.

type meResponse struct {
	Handle string `json:"handle"`
	Role   string `json:"role"`
	Active bool   `json:"active"`
}

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Hint    string `json:"hint"`
	} `json:"error"`
}

// --- key minting (via cmd/issue-key, never fabricated) --------------------

// tplKeyLine matches a raw tpl_ key on a line by itself — the exact shape
// generateRawAPIKey (internal/identity/service.go) produces: "tpl_" plus
// 64 hex characters. Anchored to the whole line so it can't accidentally
// match cmd/issue-key's own "Prefix: tpl_a1b2c3d4  Expires: ..." summary
// line, which also contains "tpl_" but isn't the raw key.
var tplKeyLine = regexp.MustCompile(`^tpl_[0-9a-f]{64}$`)

// issueKeyViaCmd shells out to `go run ./cmd/issue-key <handle>` — the
// only place in this codebase a raw key is ever minted — and parses its
// stdout for the raw key line. This is what "a real minted API key" means
// for this script: never a fabricated tpl_-looking string, always a
// genuine api_keys row created by the same CLI tool an operator would run
// by hand, against whatever database this process's own DATABASE_PATH
// (env or .env) resolves to.
func issueKeyViaCmd(handle string) (string, error) {
	cmd := exec.Command("go", "run", "./cmd/issue-key", handle)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("go run ./cmd/issue-key %s: %w\nstderr:\n%s", handle, err, stderr.String())
	}
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if tplKeyLine.MatchString(line) {
			return line, nil
		}
	}
	return "", fmt.Errorf("could not find a tpl_<hex> key line in `go run ./cmd/issue-key %s` stdout:\n%s", handle, stdout.String())
}

// --- main ------------------------------------------------------------------

func resolveBaseURL() string {
	if v := os.Getenv("SMOKE_BASE_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://localhost:8080"
}

func main() {
	base := resolveBaseURL()
	apiBase := base + "/api/v1"
	client := &http.Client{Timeout: 10 * time.Second}

	fmt.Printf("Smoke: %s\n", apiBase)

	// Preflight: is anything listening at all? Everything below assumes
	// yes (see package doc). Checked once here so a down server produces
	// one clear message instead of every check below failing with a
	// confusing connection-refused detail.
	if _, _, err := doRequest(client, http.MethodGet, base+"/healthz", nil, nil); err != nil {
		fmt.Fprintf(os.Stderr, "\nNo live instance reachable at %s: %v\n", base, err)
		fmt.Fprintln(os.Stderr, "This script does not start the server itself — see cmd/smoke/main.go's usage comment.")
		os.Exit(1)
	}

	ts := time.Now().UnixNano()
	handle1 := fmt.Sprintf("smoke-%d", ts)
	handle2 := fmt.Sprintf("smoke2-%d", ts)

	key1, err := issueKeyViaCmd(handle1)
	if err != nil {
		fatalErr(fmt.Errorf("minting the primary smoke key: %w", err))
	}
	key2, err := issueKeyViaCmd(handle2)
	if err != nil {
		fatalErr(fmt.Errorf("minting the second smoke key (shared-collection check): %w", err))
	}
	fmt.Printf("Minted real keys for %q and %q via cmd/issue-key.\n\n", handle1, handle2)

	auth1 := authHeader(key1)
	auth2 := authHeader(key2)

	// crSeq makes a unique clientRequestId per write below — every write
	// this script makes needs its own, since I19 makes a repeat into a
	// no-op rather than a second write, and this script wants each check
	// below to actually happen.
	crSeq := 0
	nextCR := func(label string) string {
		crSeq++
		return fmt.Sprintf("smoke-%d-%s-%d", ts, label, crSeq)
	}

	// ---- 1. identity ---------------------------------------------------

	noCredStatus, _, err := doRequest(client, http.MethodGet, apiBase+"/me", nil, nil)
	if err != nil {
		fatalErr(err)
	}
	record("GET /me with no credential is refused",
		noCredStatus == http.StatusUnauthorized,
		fmt.Sprintf("%d", noCredStatus), "")

	meStatus, meBody, err := doRequest(client, http.MethodGet, apiBase+"/me", auth1, nil)
	if err != nil {
		fatalErr(err)
	}
	me := decode[meResponse](meBody)
	record("GET /me with a real key identifies the caller",
		meStatus == http.StatusOK && me.Handle == handle1 && me.Role == "agent" && me.Active,
		fmt.Sprintf("%d handle=%s role=%s active=%v", meStatus, me.Handle, me.Role, me.Active), "")

	// ---- 2. actor-field rejection (I1) ----------------------------------

	for _, field := range []string{"actor", "actorId", "ownerId"} {
		body := newCreateBodyWithForbiddenField("smoke — should be rejected", nextCR("forbidden-"+field), field)
		status, respBody, err := doRequest(client, http.MethodPost, apiBase+resourcePath, auth1, body)
		if err != nil {
			fatalErr(err)
		}
		env := decode[errorEnvelope](respBody)
		record(fmt.Sprintf("POST %s with %q in the body is rejected, not silently ignored", resourcePath, field),
			status == http.StatusBadRequest && env.Error.Code == "actor_field_present",
			fmt.Sprintf("%d %s", status, env.Error.Code), "I1")
	}

	xActorStatus, xActorBody, err := doRequest(client, http.MethodGet, apiBase+"/me",
		mergeHeaders(auth1, map[string]string{"X-Actor": "someone-else"}), nil)
	if err != nil {
		fatalErr(err)
	}
	xActorEnv := decode[errorEnvelope](xActorBody)
	record("GET /me with an X-Actor header is rejected, not silently ignored",
		xActorStatus == http.StatusBadRequest && xActorEnv.Error.Code == "actor_field_present",
		fmt.Sprintf("%d %s", xActorStatus, xActorEnv.Error.Code), "I1")

	// ---- 3. real CRUD round-trip, and the removed DELETE's genuine 404 ---

	createStatus, createBody, err := doRequest(client, http.MethodPost, apiBase+resourcePath, auth1,
		newCreateBody("smoke crud round-trip — disposable", nextCR("crud-create")))
	if err != nil {
		fatalErr(err)
	}
	created := decode[resource](createBody)
	record(fmt.Sprintf("POST %s creates a resource owned by the caller, status open, no assignee/priority/dueDate yet", resourcePath),
		createStatus == http.StatusCreated && created.ID != "" && created.Title == "smoke crud round-trip — disposable" &&
			created.Status == "open" && created.AssigneeID == nil && created.Priority == nil && created.DueDate == nil && created.CreatedBy != "",
		fmt.Sprintf("%d id=%s status=%s createdBy=%s", createStatus, created.ID, created.Status, created.CreatedBy), "")
	if created.ID == "" {
		fatalErr(fmt.Errorf("cannot continue the CRUD check: POST %s did not return an id", resourcePath))
	}
	id := created.ID

	listStatus, listBody, err := doRequest(client, http.MethodGet, apiBase+resourcePath, auth1, nil)
	if err != nil {
		fatalErr(err)
	}
	list := decode[resourceListResponse](listBody)
	foundInList := false
	for _, t := range list.Items {
		if t.ID == id {
			foundInList = true
			break
		}
	}
	record(fmt.Sprintf("GET %s includes the created resource", resourcePath),
		listStatus == http.StatusOK && foundInList,
		fmt.Sprintf("%d found=%v of %d item(s)", listStatus, foundInList, len(list.Items)), "")

	patchStatus, patchBody, err := doRequest(client, http.MethodPatch, apiBase+resourcePath+"/"+id, auth1,
		newUpdateTitleBody("smoke crud round-trip — renamed", nextCR("crud-patch")))
	if err != nil {
		fatalErr(err)
	}
	patched := decode[resource](patchBody)
	record(fmt.Sprintf("PATCH %s/:id renames it (status/assignee/priority/dueDate moved to the event endpoint, not this one)", resourcePath),
		patchStatus == http.StatusOK && patched.Title == "smoke crud round-trip — renamed",
		fmt.Sprintf("%d title=%q", patchStatus, patched.Title), "")

	doneBodyStatus, doneBodyResp, err := doRequest(client, http.MethodPatch, apiBase+resourcePath+"/"+id, auth1,
		[]byte(fmt.Sprintf(`{"title":"still renamed","clientRequestId":%q,"done":true}`, nextCR("crud-stray-done"))))
	if err != nil {
		fatalErr(err)
	}
	doneBodyEnv := decode[errorEnvelope](doneBodyResp)
	record("PATCH with a stray \"done\" field is rejected, not silently accepted (done was removed, replaced by status)",
		doneBodyStatus == http.StatusBadRequest,
		fmt.Sprintf("%d %s", doneBodyStatus, doneBodyEnv.Error.Code), "")

	deleteStatus, _, err := doRequest(client, http.MethodDelete, apiBase+resourcePath+"/"+id, auth1, nil)
	if err != nil {
		fatalErr(err)
	}
	record(fmt.Sprintf("DELETE %s/:id no longer exists — a genuine 404 (no route registered), not a 405 and not a success", resourcePath),
		deleteStatus == http.StatusNotFound,
		fmt.Sprintf("%d", deleteStatus), "")

	getAfterDeleteAttemptStatus, getAfterDeleteAttemptBody, err := doRequest(client, http.MethodGet, apiBase+resourcePath+"/"+id, auth1, nil)
	if err != nil {
		fatalErr(err)
	}
	getAfterDeleteAttempt := decode[resource](getAfterDeleteAttemptBody)
	record("the resource still exists after the rejected DELETE attempt (the 404 above was routing, not a hidden delete)",
		getAfterDeleteAttemptStatus == http.StatusOK && getAfterDeleteAttempt.ID == id,
		fmt.Sprintf("%d id=%s", getAfterDeleteAttemptStatus, getAfterDeleteAttempt.ID), "")

	// ---- 4. shared collection — I3 no longer scopes todos -----------------
	// milestone-4's Ownership model decision: todos went from
	// private-per-actor to a shared collection every agent and the owner
	// act on together. A second, genuinely different key (a real one,
	// minted for handle2 the same way key1 was) must be able to read AND
	// write the first caller's todo — the opposite of what this same
	// section checked before milestone-4 — and an unknown id must still
	// be a plain 404 with no other case left to produce one.

	crossGetStatus, crossGetBody, err := doRequest(client, http.MethodGet, apiBase+resourcePath+"/"+id, auth2, nil)
	if err != nil {
		fatalErr(err)
	}
	crossGet := decode[resource](crossGetBody)
	record("a second key's caller CAN GET the first caller's todo (I3 no longer scopes todos, GOAL.md)",
		crossGetStatus == http.StatusOK && crossGet.ID == id,
		fmt.Sprintf("%d id=%s", crossGetStatus, crossGet.ID), "I3")

	crossPatchStatus, crossPatchBody, err := doRequest(client, http.MethodPatch, apiBase+resourcePath+"/"+id, auth2,
		newUpdateTitleBody("smoke crud round-trip — renamed by the second caller", nextCR("shared-patch")))
	if err != nil {
		fatalErr(err)
	}
	crossPatched := decode[resource](crossPatchBody)
	record("a second key's caller CAN PATCH the first caller's todo, and the write genuinely lands",
		crossPatchStatus == http.StatusOK && crossPatched.Title == "smoke crud round-trip — renamed by the second caller",
		fmt.Sprintf("%d title=%q", crossPatchStatus, crossPatched.Title), "I3")

	confirmStatus, confirmBody, err := doRequest(client, http.MethodGet, apiBase+resourcePath+"/"+id, auth1, nil)
	if err != nil {
		fatalErr(err)
	}
	confirm := decode[resource](confirmBody)
	record("the first caller's own read shows the second caller's write (one shared row, not two divergent views)",
		confirmStatus == http.StatusOK && confirm.Title == "smoke crud round-trip — renamed by the second caller",
		fmt.Sprintf("%d title=%q", confirmStatus, confirm.Title), "I3")

	unknownIDStatus, unknownIDBody, err := doRequest(client, http.MethodGet, apiBase+resourcePath+"/00000000-0000-0000-0000-000000000000", auth1, nil)
	if err != nil {
		fatalErr(err)
	}
	unknownIDEnv := decode[errorEnvelope](unknownIDBody)
	record("an id that was never issued is still a plain 404 — the only case left, now that there is no 'wrong owner' (I3)",
		unknownIDStatus == http.StatusNotFound && unknownIDEnv.Error.Code == "not_found",
		fmt.Sprintf("%d %s", unknownIDStatus, unknownIDEnv.Error.Code), "I3")

	// ---- 5. event-append flow (I15/I16/I18/I19) ----------------------------
	// A fresh todo, dedicated to this section, so its event count/ordering
	// assertions below aren't polluted by section 3/4's own writes to id.

	flowCreateStatus, flowCreateBody, err := doRequest(client, http.MethodPost, apiBase+resourcePath, auth1,
		newCreateBody("smoke event-append flow — disposable", nextCR("flow-create")))
	if err != nil {
		fatalErr(err)
	}
	flow := decode[resource](flowCreateBody)
	if flowCreateStatus != http.StatusCreated || flow.ID == "" {
		fatalErr(fmt.Errorf("cannot continue the event-append flow check: creating its todo returned %d", flowCreateStatus))
	}
	flowID := flow.ID
	flowEventsURL := apiBase + resourcePath + "/" + flowID + "/events"

	// I16: "created" is never client-specifiable — POST .../events has no
	// case that produces one, "created" included, same as any other
	// unrecognised type.
	createdRejectStatus, createdRejectBody, err := doRequest(client, http.MethodPost, flowEventsURL, auth1,
		newCreatedEventBody(nextCR("flow-created-rejected")))
	if err != nil {
		fatalErr(err)
	}
	createdRejectEnv := decode[errorEnvelope](createdRejectBody)
	record(`POST .../events with type: "created" is rejected — a created event only ever happens as POST /todos' own side effect`,
		createdRejectStatus == http.StatusBadRequest && createdRejectEnv.Error.Code == "validation_error",
		fmt.Sprintf("%d %s", createdRejectStatus, createdRejectEnv.Error.Code), "I16")

	// commented
	commentStatus, commentBody, err := doRequest(client, http.MethodPost, flowEventsURL, auth1,
		newCommentedEventBody("smoke says hello", nextCR("flow-comment")))
	if err != nil {
		fatalErr(err)
	}
	commentEvent := decode[resourceEvent](commentBody)
	record("POST .../events type: commented appends a real comment event, attributed to the caller",
		commentStatus == http.StatusCreated && commentEvent.Type == "commented" &&
			commentEvent.ActorHandle == handle1 && commentEvent.Body != nil && *commentEvent.Body == "smoke says hello",
		fmt.Sprintf("%d type=%s actorHandle=%s body=%v", commentStatus, commentEvent.Type, commentEvent.ActorHandle, commentEvent.Body), "I15")

	// status_changed
	statusChangeStatus, statusChangeBody, err := doRequest(client, http.MethodPost, flowEventsURL, auth1,
		newStatusChangedEventBody("in_progress", nextCR("flow-status")))
	if err != nil {
		fatalErr(err)
	}
	statusChangeEvent := decode[resourceEvent](statusChangeBody)
	record("POST .../events type: status_changed moves the todo and records {from,to} in the payload",
		statusChangeStatus == http.StatusCreated && statusChangeEvent.Payload["from"] == "open" && statusChangeEvent.Payload["to"] == "in_progress",
		fmt.Sprintf("%d payload=%v", statusChangeStatus, statusChangeEvent.Payload), "I15")

	// assigned — to the todo's own creator, the one real user id this
	// script can name without a way to look up handle2's id (GET /me
	// never returns an id, only handle/role/active).
	assignStatus, assignBody, err := doRequest(client, http.MethodPost, flowEventsURL, auth1,
		newAssignedEventBody(&flow.CreatedBy, nextCR("flow-assign")))
	if err != nil {
		fatalErr(err)
	}
	assignEvent := decode[resourceEvent](assignBody)
	record("POST .../events type: assigned appends an assignment event naming the new assignee",
		assignStatus == http.StatusCreated && assignEvent.Payload["to"] != nil,
		fmt.Sprintf("%d payload=%v", assignStatus, assignEvent.Payload), "I15")

	afterAssignStatus, afterAssignBody, err := doRequest(client, http.MethodGet, apiBase+resourcePath+"/"+flowID, auth1, nil)
	if err != nil {
		fatalErr(err)
	}
	afterAssign := decode[resource](afterAssignBody)
	record("the todo's own assigneeId/assigneeHandle reflect the assignment (handle-exposure fix-round's own field)",
		afterAssignStatus == http.StatusOK && afterAssign.AssigneeID != nil && *afterAssign.AssigneeID == flow.CreatedBy &&
			afterAssign.AssigneeHandle != nil && *afterAssign.AssigneeHandle == handle1,
		fmt.Sprintf("%d assigneeId=%v assigneeHandle=%v", afterAssignStatus, afterAssign.AssigneeID, afterAssign.AssigneeHandle), "")

	// assigned — to an id that syntactically could be a user but resolves
	// to nobody (ErrUnknownAssignee -> 400 validation_error hint "to").
	unknownAssigneeStatus, unknownAssigneeBody, err := doRequest(client, http.MethodPost, flowEventsURL, auth1,
		newAssignedEventBody(strPtr("00000000-0000-0000-0000-000000000000"), nextCR("flow-assign-unknown")))
	if err != nil {
		fatalErr(err)
	}
	unknownAssigneeEnv := decode[errorEnvelope](unknownAssigneeBody)
	record(`an "assigned" event whose "to" id resolves to nobody is a validation_error, not a silent write`,
		unknownAssigneeStatus == http.StatusBadRequest && unknownAssigneeEnv.Error.Code == "validation_error" && unknownAssigneeEnv.Error.Hint == "to",
		fmt.Sprintf("%d %s hint=%s", unknownAssigneeStatus, unknownAssigneeEnv.Error.Code, unknownAssigneeEnv.Error.Hint), "")

	// unassign (to: null)
	unassignStatus, _, err := doRequest(client, http.MethodPost, flowEventsURL, auth1,
		newAssignedEventBody(nil, nextCR("flow-unassign")))
	if err != nil {
		fatalErr(err)
	}
	afterUnassignStatus, afterUnassignBody, err := doRequest(client, http.MethodGet, apiBase+resourcePath+"/"+flowID, auth1, nil)
	if err != nil {
		fatalErr(err)
	}
	afterUnassign := decode[resource](afterUnassignBody)
	record(`type: assigned with "to": null clears the assignment`,
		unassignStatus == http.StatusCreated && afterUnassignStatus == http.StatusOK &&
			afterUnassign.AssigneeID == nil && afterUnassign.AssigneeHandle == nil,
		fmt.Sprintf("unassign=%d get=%d assigneeId=%v assigneeHandle=%v", unassignStatus, afterUnassignStatus, afterUnassign.AssigneeID, afterUnassign.AssigneeHandle), "")

	// field_changed — priority
	priorityStatus, _, err := doRequest(client, http.MethodPost, flowEventsURL, auth1,
		newFieldChangedEventBody("priority", "high", nextCR("flow-priority")))
	if err != nil {
		fatalErr(err)
	}
	afterPriorityStatus, afterPriorityBody, err := doRequest(client, http.MethodGet, apiBase+resourcePath+"/"+flowID, auth1, nil)
	if err != nil {
		fatalErr(err)
	}
	afterPriority := decode[resource](afterPriorityBody)
	record("field_changed field: priority actually changes the todo's priority",
		priorityStatus == http.StatusCreated && afterPriorityStatus == http.StatusOK &&
			afterPriority.Priority != nil && *afterPriority.Priority == "high",
		fmt.Sprintf("post=%d priority=%v", priorityStatus, afterPriority.Priority), "")

	// field_changed — dueDate
	dueDate := "2026-09-01T00:00:00Z"
	dueDateStatus, _, err := doRequest(client, http.MethodPost, flowEventsURL, auth1,
		newFieldChangedEventBody("dueDate", dueDate, nextCR("flow-duedate")))
	if err != nil {
		fatalErr(err)
	}
	afterDueDateStatus, afterDueDateBody, err := doRequest(client, http.MethodGet, apiBase+resourcePath+"/"+flowID, auth1, nil)
	if err != nil {
		fatalErr(err)
	}
	afterDueDate := decode[resource](afterDueDateBody)
	record("field_changed field: dueDate actually changes the todo's due date",
		dueDateStatus == http.StatusCreated && afterDueDateStatus == http.StatusOK &&
			afterDueDate.DueDate != nil && *afterDueDate.DueDate == dueDate,
		fmt.Sprintf("post=%d dueDate=%v", dueDateStatus, afterDueDate.DueDate), "")

	// I18: an agent key may not move a todo to status: closed — the one
	// restriction the fixed four-value status enum exists to bind. This
	// project has never had a distinct 403, so the rejection is the same
	// 401 unauthorized shape a bad credential would produce (I5).
	closedStatus, closedBody, err := doRequest(client, http.MethodPost, flowEventsURL, auth1,
		newStatusChangedEventBody("closed", nextCR("flow-close-attempt")))
	if err != nil {
		fatalErr(err)
	}
	closedEnv := decode[errorEnvelope](closedBody)
	record("an agent key cannot move a todo to status: closed — owner-only, rejected as 401 unauthorized (no distinct 403 on this surface)",
		closedStatus == http.StatusUnauthorized && closedEnv.Error.Code == "unauthorized",
		fmt.Sprintf("%d %s", closedStatus, closedEnv.Error.Code), "I18")

	stillNotClosedStatus, stillNotClosedBody, err := doRequest(client, http.MethodGet, apiBase+resourcePath+"/"+flowID, auth1, nil)
	if err != nil {
		fatalErr(err)
	}
	stillNotClosed := decode[resource](stillNotClosedBody)
	record("the rejected close attempt did not sneak through — status is untouched by it",
		stillNotClosedStatus == http.StatusOK && stillNotClosed.Status != "closed",
		fmt.Sprintf("%d status=%s", stillNotClosedStatus, stillNotClosed.Status), "I18")

	// I19: repeating the comment event's exact clientRequestId returns
	// the original write, unchanged, and creates nothing new.
	repeatCR := nextCR("flow-comment-repeat-source")
	firstOfPairStatus, firstOfPairBody, err := doRequest(client, http.MethodPost, flowEventsURL, auth1,
		newCommentedEventBody("idempotency probe", repeatCR))
	if err != nil {
		fatalErr(err)
	}
	firstOfPair := decode[resourceEvent](firstOfPairBody)

	preRepeatListStatus, preRepeatListBody, err := doRequest(client, http.MethodGet, flowEventsURL, auth1, nil)
	if err != nil {
		fatalErr(err)
	}
	preRepeatList := decode[resourceEventListResponse](preRepeatListBody)

	repeatStatus, repeatBody, err := doRequest(client, http.MethodPost, flowEventsURL, auth1,
		newCommentedEventBody("idempotency probe", repeatCR))
	if err != nil {
		fatalErr(err)
	}
	repeatEvent := decode[resourceEvent](repeatBody)

	postRepeatListStatus, postRepeatListBody, err := doRequest(client, http.MethodGet, flowEventsURL, auth1, nil)
	if err != nil {
		fatalErr(err)
	}
	postRepeatList := decode[resourceEventListResponse](postRepeatListBody)

	record("repeating a clientRequestId on POST .../events returns the original event, and writes nothing new",
		firstOfPairStatus == http.StatusCreated && repeatStatus == http.StatusCreated &&
			repeatEvent.ID == firstOfPair.ID && preRepeatListStatus == http.StatusOK && postRepeatListStatus == http.StatusOK &&
			len(postRepeatList.Items) == len(preRepeatList.Items),
		fmt.Sprintf("first=%s repeat=%s before=%d after=%d events", firstOfPair.ID, repeatEvent.ID, len(preRepeatList.Items), len(postRepeatList.Items)), "I19")

	// I19, the other write path: repeating POST /todos' own
	// clientRequestId returns the same todo, not a second one.
	dupeCreateCR := nextCR("dup-create")
	firstCreateStatus, firstCreateBody, err := doRequest(client, http.MethodPost, apiBase+resourcePath, auth1,
		newCreateBody("smoke idempotent-create probe", dupeCreateCR))
	if err != nil {
		fatalErr(err)
	}
	firstCreate := decode[resource](firstCreateBody)
	secondCreateStatus, secondCreateBody, err := doRequest(client, http.MethodPost, apiBase+resourcePath, auth1,
		newCreateBody("smoke idempotent-create probe", dupeCreateCR))
	if err != nil {
		fatalErr(err)
	}
	secondCreate := decode[resource](secondCreateBody)
	record("repeating a clientRequestId on POST /todos returns the same todo, and creates nothing new",
		firstCreateStatus == http.StatusCreated && secondCreateStatus == http.StatusCreated &&
			firstCreate.ID != "" && secondCreate.ID == firstCreate.ID,
		fmt.Sprintf("first=%s second=%s", firstCreate.ID, secondCreate.ID), "I19")

	// The full timeline, read back oldest-first (the endpoint's own
	// documented order, unlike bff's newest-first cross-todo feed) —
	// "created" must be its first row, since that's the one event type
	// this flow never posted directly (I16) yet the todo undeniably has
	// one.
	timelineStatus, timelineBody, err := doRequest(client, http.MethodGet, flowEventsURL, auth1, nil)
	if err != nil {
		fatalErr(err)
	}
	timeline := decode[resourceEventListResponse](timelineBody)
	orderedBySeq := true
	for i := 1; i < len(timeline.Items); i++ {
		if timeline.Items[i].Seq <= timeline.Items[i-1].Seq {
			orderedBySeq = false
			break
		}
	}
	firstIsCreated := len(timeline.Items) > 0 && timeline.Items[0].Type == "created"
	record("GET .../events returns this todo's full timeline, oldest first, starting with its own created event",
		timelineStatus == http.StatusOK && orderedBySeq && firstIsCreated,
		fmt.Sprintf("%d events, ordered=%v firstType=%s", len(timeline.Items), orderedBySeq, timeline.Items[0].Type), "I15")

	// ---- 6. spec validation — rejected before handler logic ---------------
	// A request missing CreateTodoRequest's required fields (title,
	// clientRequestId) must be refused by openapi.yaml's gin-middleware
	// validator (internal/api.RequestValidator), not reach
	// TodoServer.CreateTodo's own fallback (todo_handler.go's
	// ShouldBindJSON branch, which writes a bare 400 with NO body via
	// c.AbortWithStatus — not the structured error envelope). A
	// non-empty body carrying code "validation_error" is what
	// distinguishes "the validator caught this" from "it reached the
	// handler and the handler's own defensive fallback caught it
	// instead" — the exact distinction this check exists to prove, not
	// just get a 400.

	specStatus, specBody, err := doRequest(client, http.MethodPost, apiBase+resourcePath, auth1, missingRequiredFieldBody)
	if err != nil {
		fatalErr(err)
	}
	specEnv := decode[errorEnvelope](specBody)
	record(fmt.Sprintf("POST %s missing required fields is rejected by the openapi validator before handler logic", resourcePath),
		specStatus == http.StatusBadRequest && len(specBody) > 0 && specEnv.Error.Code == "validation_error",
		fmt.Sprintf("%d %s %q", specStatus, specEnv.Error.Code, specEnv.Error.Message), "")

	// ---- report -------------------------------------------------------

	fmt.Println()
	fmt.Println("Not covered by this run (see cmd/smoke/main.go's own doc comment for why):")
	fmt.Println("  - GET /api/bff/activity and every other /api/bff/* route — session-only, no Bearer-key path reaches them.")
	fmt.Println()
	failed := printReport()
	if failed > 0 {
		os.Exit(1)
	}
}
