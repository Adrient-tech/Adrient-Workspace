import { useEffect, useState } from "react";
import { Alert, Box, Button, Divider, FormControl, FormLabel, Modal, ModalClose, ModalDialog, Option, Select, Switch, Textarea, Typography } from "@mui/joy";
import { THEMES, emptyExtras, embedSnippet, isEmail, splitEmails, themeVars, type FormExtras } from "./fxLogic";

async function call<T>(path: string, init?: RequestInit): Promise<T> {
  const r = await fetch(`/api/v1/${path}`, { credentials: "same-origin", headers: { Accept: "application/json", "Content-Type": "application/json" }, ...init });
  if (!r.ok) throw new Error((await r.text().catch(() => "")).trim() || `HTTP ${r.status}`);
  return (await r.json()) as T;
}
export const getExtras = (id: string) => call<FormExtras>(`fx/${id}/extras`);
export const saveExtras = (id: string, e: FormExtras) => call<FormExtras>(`fx/${id}/extras`, { method: "PUT", body: JSON.stringify(e) });
export const sendForm = (id: string, body: { emails: string[]; group_id: string; message: string }) => call<{ sent: number }>(`fx/${id}/send`, { method: "POST", body: JSON.stringify(body) });
const groups = () => call<{ groups?: { id: string; name: string }[] }>("contacts/groups").then((r) => r.groups ?? []);

/** Applies the form's colours to the page while it is shown. */
export function useFormTheme(formId: string) {
  useEffect(() => {
    let vars: string[] = [];
    let alive = true;
    getExtras(formId)
      .then((e) => {
        if (!alive) return;
        const v = themeVars(e);
        vars = Object.keys(v);
        for (const [k, val] of Object.entries(v)) document.documentElement.style.setProperty(k, val);
      })
      .catch(() => undefined);
    return () => {
      alive = false;
      for (const k of vars) document.documentElement.style.removeProperty(k);
    };
  }, [formId]);
}

/** Appearance and notification settings for the editor's Settings tab. */
export function ExtrasSettings({ formId }: { formId: string }) {
  const [x, setX] = useState<FormExtras>(emptyExtras());
  const [loaded, setLoaded] = useState(false);
  const [err, setErr] = useState("");
  useEffect(() => {
    getExtras(formId).then((e) => { setX(e); setLoaded(true); }).catch((e: Error) => setErr(e.message));
  }, [formId]);
  async function update(next: FormExtras) {
    setX(next);
    setErr("");
    try {
      await saveExtras(formId, next);
    } catch (e) {
      setErr((e as Error).message);
    }
  }
  if (!loaded && !err) return null;
  return (
    <Box data-testid="fx-settings">
      <Divider sx={{ my: 2 }} />
      <Typography level="title-sm" sx={{ mb: 1, opacity: 0.8 }}>Theme</Typography>
      <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>
        {THEMES.map((t) => (
          <Button key={t.name} size="sm" variant="outlined" color="neutral" data-testid={`theme-${t.name.split(" ")[0].toLowerCase()}`}
            onClick={() => update({ ...x, theme_color: t.color, theme_bg: t.bg })}
            startDecorator={<span style={{ width: 14, height: 14, borderRadius: 7, background: t.color, display: "inline-block" }} />}
            sx={{ outline: x.theme_color === t.color ? "2px solid" : "none", outlineColor: "primary.500" }}>
            {t.name}
          </Button>
        ))}
        <Button size="sm" variant="plain" color="neutral" onClick={() => update({ ...x, theme_color: "", theme_bg: "" })}>Default</Button>
      </Box>
      <Typography level="title-sm" sx={{ mt: 2, mb: 1, opacity: 0.8 }}>Notifications</Typography>
      <Box sx={{ display: "flex", alignItems: "center", py: 0.5 }}>
        <Box sx={{ flex: 1 }}>
          <Typography level="body-md">Notify me of new responses</Typography>
          <Typography level="body-xs" sx={{ opacity: 0.6 }}>You get a notification in Adrient, at most once a minute.</Typography>
        </Box>
        <Switch checked={x.notify_owner} onChange={(e) => update({ ...x, notify_owner: e.target.checked })} slotProps={{ input: { "aria-label": "Notify me of new responses" } }} />
      </Box>
      {err && <Alert color="danger" sx={{ mt: 1 }}>{err}</Alert>}
    </Box>
  );
}

export function EmbedDialog({ formId, onClose }: { formId: string; onClose: () => void }) {
  const code = embedSnippet(window.location.origin, formId);
  const [copied, setCopied] = useState(false);
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: "min(620px, 96vw)" }} data-testid="embed-dialog">
        <ModalClose />
        <Typography level="title-lg">Embed form</Typography>
        <Typography level="body-sm">Paste this HTML into your page. People still sign in to Adrient to respond.</Typography>
        <Textarea readOnly minRows={4} value={code} slotProps={{ textarea: { "data-testid": "embed-code" } }} sx={{ fontFamily: "monospace", fontSize: 12 }} />
        <Button onClick={async () => { try { await navigator.clipboard.writeText(code); setCopied(true); } catch { window.prompt("Copy the HTML", code); } }}>{copied ? "Copied" : "Copy HTML"}</Button>
      </ModalDialog>
    </Modal>
  );
}

export function SendDialog({ formId, onClose }: { formId: string; onClose: () => void }) {
  const [emails, setEmails] = useState("");
  const [groupId, setGroupId] = useState("");
  const [list, setList] = useState<{ id: string; name: string }[]>([]);
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [done, setDone] = useState<number | null>(null);
  useEffect(() => { groups().then(setList).catch(() => setList([])); }, []);
  async function send() {
    const addrs = splitEmails(emails);
    const bad = addrs.find((a) => !isEmail(a));
    if (bad) return setErr(`"${bad}" is not a valid email address.`);
    if (!addrs.length && !groupId) return setErr("Add an email address or choose a contacts group.");
    setBusy(true);
    setErr("");
    try {
      const r = await sendForm(formId, { emails: addrs, group_id: groupId, message });
      setDone(r.sent);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: "min(560px, 96vw)" }} data-testid="send-dialog">
        <ModalClose />
        <Typography level="title-lg">Send form</Typography>
        {done !== null ? (
          <Alert color="success" data-testid="send-done">Sent to {done} {done === 1 ? "person" : "people"}.</Alert>
        ) : (
          <>
            <FormControl>
              <FormLabel>To</FormLabel>
              <Textarea minRows={2} placeholder="name@example.com, another@example.com" value={emails} onChange={(e) => setEmails(e.target.value)} slotProps={{ textarea: { "data-testid": "send-emails" } }} />
            </FormControl>
            <FormControl>
              <FormLabel>Or a Contacts group</FormLabel>
              <Select value={groupId || null} placeholder="No group" onChange={(_, v) => setGroupId(v ?? "")}>
                <Option value="">No group</Option>
                {list.map((g) => <Option key={g.id} value={g.id}>{g.name}</Option>)}
              </Select>
            </FormControl>
            <FormControl>
              <FormLabel>Message (optional)</FormLabel>
              <Textarea minRows={2} value={message} onChange={(e) => setMessage(e.target.value)} />
            </FormControl>
            {err && <Alert color="danger" data-testid="send-error">{err}</Alert>}
            <Button loading={busy} onClick={send} data-testid="send-submit">Send</Button>
          </>
        )}
      </ModalDialog>
    </Modal>
  );
}
