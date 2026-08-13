// TPL-1 milestone-3/task-3: a thin, hand-written fetch wrapper over the
// types openapi-typescript generates from bff-openapi.yaml
// (bff-schema.gen.ts, `npm run generate:api`) — types only, not a full
// client generator, per GOAL.md's Decisions table ("Typed client, still no
// tRPC ... openapi-typescript generates types ... a thin fetch wrapper
// underneath"). Used by every /api/bff/* hook (lib/todos.ts, lib/keys.ts)
// except auth-client.ts's own GET /api/bff/me call, which deliberately
// does NOT go through this wrapper — see that file's own comment on why
// (I5: a 401 here is parsed for `message`/`code` so the CRUD screens can
// show something better than a bare status code in a toast; GET /me's own
// "no session" case is different in kind, not just degree, since it's not
// an error to surface at all).
import type { components } from "./bff-schema.gen";

export type ApiErrorBody = components["schemas"]["Error"]["error"];

/** Thrown by bffFetch when the BFF answers a non-2xx/204 status. */
export class BFFRequestError extends Error {
  readonly status: number;
  readonly code?: string;

  constructor(status: number, body: ApiErrorBody | undefined) {
    super(body?.message ?? `Request failed (${status})`);
    this.name = "BFFRequestError";
    this.status = status;
    this.code = body?.code;
  }
}

/**
 * Calls one of the BFF's /api/bff/* endpoints with credentials included —
 * the session cookie is HttpOnly (`_contract/API.md`), so fetch only ever
 * sends it, never reads it. `path` is relative to `/api/bff` (e.g.
 * "/todos", not "/api/bff/todos").
 */
export async function bffFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/api/bff${path}`, {
    ...init,
    credentials: "include",
    headers: {
      ...(init?.body ? { "Content-Type": "application/json" } : {}),
      ...init?.headers,
    },
  });

  if (!res.ok) {
    // bff-openapi.yaml's Error envelope ({error:{code,message,hint}}) —
    // every /api/bff/* endpoint other than GET /me returns this shape on
    // failure, so parsing it here is what lets the CRUD screens surface a
    // real message instead of a bare status code.
    let body: components["schemas"]["Error"] | undefined;
    try {
      body = (await res.json()) as components["schemas"]["Error"];
    } catch {
      // A non-JSON body (e.g. a proxy's own error page reaching the SPA
      // before the Go backend does) — fall through to BFFRequestError's
      // own generic message.
    }
    throw new BFFRequestError(res.status, body?.error);
  }

  if (res.status === 204) {
    return undefined as T;
  }
  return (await res.json()) as T;
}
