/** Creates a task in the user's Inbox list from another app (Mail, Chat). */
export interface MadeTask {
  id: string;
  list_id: string;
  title: string;
}

export async function makeTask(input: { title: string; notes?: string; link?: string }): Promise<MadeTask> {
  const r = await fetch("/api/v1/tx/quick", {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(input),
  });
  if (!r.ok) throw new Error((await r.text().catch(() => "")).trim() || `HTTP ${r.status}`);
  return (await r.json()) as MadeTask;
}

/** First line of a message, trimmed to a task title. */
export function titleFrom(text: string, fallback = "Follow up"): string {
  const first = text.replace(/<[^>]*>/g, " ").split("\n").map((l) => l.trim()).find(Boolean) ?? "";
  const t = first.replace(/\s+/g, " ");
  if (!t) return fallback;
  return t.length > 120 ? t.slice(0, 117) + "..." : t;
}
