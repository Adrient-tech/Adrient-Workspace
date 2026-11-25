import { searchDirectory } from "../../api/directory";
import { useCallback, useEffect, useState } from "react";
import { Alert, Avatar, Box, Button, CircularProgress, Input, Modal, ModalClose, ModalDialog, Typography } from "@mui/joy";
import { getHistory, undoChange, type HistoryItem } from "./extrasApi";
import { createContact } from "./api";

/** Recent changes with an Undo button (30 days). */
export function UndoDialog({ onClose, onUndone }: { onClose: () => void; onUndone: () => Promise<void> | void }) {
  const [items, setItems] = useState<HistoryItem[] | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState("");
  const load = useCallback(() => getHistory().then(setItems).catch((e: Error) => { setErr(e.message); setItems([]); }), []);
  useEffect(() => { void load(); }, [load]);
  async function undo(h: HistoryItem) {
    setBusy(h.id);
    setErr("");
    try {
      await undoChange(h.id);
      await onUndone();
      await load();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy("");
    }
  }
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: "min(560px, 96vw)", maxHeight: "86vh", overflowY: "auto" }} data-testid="undo-dialog">
        <ModalClose />
        <Typography level="title-lg">Undo changes</Typography>
        <Typography level="body-sm">Edits and deletions from the last 30 days. Undo puts the contact back the way it was.</Typography>
        {err && <Alert color="danger">{err}</Alert>}
        {items === null && <CircularProgress />}
        {items?.length === 0 && <Typography level="body-sm" sx={{ mt: 1 }}>No recent changes.</Typography>}
        {items?.map((h) => (
          <Box key={h.id} sx={{ display: "flex", alignItems: "center", gap: 1, py: 0.75, borderBottom: "1px solid", borderColor: "divider" }} data-testid="undo-row">
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Typography level="body-sm" noWrap>{h.name || "(no name)"}</Typography>
              <Typography level="body-xs">
                {h.action === "trash" ? "Deleted" : "Edited"} · {new Date(h.at).toLocaleString([], { dateStyle: "medium", timeStyle: "short" })}
              </Typography>
            </Box>
            {h.undone ? (
              <Typography level="body-xs">Undone</Typography>
            ) : (
              <Button size="sm" variant="outlined" color="neutral" loading={busy === h.id} onClick={() => undo(h)}>Undo</Button>
            )}
          </Box>
        ))}
      </ModalDialog>
    </Modal>
  );
}

/** Real how-to text for moving contacts in and out. */
export function HelpDialog({ onClose }: { onClose: () => void }) {
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: "min(600px, 96vw)", maxHeight: "86vh", overflowY: "auto" }}>
        <ModalClose />
        <Typography level="title-lg">Importing, exporting and syncing</Typography>
        <Typography level="title-sm" sx={{ mt: 1 }}>From Google Contacts</Typography>
        <Typography level="body-sm">In Google Contacts choose Export, pick Google CSV or vCard, then use Create contact, then Import here.</Typography>
        <Typography level="title-sm" sx={{ mt: 1 }}>From Outlook</Typography>
        <Typography level="body-sm">In Outlook choose Manage contacts, then Export contacts (CSV), then import the file here. Dates such as 05/06/1990 are skipped when they could be read two ways.</Typography>
        <Typography level="title-sm" sx={{ mt: 1 }}>From Apple Contacts</Typography>
        <Typography level="body-sm">Select the contacts, File, Export, Export vCard, then import the .vcf file here.</Typography>
        <Typography level="title-sm" sx={{ mt: 1 }}>Export</Typography>
        <Typography level="body-sm">The download button exports every contact (or the open group) as a .vcf file that Google, Outlook and Apple can import.</Typography>
        <Typography level="title-sm" sx={{ mt: 1 }}>Birthdays</Typography>
        <Typography level="body-sm">Add a birthday or anniversary to a contact and it appears in Calendar under Birthdays.</Typography>
        <Typography level="body-xs" sx={{ mt: 1.5 }}>Live two-way sync with other address books (CardDAV) is not available yet.</Typography>
      </ModalDialog>
    </Modal>
  );
}

interface Member {
  id: string;
  name: string;
  email: string;
}

/** People in your organisation. Add any of them to My contacts. */
export function DirectoryPanel({ existing, onAdded }: { existing: string[]; onAdded: () => Promise<void> | void }) {
  const [q, setQ] = useState("");
  const [list, setList] = useState<Member[] | null>(null);
  const [added, setAdded] = useState<Set<string>>(new Set());
  const [err, setErr] = useState("");
  useEffect(() => {
    const t = setTimeout(() => {
      searchDirectory(q)
        .then((m) => setList(m as Member[]))
        .catch((e: Error) => { setErr(e.message); setList([]); });
    }, 200);
    return () => clearTimeout(t);
  }, [q]);
  async function add(m: Member) {
    const [first, ...rest] = m.name.split(" ");
    try {
      await createContact({ display_name: m.name, first_name: first ?? "", last_name: rest.join(" "), emails: [m.email] });
      setAdded((s) => new Set(s).add(m.email.toLowerCase()));
      await onAdded();
    } catch (e) {
      setErr((e as Error).message);
    }
  }
  return (
    <Box data-directory="true" data-testid="directory-panel">
      <Typography level="title-md" sx={{ mb: 0.5 }}>Directory</Typography>
      <Typography level="body-sm" sx={{ mb: 1 }}>Everyone in your organisation. My contacts holds the people you added yourself.</Typography>
      <Input placeholder="Search people" value={q} onChange={(e) => setQ(e.target.value)} sx={{ mb: 1, maxWidth: 420 }} slotProps={{ input: { "data-testid": "directory-search" } }} />
      {err && <Alert color="danger" sx={{ mb: 1 }}>{err}</Alert>}
      {list === null && <CircularProgress />}
      {list?.length === 0 && <Typography level="body-sm">No one found.</Typography>}
      {list?.map((m) => {
        const have = added.has(m.email.toLowerCase()) || existing.includes(m.email.toLowerCase());
        return (
          <Box key={m.id || m.email} sx={{ display: "flex", alignItems: "center", gap: 1.5, py: 0.75, borderBottom: "1px solid", borderColor: "divider" }} data-testid="directory-row">
            <Avatar size="sm">{(m.name || m.email)[0]?.toUpperCase()}</Avatar>
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Typography level="body-sm" noWrap>{m.name || m.email}</Typography>
              <Typography level="body-xs" noWrap>{m.email}</Typography>
            </Box>
            {have ? <Typography level="body-xs">In My contacts</Typography> : <Button size="sm" variant="outlined" color="neutral" onClick={() => add(m)}>Add</Button>}
          </Box>
        );
      })}
    </Box>
  );
}
