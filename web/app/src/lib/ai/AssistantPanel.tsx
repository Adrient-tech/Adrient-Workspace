import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import ReactMarkdown from "react-markdown";
import { loadDriveText, type Attachment } from "./attachments";
import { suggestionsFor } from "./defaults";
import { clearHistory, loadHistory, saveHistory, toWireHistory, type UIMessage } from "./history";
import { getRegisteredTools, subscribeTools } from "./registry";
import { fetchAIStatus, postChat, type AIStatus } from "./stream";
import { runAssistantTurn, type AppliedChange } from "./toolLoop";
import type { AssistantContext, AssistantPanelProps, AssistantTool } from "./types";

const DrivePicker = lazy(() => import("../../pages/drive/DrivePicker").then((m) => ({ default: m.DrivePicker })));

const MAX_CONTEXT = 8000;
/** Errors that mean the assistant cannot be used at all right now. */
const BLOCKING = new Set(["not_configured", "kill_switch", "disabled_for_app", "disabled_for_user"]);
let seq = 0;
const uid = () => `m${Date.now().toString(36)}${seq++}`;

function useRegisteredTools(app: string): AssistantTool[] {
  const snap = useSyncExternalStore(
    subscribeTools,
    () => getRegisteredTools(app).map((t) => t.name).join("|") + ":" + getRegisteredTools(app).length,
  );
  // snap only signals change; read the live list.
  return useMemo(() => getRegisteredTools(app), [app, snap]);
}

const box: React.CSSProperties = {
  display: "flex", flexDirection: "column", height: "100%", minWidth: 0,
  background: "var(--joy-palette-background-surface, #fff)",
  color: "var(--joy-palette-text-primary, #1b1f1e)",
  borderLeft: "1px solid var(--joy-palette-divider, #dfe5e3)", fontSize: 14,
};
const btn: React.CSSProperties = {
  border: "1px solid var(--joy-palette-divider, #dfe5e3)", background: "transparent", color: "inherit",
  borderRadius: 6, padding: "2px 8px", cursor: "pointer", fontSize: 12,
};

