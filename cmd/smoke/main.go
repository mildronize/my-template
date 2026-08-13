// Command smoke is this template's end-to-end smoke test — the Go
// counterpart to my-task's smoke-api-v1.ts
// (~/gits/my-task/src/server/scripts/smoke-api-v1.ts). Same purpose, same
// reason it exists, ported to this project's own domain (todos, not
// my-task's tasks/projects/labels) and this project's own language (Go,
// matching every other cmd/ entrypoint here — my-task's version is
// TypeScript only because that's my-task's language, not because the
// pattern is TypeScript-specific).
//
// Why this exists: every test in this repo through task-5
// (internal/transport/publicapi's own handler tests, bff's,
// internal/invariants_test.go's I1-I14 tests included) either injects the
// actor directly on a gin context built in-process, or signs a session
// cookie directly with the same in-process signer under test — none of
// them go through the real Authorization: Bearer HTTP header ->
// internal/identity.Service.ResolveActor -> key-hash lookup -> database
// round trip, against a genuinely separate running process. That chain is
// this milestone's central security claim — I1 ("identity comes only from
// the resolved credential, never a request field") and I3 ("ownership
// scoping is absence, not permission") — and it is the one thing no
// automated test in the suite touches. This program closes that gap the
// same way smoke-api-v1.ts closes it for my-task: real HTTP, against a
// real running server, with a real minted key, not a fabricated one.
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
//     tests either.
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
//     (handles "smoke-<ts>" and "smoke2-<ts>", one fresh pair per run) and
//     are left behind — cheap, and mirrors smoke-api-v1.ts leaving its one
//     disposable task behind rather than attempting cleanup a real crash
//     could leave inconsistent anyway. The one todo each check creates IS
//     cleaned up before the run ends, since (unlike my-task's tasks, which
//     I12 forbids hard-deleting) this domain's DELETE is real and
//     leaving disposable todos behind would just be litter, not a
//     limitation worth documenting.
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
// this template's example domain (todos: title/done) — the resource path,
// the request/response shapes, and the literal request bodies every check
// further down builds from. Forking this file onto a different domain
// (docs/GETTING-STARTED.md's fork checklist) means editing this block and
// nothing else — the 16 checks in main() below call these names, not
// literals of their own, specifically so "did I update this file for my
// domain" is answerable by reading this one block, not the whole file.
//
// docs/GETTING-STARTED.md spells out the cost of skipping this: this
// program is the ONLY real-HTTP check of I1 (actor-field rejection) and I3
// (ownership scoping) in the entire test suite — every other I1/I3 test
// runs in-process. Fork without touching this block and every check below
// keeps compiling and keeps hitting /todos against a server that has no
// such endpoint anymore — but `go build ./...` and `go test ./...` both
// stay green regardless, because this program has no test file of its own
// and nothing else in the suite runs it. Skipping this update doesn't
// leave a stale test; it leaves zero working verification of I1/I3 over a
// real network path, with nothing to say so.

// resourcePath is the collection endpoint under apiBase this smoke run
// exercises — every check below builds its URL from this, never a literal
// "/todos" of its own.
const resourcePath = "/todos"

