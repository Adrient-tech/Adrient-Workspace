// Paged, server-side directory search used by the async pickers
// (GET /api/v1/directory/search, keyset pagination, 50 per page at most).

export type DirectoryType = "user" | "group" | "unit";

export interface DirectoryEntry {
  id: string;
  type: DirectoryType;
  name: string;
  email?: string;
  department?: string;
  job_title?: string;
  employee_id?: string;
  upn?: string;
  count?: number;
  topics?: number;
  detail?: string;
  status?: string;
}

export interface Page<T> {
  items: T[];
  next_cursor?: string;
  total_estimate?: number;
  total_capped?: boolean;
}

export const DIRECTORY_PAGE = 30;

export async function searchDirectoryPage(
  type: DirectoryType,
  q: string,
  cursor?: string,
  signal?: AbortSignal,
  opts: { limit?: number; includeBlocked?: boolean } = {},
): Promise<Page<DirectoryEntry>> {
  const p = new URLSearchParams({ type, limit: String(opts.limit ?? DIRECTORY_PAGE) });
  if (q) p.set("q", q);
  if (cursor) p.set("cursor", cursor);
  if (opts.includeBlocked) p.set("status", "any");
  const r = await fetch(`/api/v1/directory/search?${p}`, { credentials: "same-origin", signal });
  if (!r.ok) throw new Error(`Search failed (HTTP ${r.status})`);
  return (await r.json()) as Page<DirectoryEntry>;
}

/** Loads the entries for already-selected ids (chips after a reload), 50 per request, up to 200. */
export async function lookupDirectory(ids: string[]): Promise<DirectoryEntry[]> {
  const chunks: string[][] = [];
  for (let i = 0; i < Math.min(ids.length, 200); i += 50) chunks.push(ids.slice(i, i + 50));
  const parts = await Promise.all(
    chunks.map(async (c) => {
      const r = await fetch(`/api/v1/directory/search?type=user&status=any&ids=${encodeURIComponent(c.join(","))}`, {
        credentials: "same-origin",
      });
      return r.ok ? ((await r.json()) as Page<DirectoryEntry>).items : [];
    }),
  );
  return parts.flat();
}
