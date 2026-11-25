/**
 * "Ask Adrient": a question from the global search bar, answered from the
 * user's own Drive, Docs, Sheets, Slides, Mail, Calendar, Contacts and Keep
 * via the permission-checked global search. Sources are always listed.
 */
import { useCallback, useEffect, useRef, useState } from "react";
import ReactMarkdown from "react-markdown";
import { Alert, Box, Button, Chip, CircularProgress, Input, Modal, ModalClose, ModalDialog, Typography } from "@mui/joy";
import AutoAwesomeIcon from "@mui/icons-material/AutoAwesome";
import { Link as RouterLink } from "react-router-dom";
import { fetchAIStatus, postAsk, type AIStatus } from "./stream";

interface Turn {
  q: string;
  a: string;
  sources: { title: string; url?: string; app?: string }[];
  error?: string;
  done: boolean;
}

const APP_LABEL: Record<string, string> = { drive: "Drive", docs: "Docs", sheets: "Sheets", slides: "Slides", contacts: "Contacts", keep: "Keep", calendar: "Calendar", mail: "Mail" };

export default function AskAdrient({ question, onClose }: { question: string; onClose: () => void }) {
  const [status, setStatus] = useState<AIStatus | null>(null);
  const [turns, setTurns] = useState<Turn[]>([]);
  const [input, setInput] = useState("");
  const abort = useRef<AbortController | null>(null);
  const busy = turns.some((t) => !t.done);

  const ask = useCallback(async (q: string, history: Turn[]) => {
    const ac = new AbortController();
    abort.current = ac;
    const idx = history.length;
    setTurns([...history, { q, a: "", sources: [], done: false }]);
    const upd = (f: (t: Turn) => Turn) => setTurns((cur) => cur.map((t, i) => (i === idx ? f(t) : t)));
    try {
      await postAsk(
        { question: q, history: history.flatMap((t) => [{ role: "user" as const, content: t.q }, { role: "assistant" as const, content: t.a }]) },
        ac.signal,
        (e) => {
          if (e.type === "delta") upd((t) => ({ ...t, a: t.a + e.text }));
          else if (e.type === "source") upd((t) => ({ ...t, sources: [...t.sources, e] }));
          else if (e.type === "error") upd((t) => ({ ...t, error: e.message }));
        },
      );
    } catch (e) {
      if (!ac.signal.aborted) upd((t) => ({ ...t, error: (e as Error).message }));
    }
    upd((t) => ({ ...t, done: true }));
  }, []);

  useEffect(() => {
    let alive = true;
    void fetchAIStatus("global").then((s) => {
      if (!alive) return;
      setStatus(s);
      if (s.enabled) void ask(question, []);
    });
    return () => {
      alive = false;
      abort.current?.abort();
    };
  }, [question, ask]);

  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 720, maxWidth: "96vw", maxHeight: "86vh", overflow: "auto" }} aria-label="Ask Adrient" data-testid="ask-adrient">
        <ModalClose />
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <AutoAwesomeIcon color="primary" />
          <Typography level="title-lg">Ask Adrient</Typography>
        </Box>
        {!status && <CircularProgress size="sm" />}
        {status && !status.enabled && (
          <Alert color="neutral" variant="soft">
            <Box>
              <Typography level="title-sm">
                {status.reason === "kill_switch" ? "The assistant is switched off for your organisation." : status.reason === "disabled_for_app" ? "Ask Adrient is not enabled." : status.reason === "disabled_for_user" ? "The assistant is not available for your account type." : "AI is not configured. Ask your admin to set it up."}
              </Typography>
              {status.is_admin && <RouterLink to="/admin/ai" onClick={onClose}>Open AI settings</RouterLink>}
            </Box>
          </Alert>
        )}
        {turns.map((t, i) => (
          <Box key={i} sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
            <Typography level="title-sm" sx={{ bgcolor: "primary.softBg", px: 1.5, py: 0.75, borderRadius: "md", alignSelf: "flex-end" }}>{t.q}</Typography>
            {t.sources.length > 0 && (
              <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }} aria-label="Sources">
                {t.sources.map((s, j) => (
                  <Chip key={j} size="sm" variant="outlined" component={s.url ? RouterLink : "span"} {...(s.url ? { to: s.url, onClick: onClose } : {})} sx={{ maxWidth: 260 }}>
                    [{j + 1}] {APP_LABEL[s.app ?? ""] ?? s.app}: {s.title || "(Untitled)"}
                  </Chip>
                ))}
              </Box>
            )}
            {t.done && t.sources.length === 0 && !t.error && <Typography level="body-xs" sx={{ opacity: 0.7 }}>No matching items were found in your data.</Typography>}
            <Box sx={{ "& p": { my: 0.5 } }}><ReactMarkdown>{t.a}</ReactMarkdown></Box>
            {!t.done && !t.a && <CircularProgress size="sm" />}
            {t.error && <Alert color="danger" size="sm">{t.error}</Alert>}
          </Box>
        ))}
        {status?.enabled && (
          <Box sx={{ display: "flex", gap: 1 }}>
            <Input sx={{ flex: 1 }} size="sm" placeholder="Ask a follow-up…" value={input} onChange={(e) => setInput(e.target.value)} disabled={busy}
              onKeyDown={(e) => { if (e.key === "Enter" && input.trim()) { void ask(input.trim(), turns); setInput(""); } }} />
            {busy ? <Button size="sm" variant="outlined" onClick={() => abort.current?.abort()}>Stop</Button> : <Button size="sm" disabled={!input.trim()} onClick={() => { void ask(input.trim(), turns); setInput(""); }}>Ask</Button>}
          </Box>
        )}
        <Typography level="body-xs" sx={{ opacity: 0.6 }}>Answers use only items you can open. AI can make mistakes; check the sources.</Typography>
      </ModalDialog>
    </Modal>
  );
}