export default function AssistantPanel(props: AssistantPanelProps) {
  const { app, documentId, documentTitle, getContext, subscribeContext, systemHint, onClose, onInsertText } = props;
  const registered = useRegisteredTools(app);
  const tools = props.tools ?? registered;
  const [status, setStatus] = useState<AIStatus | null>(null);
  const [msgs, setMsgs] = useState<UIMessage[]>(() => loadHistory(app, documentId));
  const [input, setInput] = useState("");
  const [busy, setBusy] = useState(false);
  const [changes, setChanges] = useState<Record<string, AppliedChange>>({});
  const [confirmReq, setConfirmReq] = useState<{ tool: AssistantTool; args: Record<string, unknown>; resolve: (b: boolean) => void } | null>(null);
  const [ctx, setCtx] = useState<AssistantContext | null>(null);
  const [attachments, setAttachments] = useState<Attachment[]>([]);
  const [picking, setPicking] = useState(false);
  const abortRef = useRef<AbortController | null>(null);
  const endRef = useRef<HTMLDivElement>(null);
  const toolsRef = useRef(tools);
  toolsRef.current = tools;

  useEffect(() => {
    let alive = true;
    void fetchAIStatus(app).then((s) => alive && setStatus(s));
    return () => { alive = false; };
  }, [app]);

  useEffect(() => {
    setMsgs(loadHistory(app, documentId));
    setChanges({});
  }, [app, documentId]);

  useEffect(() => {
    const refresh = () => setCtx(getContext?.() ?? null);
    refresh();
    return subscribeContext?.(refresh);
  }, [getContext, subscribeContext]);

  useEffect(() => {
    if (!busy) saveHistory(app, documentId, msgs);
    endRef.current?.scrollIntoView?.({ block: "end" });
  }, [msgs, busy, app, documentId]);

  const patch = useCallback((id: string, f: (m: UIMessage) => UIMessage) =>
    setMsgs((cur) => cur.map((m) => (m.id === id ? f(m) : m))), []);

  const send = useCallback(async (text: string) => {
    const q = text.trim();
    if (!q || busy) return;
    setInput("");
    const live = getContext?.() ?? null;
    const history = toWireHistory(msgs);
    const files = attachments;
    setAttachments([]);
    const userMsg: UIMessage = { id: uid(), role: "user", text: files.length ? `${q}\n\n📎 ${files.map((f) => f.name).join(", ")}` : q };
    let cur: UIMessage = { id: uid(), role: "assistant", text: "" };
    setMsgs((m) => [...m, userMsg, cur]);
    const ac = new AbortController();
    abortRef.current = ac;
    setBusy(true);
    // Activity rows get their own ids: providers may reuse tool-call ids across turns.
    const rowFor = new Map<string, string>();
    const result = await runAssistantTurn({
      transport: postChat,
      base: {
        app, document_id: documentId, system_hint: systemHint,
        context: live ? { label: live.label, text: live.text?.slice(0, MAX_CONTEXT) } : undefined,
        attachments: files.length ? files : undefined,
      },
      history: [...history, { role: "user", content: q }],
      tools: toolsRef.current,
      signal: ac.signal,
      hooks: {
        onText: (d) => { const id = cur.id; patch(id, (m) => ({ ...m, text: m.text + d })); },
        onSource: (s) => { const id = cur.id; patch(id, (m) => ({ ...m, sources: [...(m.sources ?? []), s] })); },
        onToolStart: (call, tool) => {
          const rowId = uid();
          rowFor.set(call.id, rowId);
          const row: UIMessage = { id: rowId, role: "activity", status: "pending", text: tool ? (tool.label?.(safeJSON(call.arguments)) ?? tool.name) : call.name };
          setMsgs((m) => [...m, row]);
        },
        onToolEnd: (call, st, summary, change) => {
          if (change) setChanges((c) => ({ ...c, [change.id]: change }));
          patch(rowFor.get(call.id) ?? "", (m) => ({ ...m, status: st, text: summary, changeId: change?.id }));
          cur = { id: uid(), role: "assistant", text: "" };
          setMsgs((m) => [...m, cur]);
        },
        confirm: (tool, args) => new Promise<boolean>((resolve) => setConfirmReq({ tool, args, resolve })),
      },
    });
    setMsgs((m) => {
      let out = m.filter((x) => !(x.role === "assistant" && x.text === "" && !x.sources));
      if (result.error) out = [...out, { id: uid(), role: "activity", status: "error", text: result.error.message }];
      if (result.stopped) out = [...out, { id: uid(), role: "activity", status: "declined", text: "Stopped" }];
      return out.map((x) => (x.status === "pending" ? { ...x, status: "error" as const } : x));
    });
    if (result.error && BLOCKING.has(result.error.code)) {
      const code = result.error.code;
      setStatus((s) => ({ ...(s ?? { enabled: false, configured: false }), enabled: false, reason: code }));
    }
    setBusy(false);
    abortRef.current = null;
  }, [app, attachments, busy, documentId, getContext, msgs, patch, systemHint]);

  const attach = async (f: { id: string; name: string; mime_type?: string; size_bytes?: string }) => {
    setPicking(false);
    try {
      const a = await loadDriveText(f);
      setAttachments((cur) => [...cur.filter((x) => x.name !== a.name), a].slice(-5));
    } catch (e) {
      setMsgs((m) => [...m, { id: uid(), role: "activity", status: "error", text: (e as Error).message }]);
    }
  };

  const undo = async (id: string) => {
    const c = changes[id];
    if (!c?.undo || c.undone) return;
    try {
      await c.undo();
      setChanges((x) => ({ ...x, [id]: { ...c, undone: true } }));
    } catch (e) {
      setMsgs((m) => [...m, { id: uid(), role: "activity", status: "error", text: `Undo failed: ${(e as Error).message}` }]);
    }
  };

  const undoAll = async () => {
    const list = Object.values(changes).filter((c) => c.undo && !c.undone).reverse();
    for (const c of list) await undo(c.id);
  };

  const blocked = status && !status.enabled;
  const chips = props.suggestions ?? suggestionsFor(app);
  const pendingChanges = Object.values(changes).filter((c) => c.undo && !c.undone).length;

  return (
    <aside style={box} aria-label="AI Assistant" data-testid="ai-panel">
      <header style={{ display: "flex", alignItems: "center", gap: 8, padding: "10px 12px", borderBottom: "1px solid var(--joy-palette-divider, #dfe5e3)" }}>
        <strong style={{ flex: 1, minWidth: 0, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }} title={documentTitle}>Assistant{documentTitle ? <span style={{ fontWeight: 400, opacity: 0.6 }}> · {documentTitle}</span> : null}</strong>
        {pendingChanges > 0 && <button style={btn} onClick={() => void undoAll()} title="Undo every change made by the assistant in this session">Undo all AI changes</button>}
        <button style={btn} onClick={() => { clearHistory(app, documentId); setMsgs([]); }} disabled={busy} aria-label="New chat">New chat</button>
        {onClose && <button style={btn} onClick={onClose} aria-label="Close assistant">✕</button>}
      </header>

      <div style={{ flex: 1, overflowY: "auto", padding: 12, display: "flex", flexDirection: "column", gap: 10 }} data-testid="ai-messages">
        {!blocked && msgs.length === 0 && (
          <div>
            <p style={{ opacity: 0.75, marginTop: 0 }}>Ask about this {app === "global" ? "workspace" : "document"} or tell me what to change.</p>
            <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
              {chips.map((c) => (
                <button key={c} style={{ ...btn, textAlign: "left", padding: "6px 10px", fontSize: 13 }} onClick={() => void send(c)}>{c}</button>
              ))}
            </div>
            {status?.notice && <p style={{ fontSize: 12, opacity: 0.65 }}>{status.notice}</p>}
          </div>
        )}
        {msgs.map((m) => (
          <Message key={m.id} m={m} change={m.changeId ? changes[m.changeId] : undefined} onUndo={undo} onInsert={onInsertText} />
        ))}
        {confirmReq && (
          <div role="alertdialog" aria-label="Confirm AI action" style={{ border: "1px solid #c77d00", borderRadius: 8, padding: 10, background: "rgba(199,125,0,.08)" }}>
            <div style={{ fontWeight: 600 }}>Confirm: {confirmReq.tool.label?.(confirmReq.args) ?? confirmReq.tool.name}</div>
            <pre style={{ whiteSpace: "pre-wrap", fontSize: 12, margin: "6px 0", maxHeight: 120, overflow: "auto" }}>{JSON.stringify(confirmReq.args, null, 1).slice(0, 600)}</pre>
            <button style={{ ...btn, marginRight: 6 }} onClick={() => { confirmReq.resolve(true); setConfirmReq(null); }}>Allow</button>
            <button style={btn} onClick={() => { confirmReq.resolve(false); setConfirmReq(null); }}>Decline</button>
          </div>
        )}
        <div ref={endRef} />
      </div>

      <footer style={{ borderTop: "1px solid var(--joy-palette-divider, #dfe5e3)", padding: 10 }}>
        {ctx && <div data-testid="ai-context-chip" title="Sent to the assistant as context" style={{ display: "inline-block", fontSize: 12, padding: "1px 8px", borderRadius: 10, background: "var(--joy-palette-primary-softBg, #ecf6f3)", marginBottom: 6 }}>{ctx.label}</div>}
        {blocked && <div style={{ marginBottom: 8 }}><NotConfigured status={status!} /></div>}
        {attachments.length > 0 && (
          <div style={{ display: "flex", flexWrap: "wrap", gap: 4, marginBottom: 6 }} data-testid="ai-attachments">
            {attachments.map((a) => (
              <span key={a.name} style={{ fontSize: 12, padding: "1px 8px", borderRadius: 10, border: "1px solid var(--joy-palette-divider, #dfe5e3)" }}>
                📎 {a.name}{" "}
                <button style={{ ...btn, border: 0, padding: 0 }} aria-label={`Remove ${a.name}`} onClick={() => setAttachments((c) => c.filter((x) => x !== a))}>×</button>
              </span>
            ))}
          </div>
        )}
        <div style={{ display: "flex", gap: 6 }}>
          <button style={btn} onClick={() => setPicking(true)} disabled={!!blocked || busy} aria-label="Attach from Drive" title="Attach a text file from Drive">📎</button>
          <textarea
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); void send(input); } }}
            placeholder={blocked ? "AI is not available" : "Ask the assistant…"}
            disabled={!!blocked}
            rows={2}
            aria-label="Message the assistant"
            style={{ flex: 1, resize: "none", borderRadius: 6, padding: 6, font: "inherit", border: "1px solid var(--joy-palette-divider, #dfe5e3)", background: "transparent", color: "inherit" }}
          />
          {busy ? (
            <button style={btn} onClick={() => abortRef.current?.abort()} aria-label="Stop">Stop</button>
          ) : (
            <button style={{ ...btn, background: "var(--joy-palette-primary-500, #126C5A)", color: "#fff", opacity: !input.trim() || blocked ? 0.5 : 1 }} onClick={() => void send(input)} disabled={!input.trim() || !!blocked} aria-label="Send">Send</button>
          )}
        </div>
        {picking && (
          <Suspense fallback={null}>
            <DrivePicker mode="file" title="Attach from Drive" confirmLabel="Attach" onClose={() => setPicking(false)} onPick={(f) => void attach(f as { id: string; name: string })} />
          </Suspense>
        )}
        <div style={{ fontSize: 11, opacity: 0.55, marginTop: 4 }}>AI can make mistakes. Check important results.{status?.local_only ? " Local model only." : ""}</div>
      </footer>
    </aside>
  );
}

