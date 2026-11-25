import { useCallback, useEffect, useMemo, useState } from "react";
import { Box, Button, Card, Chip, CircularProgress, Container, DialogTitle, Modal, ModalClose, ModalDialog, Option, Select, Typography } from "@mui/joy";
import DrawOutlined from "@mui/icons-material/DrawOutlined";
import { Header } from "../../components/Header";
import { BizAssistant } from "../../components/BizAssistant";
import type { User } from "../../api/types";
import { deepParam } from "../../lib/deepLink";
import { fmtDate } from "../know/http";
import { signatures, type Request, type TrailEvent } from "./api";
import { NewRequest } from "./NewRequest";
import { STATUS_COLOR, STATUS_LABEL, waitingOn } from "./logic";
import { createSignaturesTools, SIGNATURES_HINT, SIGNATURES_SUGGESTIONS } from "./ai/tools";

const ACCENT = "#126C5A";

function Detail({ id, me, onClose, onChanged }: { id: string; me: string; onClose: () => void; onChanged: () => void }) {
  const [r, setR] = useState<Request | null>(null);
  const [trail, setTrail] = useState<{ events: TrailEvent[]; intact: boolean } | null>(null);
  const [links, setLinks] = useState<Record<string, string>>({});
  const [err, setErr] = useState("");
  const load = useCallback(() => { signatures.get(id).then(setR).catch((e: Error) => setErr(e.message)); signatures.trail(id).then(setTrail).catch(() => undefined); }, [id]);
  useEffect(load, [load]);
  if (!r) return <Modal open onClose={onClose}><ModalDialog>{err ? <Typography color="danger">{err}</Typography> : <CircularProgress />}</ModalDialog></Modal>;
  const act = async (fn: () => Promise<unknown>, msg?: string) => { try { await fn(); if (msg) window.alert(msg); load(); onChanged(); } catch (e) { setErr((e as Error).message); } };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 760, maxWidth: "100vw", maxHeight: "96vh", overflow: "auto", "@media (max-width: 600px)": { width: "100vw", height: "100vh", maxHeight: "100vh", borderRadius: 0 } }} role="dialog" aria-label={`Request ${r.title}`}>
        <ModalClose />
        <DialogTitle sx={{ pr: 5 }}>{r.title}</DialogTitle>
        <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", alignItems: "center", mb: 1 }}>
          <Chip color={STATUS_COLOR[r.status]} variant="soft">{STATUS_LABEL[r.status]}</Chip>
          <Typography level="body-sm">{r.progress}{waitingOn(r) ? ` · ${waitingOn(r)}` : ""}</Typography>
          {r.expires_at && r.status === "sent" && <Typography level="body-xs">Expires {fmtDate(r.expires_at)}</Typography>}
        </Box>
        {err && <Typography color="danger" role="alert" level="body-sm">{err}</Typography>}
        <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", mb: 2 }}>
          {r.can_manage && r.status === "draft" && <Button onClick={() => void act(async () => { const s = await signatures.send(id); const m: Record<string, string> = {}; s.signers?.forEach((x) => { if (x.link) m[x.id] = x.link; }); setLinks(m); })}>Send for signature</Button>}
          {r.can_manage && r.status === "sent" && <Button variant="soft" onClick={() => void act(async () => { const n = await signatures.remind(id); window.alert(`Reminder sent to ${n.sent} ${n.sent === 1 ? "person" : "people"}.`); })}>Send reminder</Button>}
          {r.can_manage && r.status === "sent" && <Button variant="soft" color="danger" onClick={() => { const why = window.prompt("Why are you cancelling this request? (optional)"); if (why !== null) void act(() => signatures.void(id, why)); }}>Void</Button>}
          {r.status === "completed" && <Button component="a" href={signatures.pdfUrl(id)} target="_blank" rel="noreferrer">Open signed PDF</Button>}
          <Button variant="outlined" color="neutral" component="a" href={signatures.pdfUrl(id, true)} target="_blank" rel="noreferrer">Original PDF</Button>
          {r.can_manage && ["draft", "voided", "declined", "expired"].includes(r.status) && <Button variant="plain" color="danger" onClick={() => { if (window.confirm("Delete this request?")) void act(() => signatures.remove(id)).then(onClose); }}>Delete</Button>}
        </Box>
        {Object.keys(links).length > 0 && (
          <Box sx={{ p: 1.5, mb: 2, bgcolor: "background.level1", borderRadius: "md" }} data-testid="signing-links">
            <Typography level="title-sm">Signing links</Typography>
            <Typography level="body-xs" sx={{ mb: 0.5 }}>Each link is private to one person and is shown only now. {"Email is sent too when mail is configured."}</Typography>
            {(r.signers ?? []).filter((s) => links[s.id]).map((s) => (
              <Box key={s.id} sx={{ display: "flex", gap: 1, alignItems: "center", mb: 0.5, flexWrap: "wrap" }}>
                <Typography level="body-sm" sx={{ minWidth: 100 }}>{s.name}</Typography>
                <Box component="code" sx={{ fontSize: 11, wordBreak: "break-all", flex: 1 }}>{links[s.id]}</Box>
                <Button size="sm" variant="soft" onClick={() => void navigator.clipboard?.writeText(links[s.id])}>Copy</Button>
              </Box>
            ))}
          </Box>
        )}
        <Typography level="title-md" sx={{ mb: 0.5 }}>People</Typography>
        {(r.signers ?? []).map((s) => (
          <Box key={s.id} sx={{ display: "flex", gap: 1.5, py: 0.7, borderBottom: "1px solid", borderColor: "divider", alignItems: "center", flexWrap: "wrap" }}>
            <Box sx={{ flex: 1, minWidth: 160 }}><Typography level="title-sm">{s.name}{s.user_id === me ? " (you)" : ""}</Typography><Typography level="body-xs">{s.email}{s.role === "cc" ? " · copy only" : ""}</Typography></Box>
            <Chip size="sm" variant="soft" color={s.status === "signed" ? "success" : s.status === "declined" ? "danger" : "neutral"}>{s.status}</Chip>
            <Typography level="body-xs" sx={{ minWidth: 150 }}>{s.signed_at ? `Signed ${new Date(s.signed_at).toLocaleString()}` : s.declined_at ? `Declined: ${s.decline_reason || "no reason"}` : s.viewed_at ? `Opened ${new Date(s.viewed_at).toLocaleString()}` : ""}</Typography>
          </Box>
        ))}
        <Typography level="title-md" sx={{ mt: 2, mb: 0.5 }}>Audit trail {trail && <Chip size="sm" variant="soft" color={trail.intact ? "success" : "danger"}>{trail.intact ? "chain intact" : "chain broken"}</Chip>}</Typography>
        {trail?.events.map((e) => (
          <Box key={e.id} sx={{ display: "flex", gap: 1, fontSize: 12, py: 0.2, flexWrap: "wrap" }}>
            <Box sx={{ width: 150 }}>{new Date(e.at).toLocaleString()}</Box><Box sx={{ width: 80, fontWeight: 600 }}>{e.event}</Box><Box sx={{ flex: 1, minWidth: 120 }}>{e.actor.length > 30 ? e.actor.slice(0, 8) + "…" : e.actor} {e.ip ? `· ${e.ip}` : ""} {e.detail ? `· ${e.detail}` : ""}</Box>
          </Box>
        ))}
        {r.signed_hash && <Typography level="body-xs" sx={{ mt: 1, wordBreak: "break-all" }}>Signed file SHA-256: {r.signed_hash}</Typography>}
      </ModalDialog>
    </Modal>
  );
}

