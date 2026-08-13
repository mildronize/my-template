#!/usr/bin/env bash
#
# Registers this service's Hydra OAuth2 client for one environment, for
# the owner-login flow (authorization_code + PKCE, sso-consumer-
# contract.md §2 — internal/transport/bff's GET /login + GET /callback).
#
#   ENV=dev \
#     SERVICE_NAME=my-real-service \
#     SERVICE_PUBLIC_URL=http://localhost:8080 \
#     SSO_ISSUER=https://sso.example.com \
#     HYDRA_ADMIN_URL=http://127.0.0.1:4445 \
#     HYDRA_PUBLIC_URL=http://127.0.0.1:4444 \
#     ./scripts/register.sh
#
#   ENV=prod \
#     SERVICE_NAME=my-real-service \
#     SERVICE_PUBLIC_URL=https://my-real-service.example.com \
#     SSO_ISSUER=https://sso.example.com \
#     HYDRA_ADMIN_URL=http://127.0.0.1:4445 \
#     HYDRA_PUBLIC_URL=http://127.0.0.1:4444 \
#     ./scripts/register.sh
#
# See docs/DEPLOY-REQUIREMENTS.md's "Owner-login Hydra client
# registration" section for what each variable means and where its value
# comes from, and docs/GETTING-STARTED.md's Step 1 for why this has to
# run before owner login works at all -- including for local-only dev.
#
# WHY THIS FILE EXISTS / WHERE IT CAME FROM. Adapted from
# prod-thw-home's deploy/sso/scripts/register-my-task.sh -- Freya's
# script, hardcoded for my-task's own literal values. This version is
# generalized so a fork can register its own client without copying that
# script and hand-editing my-task's values out of it. The structure and
# every safety property are preserved on purpose, not simplified away:
#   - refuses to overwrite an existing client rather than silently
#     recreating it
#   - reads the registration back from Hydra rather than trusting the
#     create call's own output (two different questions: what the command
#     returned, and what the server now holds)
#   - probes the authorize endpoint with BOTH a registered and an
#     unregistered redirect URI, so a pass actually proves the
#     registration is enforced rather than that Hydra accepts anything
#   - PRINTS the resulting env values and never writes them to any file
#     -- a re-run can't silently clobber a working config; the consumer
#     owns its own env file
#
# Every value below is read from the environment. Nothing in this file is
# a real service's literal value -- see the block comment right below for
# what each placeholder means.
set -euo pipefail

# --- Placeholders, filled in from the environment at run time ----------
#
# SERVICE_NAME        This service's stable name, e.g. "my-real-service"
#                      (docs/GETTING-STARTED.md Step 3, "Rename the
#                      service"). Used verbatim in the client id.
# SERVICE_PUBLIC_URL   This service's own public URL for the ENVIRONMENT
#                      BEING REGISTERED right now -- e.g.
#                      http://localhost:8080 when ENV=dev,
#                      https://my-real-service.example.com when ENV=prod.
#                      Becomes both AUDIENCE and the base of the redirect
#                      URI. Never an opaque name (contract §6, "Audience
#                      convention").
# SSO_ISSUER           Hydra's issuer URL, e.g. https://sso.example.com --
#                      the same value this service's own SSO_ISSUER
#                      config var (internal/platform/config.go) must be
#                      set to. Only used here to echo back in the printed
#                      env block below; this script never calls it.
# HYDRA_ADMIN_URL      Hydra's Admin API, reachable from wherever this
#                      script runs -- e.g. http://127.0.0.1:4445.
#                      Deliberately not defaulted to 127.0.0.1: a fork
#                      won't necessarily run this on the same host/fleet
#                      register-my-task.sh was written for.
# HYDRA_PUBLIC_URL     Hydra's public (browser-facing) endpoint, used only
#                      by the authorize-endpoint probe below -- e.g.
#                      http://127.0.0.1:4444.
# ENV                  dev or prod. No default, on purpose -- see the
#                      case statement below. Run this script once per
#                      environment you deploy to.
#
# Optional:
#
# CLIENT_SECRET        Supply an existing secret to make a rebuild
#                      transparent to this service -- otherwise Hydra
#                      mints a new one and this service's env has to be
#                      updated. Pass via the environment, never as an
#                      argument (arguments are world-readable in /proc).
# HYDRA_IMAGE          Defaults to oryd/hydra:v2.3 -- the Hydra CLI image
#                      this script shells out to.

