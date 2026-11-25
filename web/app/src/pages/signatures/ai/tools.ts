/** Signatures assistant tools: read-only on purpose. It never signs, sends or voids; those need the user's own click. */
import type { AssistantTool } from "@/lib/ai/types";
import { bizTool, capList, obj, str } from "@/lib/ai/biz";
import { signatures } from "../api";
import { waitingOn } from "../logic";

export const SIGNATURES_SUGGESTIONS = ["Which documents are waiting for signatures?", "Who still has to sign the NDA?", "Show the audit trail of my last request"];
export const SIGNATURES_HINT = "You can look things up but you cannot sign, send, remind or void; tell the user to do that in the app.";

export function createSignaturesTools(): AssistantTool[] {
  const A = "signatures";
  return [
    bizTool(A, {
      name: "signatures_list",
      description: "List signature requests with status and who has not signed yet. Optional status filter: draft, sent, completed, declined, voided, expired.",
      parameters: obj({ status: str("Status filter") }),
      run: async (a) => ({ result: capList((await signatures.list(String(a.status ?? ""))).map((r) => ({ id: r.id, title: r.title, status: r.status, progress: r.progress, waiting: waitingOn(r) || undefined, created: r.created_at.slice(0, 10), signers: (r.signers ?? []).map((s) => `${s.name} (${s.status})`) })), 25) }),
    }),
    bizTool(A, {
      name: "signatures_trail",
      description: "The audit trail of one request (events, who, when) and whether its hash chain is intact.",
      parameters: obj({ request_id: str("Request id") }, ["request_id"]),
      sensitive: true,
      run: async (a) => { const t = await signatures.trail(String(a.request_id)); return { result: { intact: t.intact, events: t.events.slice(-30).map((e) => ({ at: e.at, event: e.event, actor: e.actor })) } }; },
    }),
  ];
}
