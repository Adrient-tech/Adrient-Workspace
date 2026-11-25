import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import { Alert, Box, Button, Checkbox, Chip, CircularProgress, FormControl, FormLabel, Input, Option, Select, Sheet, Textarea, Typography } from "@mui/joy";
import { RichText } from "./TicketPane";
import type { FormField, RequestForm } from "./svcApi";

const PUB = "/api/v1/public";
async function pj<T>(path: string, init?: RequestInit): Promise<T> {
  const r = await fetch(path, { headers: { "Content-Type": "application/json" }, ...init });
  if (!r.ok) throw new Error((await r.text().catch(() => "")).trim() || `HTTP ${r.status}`);
  return (await r.json()) as T;
}
const shell = (children: React.ReactNode) => (
  <Box sx={{ minHeight: "100vh", bgcolor: "background.level1", py: { xs: 2, md: 5 }, px: 1.5 }}>
    <Box sx={{ maxWidth: 760, mx: "auto" }}>{children}</Box>
  </Box>
);

/** Requester portal at /portal/:token: request forms, article suggestions while typing, submit. */
export function PortalPage() {
  const { token = "" } = useParams();
  const [forms, setForms] = useState<RequestForm[] | null>(null);
  const [name, setName] = useState<string>("");
  const [form, setForm] = useState<RequestForm | null>(null);
  const [missing, setMissing] = useState(false);
  useEffect(() => {
    pj<{ forms: RequestForm[] }>(`${PUB}/help/${token}/forms`).then((r) => setForms(r.forms)).catch(() => setMissing(true));
    pj<{ name: string }>(`${PUB}/help/${token}`).then((r) => setName(r.name)).catch(() => undefined);
  }, [token]);
  if (missing) return shell(<Typography level="h3" data-testid="portal-missing">This portal is not available</Typography>);
  if (!forms) return shell(<CircularProgress />);
  return shell(
    <>
      <Typography level="h2" sx={{ color: "primary.600" }} data-testid="portal-title">{name || "Support"}</Typography>
      <Typography level="body-sm" sx={{ color: "text.tertiary", mb: 2 }}>Tell us what you need. You can follow your request with the link we give you after you submit. <a href={`/help/${token}`}>Browse help articles</a></Typography>
      {form ? <PortalForm token={token} form={form} onBack={() => setForm(null)} /> : (
        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 1.5 }}>
          {forms.map((f) => (
            <Sheet key={f.id} variant="outlined" onClick={() => setForm(f)} data-testid={`portal-form-${f.name}`} sx={{ p: 2, borderRadius: "lg", cursor: "pointer", "&:hover": { bgcolor: "primary.softBg" } }}>
              <Typography level="title-md">{f.name}</Typography>
              <Typography level="body-sm" sx={{ color: "text.tertiary" }}>{f.description}</Typography>
            </Sheet>
          ))}
          {forms.length === 0 && <Alert color="neutral">No request forms are available yet.</Alert>}
        </Box>
      )}
    </>,
  );
}

function Field({ f, v, on }: { f: FormField; v: string; on: (s: string) => void }) {
  if (f.type === "longtext") return <Textarea minRows={3} value={v} onChange={(e) => on(e.target.value)} />;
  if (f.type === "select") return <Select value={v || null} placeholder="Choose…" onChange={(_, s) => on(s ?? "")}>{(f.options ?? []).map((o) => <Option key={o} value={o}>{o}</Option>)}</Select>;
  if (f.type === "date") return <Input type="date" value={v} onChange={(e) => on(e.target.value)} />;
  if (f.type === "checkbox") return <Checkbox label="Yes" checked={v === "true"} onChange={(e) => on(e.target.checked ? "true" : "false")} />;
  return <Input value={v} onChange={(e) => on(e.target.value)} />;
}