: "${SERVICE_NAME:?Set SERVICE_NAME -- see docs/DEPLOY-REQUIREMENTS.md}"
: "${SERVICE_PUBLIC_URL:?Set SERVICE_PUBLIC_URL -- this service's own public URL for the environment being registered, see docs/DEPLOY-REQUIREMENTS.md}"
: "${SSO_ISSUER:?Set SSO_ISSUER -- Hydra's issuer URL, see docs/DEPLOY-REQUIREMENTS.md}"
: "${HYDRA_ADMIN_URL:?Set HYDRA_ADMIN_URL -- Hydra's Admin API, see docs/DEPLOY-REQUIREMENTS.md}"
: "${HYDRA_PUBLIC_URL:?Set HYDRA_PUBLIC_URL -- Hydra's public endpoint, see docs/DEPLOY-REQUIREMENTS.md}"

HYDRA_ADMIN="$HYDRA_ADMIN_URL"
HYDRA_PUBLIC="$HYDRA_PUBLIC_URL"
HYDRA_IMAGE="${HYDRA_IMAGE:-oryd/hydra:v2.3}"

# ENV=dev / ENV=prod only -- no third `uat`-style case the way
# register-my-task.sh has one. That third case is a second Hydra
# instance on a second, fleet-specific host (thw-home-prod) with its own
# database and issuer; a fresh fork starts on one host with one Hydra
# instance, so baking in a second host's address here would be real
# complexity for a case day-one forks don't hit. Nothing about this
# script's structure assumes exactly two environments -- add a third
# `case` arm the same shape as dev/prod below if and when a real second
# deployment target shows up.
case "${ENV:-}" in
dev)
	CLIENT_ID="${SERVICE_NAME}-dev"
	;;
prod)
	CLIENT_ID="${SERVICE_NAME}"
	;;
*)
	echo "Set ENV=dev or ENV=prod. No default on purpose -- they differ in" >&2
	echo "audience, and a token minted for one being accepted by the other" >&2
	echo "is exactly what one-audience-per-environment exists to prevent." >&2
	exit 2
	;;
esac

# AUDIENCE is this service's own public URL for the environment being
# registered right now -- never opaque (contract §6, "Audience
# convention"; docs/DEPLOY-REQUIREMENTS.md's "Audience convention"
# section restates it for this service specifically).
AUDIENCE="$SERVICE_PUBLIC_URL"

# The BFF's callback route is a fixed path (internal/transport/bff's
# GET /callback, _contract/API.md) -- only the host/scheme varies per
# service and per environment, which SERVICE_PUBLIC_URL already carries.
REDIRECT_URIS=("${SERVICE_PUBLIC_URL%/}/callback")

# An existing secret can be supplied to make a rebuild transparent to the
# consumer -- otherwise Hydra mints a new one and this service's env has
# to be updated. Pass it via the environment, never as an argument
# (arguments are world-readable in /proc).
#   CLIENT_SECRET="$(...)" ENV=dev ./scripts/register.sh
SECRET_ARGS=()
if [ -n "${CLIENT_SECRET:-}" ]; then
	SECRET_ARGS=(--secret "$CLIENT_SECRET")
fi

hydra() { docker run --rm --network host "$HYDRA_IMAGE" "$@"; }

echo "==> Checking Hydra's Admin API is reachable"
if ! curl -fsS --max-time 5 -o /dev/null "$HYDRA_ADMIN/health/ready"; then
	echo "Hydra Admin API not answering at $HYDRA_ADMIN -- start it (or" >&2
	echo "check HYDRA_ADMIN_URL) before registering anything." >&2
	exit 1
fi

# Refuse rather than overwrite. `create` on an existing id fails anyway,
# but failing here says why, and says it before a half-finished change.
echo "==> Checking whether $CLIENT_ID already exists"
if hydra get oauth2-client "$CLIENT_ID" --endpoint "$HYDRA_ADMIN" --format json >/dev/null 2>&1; then
	echo >&2
	echo "$CLIENT_ID is already registered. Not touching it." >&2
	echo >&2
	echo "To CHANGE one field (e.g. add a redirect URI), use a JSON Patch --" >&2
	echo "never 'hydra update', whose own help says it replaces the entire" >&2
	echo "client, so an omitted field is a lost field, the secret included:" >&2
	echo >&2
	echo "  curl -X PATCH $HYDRA_ADMIN/admin/clients/$CLIENT_ID \\" >&2
	echo "    -H 'Content-Type: application/json' \\" >&2
	echo "    -d '[{\"op\":\"add\",\"path\":\"/redirect_uris/-\",\"value\":\"…\"}]'" >&2
	echo >&2
	echo "To RE-CREATE it from scratch, delete it first:" >&2
	echo "  hydra delete oauth2-client $CLIENT_ID --endpoint $HYDRA_ADMIN" >&2
	exit 3
fi

echo "==> Registering $CLIENT_ID (audience $AUDIENCE)"
REDIRECT_ARGS=()
for uri in "${REDIRECT_URIS[@]}"; do
	REDIRECT_ARGS+=(--redirect-uri "$uri")
done

