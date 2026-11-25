import { useEffect, useState } from "react";
import {
  Box, Button, Checkbox, DialogTitle, Input, List, ListItem, ListItemButton, Modal, ModalClose, ModalDialog, Textarea, Typography,
} from "@mui/joy";
import { deleteSignature, listSignatures, saveSignature } from "./api";
import type { MailSignature } from "./types";

/** Create, edit, delete signatures and pick the default one (stored per user on the server). */
export function SignaturesDialog({ onClose }: { onClose: () => void }) {
  const [list, setList] = useState<MailSignature[]>([]);
  const [name, setName] = useState("");
  const [body, setBody] = useState("");
  const [isDefault, setIsDefault] = useState(false);
  const [err, setErr] = useState("");

  const load = () => listSignatures().then(setList).catch(() => setErr("Could not load signatures."));
  useEffect(() => {
    void load();
  }, []);

  function edit(s: MailSignature) {
    setName(s.name);
    setBody(s.body);
    setIsDefault(s.is_default);
  }
  async function save() {
    setErr("");
    try {
      await saveSignature({ name: name.trim(), body, is_default: isDefault });
      setName("");
      setBody("");
      setIsDefault(false);
      await load();
    } catch {
      setErr("Enter a name and a signature (max 4000 characters).");
    }
  }
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 560, maxWidth: "95vw", maxHeight: "90vh", overflow: "auto" }}>
        <ModalClose />
        <DialogTitle>Signatures</DialogTitle>
        <List size="sm" data-testid="signature-list">
          {list.length === 0 && <Typography level="body-sm" sx={{ opacity: 0.7 }}>No signatures yet.</Typography>}
          {list.map((s) => (
            <ListItem
              key={s.id}
              endAction={
                <Button size="sm" variant="plain" color="danger" onClick={() => deleteSignature(s.id).then(load)}>
                  Delete
                </Button>
              }
            >
              <ListItemButton onClick={() => edit(s)}>
                <Box>
                  <Typography level="title-sm">
                    {s.name}
                    {s.is_default ? " · default" : ""}
                  </Typography>
                  <Typography level="body-xs" noWrap sx={{ maxWidth: 380 }}>
                    {s.body.replace(/\s+/g, " ")}
                  </Typography>
                </Box>
              </ListItemButton>
            </ListItem>
          ))}
        </List>
        <Input placeholder="Signature name" value={name} onChange={(e) => setName(e.target.value)} slotProps={{ input: { "aria-label": "Signature name" } }} />
        <Textarea minRows={4} placeholder="Signature text" value={body} onChange={(e) => setBody(e.target.value)} />
        <Checkbox label="Use for new messages" checked={isDefault} onChange={(e) => setIsDefault(e.target.checked)} />
        {err && <Typography level="body-sm" color="danger">{err}</Typography>}
        <Box sx={{ display: "flex", gap: 1, justifyContent: "flex-end" }}>
          <Button variant="plain" onClick={onClose}>Close</Button>
          <Button onClick={save} disabled={!name.trim() || !body.trim()}>Save signature</Button>
        </Box>
      </ModalDialog>
    </Modal>
  );
}