function PortalForm({ token, form, onBack }: { token: string; form: RequestForm; onBack: () => void }) {
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [title, setTitle] = useState("");
  const [vals, setVals] = useState<Record<string, string>>({});
  const [sug, setSug] = useState<{ id: string; slug: string; title: string }[]>([]);
  const [err, setErr] = useState("");
  const [done, setDone] = useState<{ ref: string; track_token: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const text = title + " " + Object.values(vals).join(" ");
  useEffect(() => {
    if (text.trim().length < 6) return setSug([]);
    const t = setTimeout(() => pj<{ articles: typeof sug }>(`${PUB}/help/${token}/suggest?q=${encodeURIComponent(text)}`).then((r) => setSug(r.articles)).catch(() => setSug([])), 400);
    return () => clearTimeout(t);
  }, [text, token]);
  if (done) {
    const link = `${window.location.origin}/support/${token}/track/${done.track_token}`;
    return (
      <Sheet variant="outlined" sx={{ p: 3, borderRadius: "lg" }} data-testid="portal-done">
        <Alert color="success" sx={{ mb: 1.5 }}>Request {done.ref} received.</Alert>
        <Typography level="body-sm">Keep this link to follow your request, reply and close it:</Typography>
        <Typography level="body-sm" sx={{ wordBreak: "break-all", fontFamily: "monospace", my: 1 }}><a href={link} data-testid="track-link">{link}</a></Typography>
      </Sheet>
    );
  }
  async function submit() {
    setBusy(true);
    setErr("");
    try { setDone(await pj(`${PUB}/help/${token}/forms/${form.id}/submit`, { method: "POST", body: JSON.stringify({ name, email, title, values: vals }) })); } catch (e) { setErr((e as Error).message); } finally { setBusy(false); }
  }
  return (
    <Sheet variant="outlined" sx={{ p: { xs: 2, sm: 3 }, borderRadius: "lg", display: "flex", flexDirection: "column", gap: 1.5 }} data-testid="portal-form">
      <Button size="sm" variant="plain" color="neutral" onClick={onBack} sx={{ alignSelf: "flex-start" }}>← All forms</Button>
      <Typography level="title-lg">{form.name}</Typography>
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 1.5 }}>
        <FormControl size="sm"><FormLabel>Your name</FormLabel><Input value={name} onChange={(e) => setName(e.target.value)} /></FormControl>
        <FormControl size="sm"><FormLabel>Your email</FormLabel><Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} /></FormControl>
      </Box>
      <FormControl size="sm"><FormLabel>Subject (optional)</FormLabel><Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder={form.name} /></FormControl>
      {form.fields.map((f) => (
        <FormControl key={f.key} size="sm" required={f.required}><FormLabel>{f.label}</FormLabel><Field f={f} v={vals[f.key] ?? ""} on={(s) => setVals({ ...vals, [f.key]: s })} /></FormControl>
      ))}
      {sug.length > 0 && (
        <Sheet variant="soft" color="primary" sx={{ p: 1.25, borderRadius: "md" }} data-testid="portal-suggest">
          <Typography level="body-xs" sx={{ fontWeight: 600 }}>These articles may answer your question</Typography>
          {sug.map((a) => <Typography key={a.id} level="body-sm"><a href={`/help/${token}/${a.slug}`} target="_blank" rel="noreferrer">{a.title}</a></Typography>)}
        </Sheet>
      )}
      {err && <Alert color="danger" size="sm">{err}</Alert>}
      <Button loading={busy} onClick={submit} data-testid="portal-submit" sx={{ alignSelf: "flex-start" }}>Submit request</Button>
    </Sheet>
  );
}

interface Track { ref: string; title: string; status: string; solved: boolean; created_at: string; comments: { author: string; agent: boolean; body: string; created_at: string }[]; csat_token?: string }

/** Requester view of one ticket at /portal/:token/track/:trk. */
export function TrackPage() {
  const { trk = "" } = useParams();
  const [t, setT] = useState<Track | null>(null);
  const [missing, setMissing] = useState(false);
  const [msg, setMsg] = useState("");
  const [err, setErr] = useState("");
  const load = () => pj<Track>(`${PUB}/track/${trk}`).then(setT).catch(() => setMissing(true));
  useEffect(() => { void load(); }, [trk]); // eslint-disable-line react-hooks/exhaustive-deps
  if (missing) return shell(<Typography level="h3">This link is not valid</Typography>);
  if (!t) return shell(<CircularProgress />);
  return shell(
    <Sheet variant="outlined" sx={{ p: { xs: 2, sm: 3 }, borderRadius: "lg" }} data-testid="track-page">
      <Box sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap" }}>
        <Typography level="body-sm" sx={{ fontFamily: "monospace" }}>{t.ref}</Typography>
        <Chip size="sm" variant="soft" color={t.solved ? "success" : "primary"} data-testid="track-status">{t.status.replace(/_/g, " ")}</Chip>
      </Box>
      <Typography level="h3" sx={{ mb: 2 }}>{t.title}</Typography>
      {t.comments.length === 0 && <Typography level="body-sm" sx={{ color: "text.tertiary", mb: 2 }}>No replies yet. We will answer here.</Typography>}
      {t.comments.map((c, i) => (
        <Sheet key={i} variant="soft" color={c.agent ? "primary" : "neutral"} sx={{ p: 1.5, mb: 1, borderRadius: "md" }}>
          <Typography level="title-sm">{c.author}{c.agent ? " (support)" : ""}</Typography>
          <RichText text={c.body} />
        </Sheet>
      ))}
      {true ? (
        <Box sx={{ mt: 2 }}>
          <Textarea minRows={3} placeholder={t.solved ? "Write to reopen this request…" : "Write a reply…"} value={msg} onChange={(e) => setMsg(e.target.value)} slotProps={{ textarea: { "data-testid": "track-msg" } as never }} />
          <Box sx={{ display: "flex", gap: 1, mt: 1 }}>
            <Button disabled={!msg.trim()} onClick={async () => { setErr(""); try { await pj(`${PUB}/track/${trk}/reply`, { method: "POST", body: JSON.stringify({ body: msg }) }); setMsg(""); await load(); } catch (e) { setErr((e as Error).message); } }} data-testid="track-reply">Send reply</Button>
            {!t.solved && <Button variant="outlined" color="neutral" onClick={async () => { if (window.confirm("Close this request?")) { await pj(`${PUB}/track/${trk}/close`, { method: "POST", body: "{}" }).catch((e) => setErr((e as Error).message)); await load(); } }} data-testid="track-close">Close request</Button>}
          </Box>
          {err && <Alert color="danger" size="sm" sx={{ mt: 1 }}>{err}</Alert>}
        </Box>
      ) : null}
      {t.csat_token && <Alert color="neutral" sx={{ mt: 2 }}>How did we do? <a href={`/csat/${t.csat_token}`}>Rate this support</a></Alert>}
    </Sheet>,
  );
}
