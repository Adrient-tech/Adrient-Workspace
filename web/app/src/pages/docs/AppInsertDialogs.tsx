// Insert ▸ From app…, Contact…, Meeting link…: pull data from other Adrient
// modules into the document at the cursor.
import { useEffect, useMemo, useState } from "react";
import { Alert, Box, Button, Input, List, ListItemButton, Modal, ModalClose, ModalDialog, Stack, Typography } from "@mui/joy";
import type { Editor } from "@tiptap/react";
import { AppIcon } from "../../components/AppIcon";
import { APP_SOURCES, fetchAppTable, type AppSource, type AppTable } from "../sheets/appData";
import { listContacts } from "../contacts/api";
import type { Contact } from "../contacts/types";
import { createMeeting, meetingLink } from "../meet/api";
import { appTableHtml, contactBlockHtml, filterContacts, meetingLinkHtml } from "./appContent";

export type AppInsertKind = "app" | "contact" | "meeting";
const EVENT = "adrient-docs-app-insert";

export function openAppInsert(kind: AppInsertKind) {
  window.dispatchEvent(new CustomEvent(EVENT, { detail: kind }));
}

export function AppInsertDialogs({ editor, title }: { editor: Editor | null; title: string }) {
  const [kind, setKind] = useState<AppInsertKind | null>(null);
  useEffect(() => {
    const on = (e: Event) => setKind((e as CustomEvent<AppInsertKind>).detail);
    window.addEventListener(EVENT, on);
    return () => window.removeEventListener(EVENT, on);
  }, []);
  if (!editor || !kind) return null;
  const close = () => {
    setKind(null);
    editor.commands.focus();
  };
  const put = (html: string) => {
    editor.chain().focus().insertContent(html).run();
    close();
  };
  if (kind === "app") return <FromApp onClose={close} onInsert={put} />;
  if (kind === "contact") return <ContactPicker onClose={close} onInsert={put} />;
  return <MeetingLink title={title} onClose={close} onInsert={put} />;
}

interface Props {
  onClose: () => void;
  onInsert: (html: string) => void;
}

function FromApp({ onClose, onInsert }: Props) {
  const [picked, setPicked] = useState<AppSource | null>(null);
  const [table, setTable] = useState<AppTable | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function pick(src: AppSource) {
    setPicked(src);
    setTable(null);
    setError("");
    setBusy(true);
    try {
      setTable(await fetchAppTable(src));
    } catch (e) {
      setError((e as Error).message || "Could not load the data");
    } finally {
      setBusy(false);
    }
  }
  const built = useMemo(() => (table ? appTableHtml(table) : null), [table]);
  return (
    <Modal open onClose={onClose}>
      <ModalDialog aria-labelledby="docapp-title" sx={{ width: 560, maxWidth: "95vw" }}>
        <ModalClose />
        <Typography id="docapp-title" level="title-lg">Insert from app</Typography>
        <Typography level="body-sm">Adds a table at the cursor. Pick a source:</Typography>
        <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1 }}>
          {APP_SOURCES.map((s) => (
            <Button
              key={s.id}
              variant={picked?.id === s.id ? "soft" : "outlined"}
              color={picked?.id === s.id ? "primary" : "neutral"}
              onClick={() => void pick(s)}
              data-testid={`docapp-${s.id}`}
              sx={{ justifyContent: "flex-start", textAlign: "left", p: 1.25, gap: 1.25, height: "auto", alignItems: "flex-start" }}
            >
              <AppIcon iconName={s.icon} color="#126C5A" size={36} />
              <Box>
                <Typography level="title-sm">{s.label}</Typography>
                <Typography level="body-xs" sx={{ fontWeight: 400 }}>{s.description}</Typography>
              </Box>
            </Button>
          ))}
        </Box>
        {error && <Alert color="danger" variant="soft" role="alert">{error}</Alert>}
        {picked && !error && (
          <Typography level="body-sm" data-testid="docapp-status">
            {busy ? "Loading…" : built ? `${built.total} rows available${built.total > built.shown ? `; the first ${built.shown} will be inserted` : ""}.` : ""}
          </Typography>
        )}
        <Stack direction="row" spacing={1} justifyContent="flex-end">
          <Button variant="plain" color="neutral" onClick={onClose}>Cancel</Button>
          <Button disabled={!built} onClick={() => built && onInsert(built.html)} data-testid="docapp-insert">Insert</Button>
        </Stack>
      </ModalDialog>
    </Modal>
  );
}

function ContactPicker({ onClose, onInsert }: Props) {
  const [all, setAll] = useState<Contact[] | null>(null);
  const [q, setQ] = useState("");
  const [error, setError] = useState("");
  useEffect(() => {
    listContacts().then(setAll).catch(() => setError("Could not load contacts."));
  }, []);
  const shown = useMemo(() => filterContacts(all ?? [], q).slice(0, 100), [all, q]);
  return (
    <Modal open onClose={onClose}>
      <ModalDialog aria-labelledby="doccontact-title" sx={{ width: 480, maxWidth: "95vw" }}>
        <ModalClose />
        <Typography id="doccontact-title" level="title-lg">Insert contact</Typography>
        <Input autoFocus placeholder="Search name, company or email" value={q} onChange={(e) => setQ(e.target.value)} data-testid="doccontact-search" />
        {error && <Alert color="danger" variant="soft">{error}</Alert>}
        <List sx={{ maxHeight: 320, overflow: "auto" }}>
          {all === null && !error && <Typography level="body-sm" sx={{ p: 1 }}>Loading…</Typography>}
          {all && !shown.length && <Typography level="body-sm" sx={{ p: 1 }}>No contacts found.</Typography>}
          {shown.map((c) => (
            <ListItemButton key={c.id} onClick={() => onInsert(contactBlockHtml(c))} data-testid="doccontact-item">
              <Box>
                <Typography level="title-sm">{c.display_name || c.emails?.[0] || "(no name)"}</Typography>
                <Typography level="body-xs">{[c.company, c.emails?.[0]].filter(Boolean).join(" · ")}</Typography>
              </Box>
            </ListItemButton>
          ))}
        </List>
      </ModalDialog>
    </Modal>
  );
}

function MeetingLink({ title, onClose, onInsert }: Props & { title: string }) {
  const [name, setName] = useState(title && title !== "Untitled document" ? `${title} meeting` : "Meeting");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function go() {
    setBusy(true);
    setError("");
    try {
      const room = await createMeeting(name.trim() || "Meeting");
      const code = (room as { code?: string }).code;
      if (!code) throw new Error("no code");
      onInsert(meetingLinkHtml(name.trim() || "Meeting", meetingLink(code)));
    } catch {
      setError("Could not create the meeting.");
      setBusy(false);
    }
  }
  return (
    <Modal open onClose={onClose}>
      <ModalDialog aria-labelledby="docmeet-title" sx={{ width: 420, maxWidth: "95vw" }}>
        <ModalClose />
        <Typography id="docmeet-title" level="title-lg">Insert meeting link</Typography>
        <Typography level="body-sm">Creates a new Meet room and inserts its link.</Typography>
        <Input value={name} onChange={(e) => setName(e.target.value)} data-testid="docmeet-name" />
        {error && <Alert color="danger" variant="soft">{error}</Alert>}
        <Stack direction="row" spacing={1} justifyContent="flex-end">
          <Button variant="plain" color="neutral" onClick={onClose}>Cancel</Button>
          <Button loading={busy} onClick={() => void go()} data-testid="docmeet-create">Create and insert</Button>
        </Stack>
      </ModalDialog>
    </Modal>
  );
}
