// Directory user search + per-object ACL grant helpers, shared by Drive and
// Docs' ShareDialog. The directory endpoint returns the caller's org roster
// (plus live Zitadel matches); grants are keyed by the returned adrient user id.

const API_BASE = "/api/v1";

/** Member is one directory search result (a adrient user). */
export interface Member {
  id: string;
  name: string;
  email: string;
}

/** ObjectGrant is a per-user ACL grant on a file/doc, with the grantee resolved. */
export interface ObjectGrant {
  grantee_user_id: string;
  grantee_name: string;
  grantee_email: string;
  role: string;
  granted_by: string;
}

/**
 * searchDirectory returns the first page (up to 50) of users matching q, from
 * the paged, indexed directory search. Callers that need more than one page
 * use the PeoplePicker component instead.
 */
export async function searchDirectory(q: string): Promise<Member[]> {
  const p = new URLSearchParams({ type: "user", limit: "50" });
  if (q) p.set("q", q);
  const resp = await fetch(`${API_BASE}/directory/search?${p}`, {
    credentials: "same-origin",
    headers: { Accept: "application/json" },
  });
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
  const data = (await resp.json()) as { items?: Member[] };
  return (data.items ?? []).map((m) => ({ id: m.id, name: m.name, email: m.email }));
}
