/** Files a ticket from a Mail or Chat message (same pattern as makeTask). */
export interface MadeTicket {
  id: string;
  ref: string;
  href: string;
}

export async function makeTicket(input: { source: "mail" | "chat"; message_id: string; project_id?: string }): Promise<MadeTicket> {
  const r = await fetch("/api/v1/ts/tickets/from-source", {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(input),
  });
  if (!r.ok) throw new Error((await r.text().catch(() => "")).trim() || `HTTP ${r.status}`);
  return (await r.json()) as MadeTicket;
}