export default function SignaturesApp({ user }: { user: User }) {
  const [list, setList] = useState<Request[] | null>(null);
  const [status, setStatus] = useState("");
  const [open, setOpen] = useState(deepParam("request"));
  const [creating, setCreating] = useState(false);
  const [err, setErr] = useState("");
  const [me, setMe] = useState("");
  const tools = useMemo(() => createSignaturesTools(), []);
  const load = useCallback(() => signatures.list(status).then(setList).catch((e: Error) => setErr(e.message)), [status]);
  useEffect(() => { void load(); }, [load]);
  useEffect(() => { signatures.people().then((r) => setMe(r.me)).catch(() => undefined); }, []);
  const toSign = (list ?? []).filter((r) => r.status === "sent" && r.signers?.some((s) => s.user_id === me && s.role === "signer" && s.status !== "signed"));
  return (
    <>
      <Header user={user} />
      <Container maxWidth="xl" sx={{ py: { xs: 2, sm: 3 }, px: { xs: 1.5, sm: 3 } }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, mb: 2, flexWrap: "wrap" }}>
          <DrawOutlined sx={{ fontSize: 28, color: ACCENT }} />
          <Typography level="h2" sx={{ fontSize: { xs: "xl", sm: "xl3" } }}>Signatures</Typography>
          <Box sx={{ ml: "auto", display: "flex", gap: 1 }}>
            <Button size="sm" onClick={() => setCreating(true)} data-testid="new-request">＋ New request</Button>
            <BizAssistant app="signatures" title="Signatures" tools={tools} suggestions={SIGNATURES_SUGGESTIONS} systemHint={SIGNATURES_HINT} />
          </Box>
        </Box>
        {err && <Typography color="danger" role="alert">{err}</Typography>}
        {toSign.length > 0 && <Box sx={{ p: 1.5, mb: 2, bgcolor: "warning.softBg", borderRadius: "md" }}>{toSign.length} document{toSign.length > 1 ? "s are" : " is"} waiting for your signature: {toSign.map((r) => r.title).join(", ")}. Open it and use the link sent to your email.</Box>}
        <Select value={status} onChange={(_, v) => setStatus(v ?? "")} sx={{ width: 200, mb: 1.5 }} slotProps={{ listbox: { sx: { zIndex: 2000 } } }} aria-label="Status filter">
          <Option value="">All requests</Option>{Object.entries(STATUS_LABEL).map(([k, l]) => <Option key={k} value={k}>{l}</Option>)}
        </Select>
        {!list && <CircularProgress />}
        {list && list.length === 0 && <Typography sx={{ p: 3, textAlign: "center" }}>No requests yet. Send a PDF from Drive for signature.</Typography>}
        {list && list.length > 0 && (
          <Card variant="outlined" sx={{ p: 0 }}>
            {list.map((r) => (
              <Box key={r.id} onClick={() => setOpen(r.id)} role="button" tabIndex={0} onKeyDown={(e) => e.key === "Enter" && setOpen(r.id)} data-testid="request-row" sx={{ display: "flex", gap: 1.5, px: 1.5, py: 1, borderBottom: "1px solid", borderColor: "divider", cursor: "pointer", alignItems: "center", flexWrap: "wrap", "&:hover": { bgcolor: "background.level1" } }}>
                <Box sx={{ flex: 1, minWidth: 180 }}><Typography level="title-sm">{r.title}</Typography><Typography level="body-xs">{(r.signers ?? []).filter((s) => s.role === "signer").map((s) => s.name).join(", ")} · {fmtDate(r.created_at)}</Typography></Box>
                <Typography level="body-xs" sx={{ minWidth: 160 }}>{waitingOn(r) || r.progress}</Typography>
                <Chip size="sm" color={STATUS_COLOR[r.status]} variant="soft">{STATUS_LABEL[r.status]}</Chip>
              </Box>
            ))}
          </Card>
        )}
      </Container>
      {creating && <NewRequest onClose={() => setCreating(false)} onCreated={(id) => { setCreating(false); void load(); setOpen(id); }} />}
      {open && <Detail id={open} me={me} onClose={() => { setOpen(""); window.history.replaceState(null, "", "/signatures"); }} onChanged={() => void load()} />}
    </>
  );
}