# --access-token-strategy jwt is non-negotiable: Hydra's default is
# opaque (unsigned), which JWKS-based validation rejects outright rather
# than on audience or permission grounds -- so the failure does not look
# like a configuration problem.
#
# --id is passed explicitly so a rebuild recreates the SAME client id.
# Without it Hydra assigns a random UUID and every consumer needs
# reconfiguring after a recovery.
#
# PKCE needs no flag here -- it is enforced globally by Hydra's
# oauth2.pkce.enforced (see hydra.yml.template), and this service's own
# bff sends code_challenge unconditionally (I11).
CLIENT_JSON=$(hydra create oauth2-client \
	--endpoint "$HYDRA_ADMIN" \
	--id "$CLIENT_ID" \
	--name "$CLIENT_ID" \
	--grant-type authorization_code,refresh_token \
	--response-type code \
	--token-endpoint-auth-method client_secret_basic \
	--scope openid,offline \
	--audience "$AUDIENCE" \
	--access-token-strategy jwt \
	"${REDIRECT_ARGS[@]}" \
	"${SECRET_ARGS[@]}" \
	--format json)

CLIENT_SECRET_OUT=$(printf '%s' "$CLIENT_JSON" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("client_secret",""))')

# Read the registration back from Hydra rather than trusting the create
# call's own output. Two different questions: what the command returned,
# and what the server now holds.
echo "==> Verifying against Hydra"
READBACK=$(hydra get oauth2-client "$CLIENT_ID" --endpoint "$HYDRA_ADMIN" --format json)
# JSON goes in as an argument, not on stdin: a heredoc and a redirect
# both claim stdin, and the redirect wins -- python then reads the JSON
# as its own source and dies on `false`.
python3 -c '
import json, sys
want_id, want_aud, raw = sys.argv[1], sys.argv[2], sys.argv[3]
c = json.loads(raw)
checks = [
    ("client_id", c.get("client_id"), want_id),
    ("audience", c.get("audience"), [want_aud]),
    ("access_token_strategy", c.get("access_token_strategy"), "jwt"),
    ("token_endpoint_auth_method", c.get("token_endpoint_auth_method"), "client_secret_basic"),
    ("grant_types", sorted(c.get("grant_types") or []), ["authorization_code", "refresh_token"]),
]
bad = [(n, got, want) for n, got, want in checks if got != want]
for n, got, want in checks:
    print("    %s %s: %s" % ("ok  " if (n, got, want) not in bad else "FAIL", n, got))
print("    ---- redirect_uris: %s" % (c.get("redirect_uris"),))
sys.exit(1 if bad else 0)
' "$CLIENT_ID" "$AUDIENCE" "$READBACK"

# Prove the registration is actually enforced, with the case that SHOULD
# fail. Without the second arm, an acceptance cannot be told apart from
# Hydra accepting everything -- and a check that cannot fail has not
# passed.
echo "==> Probing the authorize endpoint (both arms)"
CHALLENGE=$(python3 -c "
import base64, hashlib
v = 'a' * 64
print(base64.urlsafe_b64encode(hashlib.sha256(v.encode()).digest()).rstrip(b'=').decode())")

probe() {
	curl -sS -o /dev/null -w '%{redirect_url}' -G "$HYDRA_PUBLIC/oauth2/auth" \
		--data-urlencode "client_id=$CLIENT_ID" \
		--data-urlencode "response_type=code" \
		--data-urlencode "scope=openid" \
		--data-urlencode "redirect_uri=$1" \
		--data-urlencode "code_challenge=$CHALLENGE" \
		--data-urlencode "code_challenge_method=S256" \
		--data-urlencode "state=probe0123456789abcdef"
}

good=$(probe "${REDIRECT_URIS[0]}")
bad=$(probe "https://not-registered.invalid/callback")

ok=0
case "$good" in *"login_challenge="*) echo "    ok   registered URI  -> /login" ;; *) echo "    FAIL registered URI  -> $good"; ok=1 ;; esac
case "$bad" in *"fallbacks/error"*) echo "    ok   unregistered URI -> error fallback" ;; *) echo "    FAIL unregistered URI was NOT rejected -> $bad"; ok=1 ;; esac
[ "$ok" -eq 0 ] || { echo "Authorize probe failed -- the client exists but is not behaving as registered." >&2; exit 1; }

cat <<EOF

==> Done. This service's env needs these ($ENV). Nothing was written for
    you -- paste them into this service's own config, chmod 600.

SSO_ISSUER=$SSO_ISSUER
SSO_CLIENT_ID=$CLIENT_ID
SSO_CLIENT_SECRET=${CLIENT_SECRET_OUT:-<unchanged -- you supplied CLIENT_SECRET>}
AUTH_AUDIENCE=$AUDIENCE

Hydra never returns a client secret after creation, so this is the only
time it is printed. If it is lost, the client has to be re-created or
patched -- restart this service and confirm the owner-login flow still
works, report the result either way, not only on failure.
EOF
