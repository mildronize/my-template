// The e2e local OIDC issuer's login/consent provider — TPL-3.
//
// Hydra has no built-in login/consent UI: it redirects an unauthenticated
// /oauth2/auth request to whatever `urls.login`/`urls.consent` name
// (../hydra/hydra.yml), and expects that something to call Hydra's own
// admin API to accept or reject, then redirect back. This file is that
// something — modeled on (never copied from) the real login/consent
// provider this fleet runs for production Hydra
// (~/gits/prod-thw-home/sso/app, read once for shape/reference only,
// never touched, never run) — reduced to the minimum a test needs:
// auto-accept for one fixed test subject, no interactive form. What's
// under test here is my-template's own redirect/callback/PKCE handling
// (internal/transport/bff), not whether a login form can be typed into —
// Hydra treats this file as an opaque redirect target either way, so a
// form would add surface without adding proof.
//
// Deliberately plain Node `http`, zero dependencies: this is ~6 REST
// calls (https://www.ory.sh/docs/hydra/reference/api#tag/oAuth2), not
// enough surface to justify a framework, and every dependency here is one
// more thing gate 3's "no test-only code in the product" grep has to be
// sure never leaked into internal/ or web/src (it can't, structurally —
// this file isn't imported by either, only run as its own process by
// docker-compose.yml).
import { createServer } from "node:http";

const ADMIN_URL = process.env.HYDRA_ADMIN_URL ?? "http://127.0.0.1:4445";
const PORT = Number(process.env.PORT ?? 4446);

// The one identity this stub ever authenticates as — must match whatever
// SEED_OWNER_SSO_SUBJECT the throwaway e2e database was seeded with
// (global-setup.ts), the same way a real deployment's owner row is
// identified by a real Hydra sub (cmd/seed/main.go's own doc comment:
// "a template's owner is a single, known person per deployment...
// identified in advance by their real Hydra sub claim"). This is that,
// scoped to one fixed test value instead of a real human's.
const TEST_SUBJECT = process.env.E2E_TEST_SUBJECT ?? "e2e-test-owner-sub";

async function hydraAdmin(path, init) {
  const res = await fetch(`${ADMIN_URL}${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...init?.headers },
  });
  if (!res.ok) {
    throw new Error(`Hydra admin API ${path} -> ${res.status}: ${await res.text()}`);
  }
  return res.json();
}

function redirect(res, location) {
  res.writeHead(302, { Location: location });
  res.end();
}

const server = createServer(async (req, res) => {
  try {
    const url = new URL(req.url, `http://${req.headers.host}`);

    if (url.pathname === "/health") {
      res.writeHead(200);
      res.end("ok");
      return;
    }

    if (url.pathname === "/login") {
      const challenge = url.searchParams.get("login_challenge");
      if (!challenge) {
        res.writeHead(400);
        res.end("missing login_challenge");
        return;
      }
      // remember/remember_for: false/0 — this stub starts a fresh login
      // every run on purpose. A remembered Hydra session would let a
      // second spec silently skip the login screen it's supposed to be
      // testing, which is exactly the kind of "green for the wrong
      // reason" this project keeps naming and fixing elsewhere.
      const { redirect_to } = await hydraAdmin(
        `/admin/oauth2/auth/requests/login/accept?login_challenge=${challenge}`,
        { method: "PUT", body: JSON.stringify({ subject: TEST_SUBJECT, remember: false }) },
      );
      redirect(res, redirect_to);
      return;
    }

    if (url.pathname === "/consent") {
      const challenge = url.searchParams.get("consent_challenge");
      if (!challenge) {
        res.writeHead(400);
        res.end("missing consent_challenge");
        return;
      }
      const consentRequest = await hydraAdmin(
        `/admin/oauth2/auth/requests/consent?consent_challenge=${challenge}`,
      );
      const { redirect_to } = await hydraAdmin(
        `/admin/oauth2/auth/requests/consent/accept?consent_challenge=${challenge}`,
        {
          method: "PUT",
          body: JSON.stringify({
            grant_scope: consentRequest.requested_scope,
            grant_access_token_audience: consentRequest.requested_access_token_audience,
            remember: false,
          }),
        },
      );
      redirect(res, redirect_to);
      return;
    }

    res.writeHead(404);
    res.end("not found");
  } catch (err) {
    console.error("oidc-login-stub error:", err);
    res.writeHead(500);
    res.end(String(err));
  }
});

server.listen(PORT, () => {
  console.log(`oidc-login-stub listening on :${PORT}, subject=${TEST_SUBJECT}`);
});