// resource is this domain's response shape (_contract/API.md).
type resource struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Done      bool   `json:"done"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// resourceListResponse is GET resourcePath's list envelope.
type resourceListResponse struct {
	Items []resource `json:"todos"`
}

// newCreateBody builds a POST resourcePath request body carrying this
// domain's one required field (title).
func newCreateBody(title string) []byte {
	return []byte(fmt.Sprintf(`{"title":%q}`, title))
}

// newCreateBodyWithForbiddenField is newCreateBody plus one forbidden
// actor-shaped field, for the I1 rejection checks below.
func newCreateBodyWithForbiddenField(title, field string) []byte {
	return []byte(fmt.Sprintf(`{"title":%q,%q:"someone-else"}`, title, field))
}

// newUpdateDoneBody builds a PATCH resourcePath/:id request body toggling
// this domain's one mutable field (done).
func newUpdateDoneBody(done bool) []byte {
	return []byte(fmt.Sprintf(`{"done":%v}`, done))
}

// missingRequiredFieldBody is a create body missing this domain's
// required field(s) entirely — `{}` works for todos because title is its
// only required field; a domain with more than one required field should
// still send a body missing at least one of them here.
var missingRequiredFieldBody = []byte(`{}`)

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

// --- response shapes (_contract/API.md) -----------------------------------
// meResponse and errorEnvelope are identity-shaped/generic, not this
// domain's — they stay here rather than in the edit-zone banner above.
// This domain's own response shapes (resource, resourceListResponse) live
// in that banner, alongside the rest of what a fork needs to touch.

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
		fatalErr(fmt.Errorf("minting the second smoke key (ownership-scoping check): %w", err))
	}
	fmt.Printf("Minted real keys for %q and %q via cmd/issue-key.\n\n", handle1, handle2)

	auth1 := authHeader(key1)
	auth2 := authHeader(key2)

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
		body := newCreateBodyWithForbiddenField("smoke — should be rejected", field)
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

	// ---- 3. real CRUD round-trip -----------------------------------------

	createStatus, createBody, err := doRequest(client, http.MethodPost, apiBase+resourcePath, auth1,
		newCreateBody("smoke crud round-trip — disposable"))
	if err != nil {
		fatalErr(err)
	}
	created := decode[resource](createBody)
	record(fmt.Sprintf("POST %s creates a resource owned by the caller", resourcePath),
		createStatus == http.StatusCreated && created.ID != "" && created.Title == "smoke crud round-trip — disposable" && !created.Done,
		fmt.Sprintf("%d id=%s title=%q done=%v", createStatus, created.ID, created.Title, created.Done), "")
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
		newUpdateDoneBody(true))
	if err != nil {
		fatalErr(err)
	}
	patched := decode[resource](patchBody)
	record(fmt.Sprintf("PATCH %s/:id updates it", resourcePath),
		patchStatus == http.StatusOK && patched.Done,
		fmt.Sprintf("%d done=%v", patchStatus, patched.Done), "")

	deleteStatus, _, err := doRequest(client, http.MethodDelete, apiBase+resourcePath+"/"+id, auth1, nil)
	if err != nil {
		fatalErr(err)
	}
	record(fmt.Sprintf("DELETE %s/:id removes it", resourcePath),
		deleteStatus == http.StatusNoContent,
		fmt.Sprintf("%d", deleteStatus), "")

	getGoneStatus, _, err := doRequest(client, http.MethodGet, apiBase+resourcePath+"/"+id, auth1, nil)
	if err != nil {
		fatalErr(err)
	}
	record(fmt.Sprintf("GET %s/:id confirms it's gone after delete", resourcePath),
		getGoneStatus == http.StatusNotFound,
		fmt.Sprintf("%d", getGoneStatus), "")

	// ---- 4. ownership scoping (I3) ----------------------------------------
	// A second, genuinely different key (a real one, minted for handle2 the
	// same way key1 was) must not be able to see or modify a todo that
	// belongs to key1's caller — and the failure must be 404, indistinguishable
	// from a nonexistent id, never 403 (which would confirm the row exists).

	probeStatus, probeBody, err := doRequest(client, http.MethodPost, apiBase+resourcePath, auth1,
		newCreateBody("smoke ownership probe — disposable"))
	if err != nil {
		fatalErr(err)
	}
	probe := decode[resource](probeBody)
	if probeStatus != http.StatusCreated || probe.ID == "" {
		fatalErr(fmt.Errorf("cannot continue the ownership-scoping check: creating the probe resource returned %d", probeStatus))
	}
	probeID := probe.ID

	crossGetStatus, crossGetBody, err := doRequest(client, http.MethodGet, apiBase+resourcePath+"/"+probeID, auth2, nil)
	if err != nil {
		fatalErr(err)
	}
	crossGetEnv := decode[errorEnvelope](crossGetBody)
	record("a second key's caller cannot GET the first caller's resource (404, not 403)",
		crossGetStatus == http.StatusNotFound,
		fmt.Sprintf("%d %s", crossGetStatus, crossGetEnv.Error.Code), "I3")

	crossPatchStatus, _, err := doRequest(client, http.MethodPatch, apiBase+resourcePath+"/"+probeID, auth2,
		newUpdateDoneBody(true))
	if err != nil {
		fatalErr(err)
	}
	record("a second key's caller cannot PATCH the first caller's resource (404, not 403)",
		crossPatchStatus == http.StatusNotFound,
		fmt.Sprintf("%d", crossPatchStatus), "I3")

	crossDeleteStatus, _, err := doRequest(client, http.MethodDelete, apiBase+resourcePath+"/"+probeID, auth2, nil)
	if err != nil {
		fatalErr(err)
	}
	record("a second key's caller cannot DELETE the first caller's resource (404, not 403)",
		crossDeleteStatus == http.StatusNotFound,
		fmt.Sprintf("%d", crossDeleteStatus), "I3")

	// The rejected cross-owner attempts above must not have snuck through —
	// checked from the real owner's own key, not assumed from the 404s alone.
	stillOwnedStatus, stillOwnedBody, err := doRequest(client, http.MethodGet, apiBase+resourcePath+"/"+probeID, auth1, nil)
	if err != nil {
		fatalErr(err)
	}
	stillOwned := decode[resource](stillOwnedBody)
	record("the owner's resource is untouched by the rejected cross-owner attempts",
		stillOwnedStatus == http.StatusOK && !stillOwned.Done,
		fmt.Sprintf("%d done=%v", stillOwnedStatus, stillOwned.Done), "I3")

	// Cleanup — unlike my-task's tasks (I12 forbids hard delete there), this
	// domain's DELETE is real, so leaving the probe behind would just be
	// litter, not a documented limitation worth preserving.
	if _, _, err := doRequest(client, http.MethodDelete, apiBase+resourcePath+"/"+probeID, auth1, nil); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to clean up probe resource %s: %v\n", probeID, err)
	}

	// ---- 5. spec validation — rejected before handler logic ---------------
	// A request missing CreateTodoRequest's one required field (title) must
	// be refused by openapi.yaml's gin-middleware validator
	// (internal/api.RequestValidator), not reach TodoServer.CreateTodo's own
	// fallback (todo_handler.go's ShouldBindJSON branch, which writes a bare
	// 400 with NO body via c.AbortWithStatus — not the structured error
	// envelope). A non-empty body carrying code "validation_error" is what
	// distinguishes "the validator caught this" from "it reached the handler
	// and the handler's own defensive fallback caught it instead" — the
	// exact distinction this check exists to prove, not just get a 400.

	specStatus, specBody, err := doRequest(client, http.MethodPost, apiBase+resourcePath, auth1, missingRequiredFieldBody)
	if err != nil {
		fatalErr(err)
	}
	specEnv := decode[errorEnvelope](specBody)
	record(fmt.Sprintf("POST %s missing a required field is rejected by the openapi validator before handler logic", resourcePath),
		specStatus == http.StatusBadRequest && len(specBody) > 0 && specEnv.Error.Code == "validation_error",
		fmt.Sprintf("%d %s %q", specStatus, specEnv.Error.Code, specEnv.Error.Message), "")

	// ---- report -------------------------------------------------------

	fmt.Println()
	failed := printReport()
	if failed > 0 {
		os.Exit(1)
	}
}