function safeJSON(s: string): Record<string, unknown> {
  try { return JSON.parse(s || "{}") as Record<string, unknown>; } catch { return {}; }
}

function NotConfigured({ status }: { status: AIStatus }) {
  const msg =
    status.reason === "kill_switch" ? "The assistant is switched off for your organisation."
    : status.reason === "disabled_for_app" ? "The assistant is not enabled for this app."
    : status.reason === "disabled_for_user" ? "The assistant is not available for your account type."
    : status.reason === "quota" ? "You have reached your AI usage limit. Try again later."
    : "AI is not configured. Ask your admin to set it up.";
  return (
    <div role="status" style={{ border: "1px dashed var(--joy-palette-divider, #aaa)", borderRadius: 8, padding: 12 }}>
      <div style={{ fontWeight: 600, marginBottom: 4 }}>{msg}</div>
      {status.is_admin && <a href="/admin/ai">Open AI settings</a>}
    </div>
  );
}

function Message({ m, change, onUndo, onInsert }: { m: UIMessage; change?: AppliedChange; onUndo: (id: string) => void; onInsert?: (t: string) => void }) {
  if (m.role === "activity") {
    const icon = m.status === "pending" ? "…" : m.status === "ok" ? "✓" : m.status === "declined" ? "–" : "!";
    return (
      <div data-testid="ai-activity" data-status={m.status} style={{ fontSize: 12, opacity: 0.85, display: "flex", gap: 6, alignItems: "center", color: m.status === "error" ? "#b3261e" : undefined }}>
        <span>{icon}</span><span style={{ flex: 1 }}>{m.text}</span>
        {change?.undo && !change.undone && <button style={btn} onClick={() => onUndo(change.id)}>Undo</button>}
        {change?.undone && <span style={{ opacity: 0.6 }}>undone</span>}
      </div>
    );
  }
  if (m.role === "assistant" && !m.text && !m.sources?.length) return null;
  const mine = m.role === "user";
  return (
    <div style={{ alignSelf: mine ? "flex-end" : "stretch", maxWidth: mine ? "85%" : "100%", background: mine ? "var(--joy-palette-primary-softBg, #ecf6f3)" : "transparent", padding: mine ? "6px 10px" : 0, borderRadius: 10 }}>
      <div className="ai-md" style={{ overflowWrap: "anywhere" }}><ReactMarkdown>{m.text}</ReactMarkdown></div>
      {!mine && m.text && (
        <div style={{ display: "flex", gap: 4, marginTop: 4 }}>
          <button style={btn} onClick={() => void navigator.clipboard?.writeText(m.text)}>Copy</button>
          {onInsert && <button style={btn} onClick={() => onInsert(m.text)}>Insert</button>}
        </div>
      )}
      {m.sources && m.sources.length > 0 && (
        <div style={{ fontSize: 12, marginTop: 6 }}>Sources: {m.sources.map((s, i) => (
          <span key={i}>{i ? ", " : ""}{s.url ? <a href={s.url}>{s.title}</a> : s.title}</span>
        ))}</div>
      )}
    </div>
  );
}
