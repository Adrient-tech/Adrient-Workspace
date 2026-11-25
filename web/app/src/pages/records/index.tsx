import { useEffect, useMemo, useState } from "react";
import { Box, Button, Card, Chip, CircularProgress, Container, DialogContent, DialogTitle, FormControl, FormLabel, Input, Modal, ModalClose, ModalDialog, Typography } from "@mui/joy";
import TableChartOutlined from "@mui/icons-material/TableChartOutlined";
import { Header } from "../../components/Header";
import { BizAssistant } from "../../components/BizAssistant";
import type { User } from "../../api/types";
import { deepParam } from "../../lib/deepLink";
import { records, type Base } from "./api";
import { Workspace } from "./Workspace";
import { createRecordsTools, RECORDS_HINT, RECORDS_SUGGESTIONS } from "./ai/tools";

export const ACCENT = "#126C5A";

function NewBase({ templates, onClose, onCreated }: { templates: { id: string; name: string; blurb: string }[]; onClose: () => void; onCreated: (b: Base) => void }) {
  const [name, setName] = useState("");
  const [tpl, setTpl] = useState("blank");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 480, maxWidth: "95vw" }} role="dialog" aria-label="New base">
        <ModalClose />
        <DialogTitle>New base</DialogTitle>
        <DialogContent>
          <FormControl sx={{ mb: 1.5 }}><FormLabel>Name</FormLabel><Input autoFocus value={name} onChange={(e) => setName(e.target.value)} data-testid="base-name" /></FormControl>
          <FormLabel>Start from</FormLabel>
          <Box sx={{ display: "grid", gap: 0.8, mb: 1.5 }}>
            {templates.map((t) => (
              <Card key={t.id} size="sm" variant={tpl === t.id ? "soft" : "outlined"} color={tpl === t.id ? "primary" : "neutral"} onClick={() => setTpl(t.id)} sx={{ cursor: "pointer" }} role="radio" aria-checked={tpl === t.id}>
                <Typography level="title-sm">{t.name}</Typography><Typography level="body-xs">{t.blurb}</Typography>
              </Card>
            ))}
          </Box>
          {err && <Typography color="danger" level="body-sm" role="alert">{err}</Typography>}
          <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1 }}>
            <Button variant="plain" color="neutral" onClick={onClose}>Cancel</Button>
            <Button loading={busy} disabled={!name.trim()} data-testid="base-create" onClick={() => { setBusy(true); records.createBase({ name, template: tpl }).then(onCreated).catch((e: Error) => { setErr(e.message); setBusy(false); }); }}>Create</Button>
          </Box>
        </DialogContent>
      </ModalDialog>
    </Modal>
  );
}

export default function RecordsApp({ user }: { user: User }) {
  const [bases, setBases] = useState<Base[] | null>(null);
  const [trash, setTrash] = useState(false);
  const [open, setOpen] = useState(deepParam("base"));
  const [templates, setTemplates] = useState<{ id: string; name: string; blurb: string }[]>([]);
  const [newOpen, setNewOpen] = useState(false);
  const [err, setErr] = useState("");
  const tools = useMemo(() => createRecordsTools(), []);
  const load = () => records.bases(trash).then(setBases).catch((e: Error) => setErr(e.message));
  useEffect(() => { void load(); // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [trash, open]);
  useEffect(() => { records.meta().then((m) => setTemplates(m.templates)).catch(() => undefined); }, []);
  const restoreFile = async (f: File) => {
    try {
      const b = await records.restoreBackup(await f.text(), f.name.replace(/\.[^.]+$/, ""));
      setOpen(b.id);
    } catch (e) { setErr((e as Error).message); }
  };
  return (
    <>
      <Header user={user} />
      <Container maxWidth={false} sx={{ py: 2, px: { xs: 1, sm: 2 } }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, mb: 1.5, flexWrap: "wrap" }}>
          <TableChartOutlined sx={{ fontSize: 28, color: ACCENT }} />
          <Typography level="h2" sx={{ fontSize: { xs: "xl", sm: "xl3" } }}>Records</Typography>
          <Box sx={{ ml: "auto" }}><BizAssistant app="records" title="Records" tools={tools} suggestions={RECORDS_SUGGESTIONS} systemHint={RECORDS_HINT} /></Box>
        </Box>
        {open ? (
          <Workspace baseId={open} initialTable={deepParam("table")} initialRecord={deepParam("record")} onBack={() => { setOpen(""); window.history.replaceState(null, "", "/records"); }} />
        ) : (
          <>
            <Box sx={{ display: "flex", gap: 1, mb: 2, flexWrap: "wrap" }}>
              {!trash && <Button onClick={() => setNewOpen(true)} data-testid="new-base">＋ New base</Button>}
              {!trash && <Button variant="outlined" color="neutral" component="label">Restore from backup<input hidden type="file" accept=".json,application/json" onChange={(e) => { const f = e.target.files?.[0]; if (f) void restoreFile(f); }} /></Button>}
              <Button variant={trash ? "solid" : "plain"} color="neutral" onClick={() => setTrash(!trash)}>{trash ? "‹ Back to bases" : "Trash"}</Button>
            </Box>
            {err && <Typography color="danger" role="alert">{err}</Typography>}
            {!bases && <CircularProgress />}
            {bases && bases.length === 0 && (
              <Box sx={{ p: 4, textAlign: "center", color: "text.secondary" }}>{trash ? "The trash is empty." : "No bases yet. Create one from a template or a CSV file."}</Box>
            )}
            <Box sx={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(250px, 1fr))", gap: 1.5 }}>
              {bases?.map((b) => (
                <Card key={b.id} variant="outlined" sx={{ cursor: "pointer", "&:hover": { boxShadow: "md" } }} onClick={() => !trash && setOpen(b.id)} data-testid="base-card">
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                    <Box sx={{ width: 36, height: 36, borderRadius: "8px", bgcolor: b.color, color: "#fff", display: "grid", placeItems: "center", fontWeight: 700 }}>{b.name.slice(0, 1).toUpperCase()}</Box>
                    <Box sx={{ minWidth: 0 }}>
                      <Typography level="title-md" noWrap>{b.name}</Typography>
                      <Typography level="body-xs">Updated {new Date(b.updated_at).toLocaleDateString()}</Typography>
                    </Box>
                  </Box>
                  {b.description && <Typography level="body-sm" sx={{ mt: 0.5 }}>{b.description}</Typography>}
                  <Box sx={{ mt: 0.5 }}>{b.role < 3 && <Chip size="sm" variant="soft">{b.role >= 2 ? "Can edit" : "View only"}</Chip>}</Box>
                  {trash && (
                    <Box sx={{ display: "flex", gap: 1 }}>
                      <Button size="sm" onClick={(e) => { e.stopPropagation(); void records.restoreBase(b.id).then(load); }}>Restore</Button>
                      <Button size="sm" color="danger" variant="soft" onClick={(e) => { e.stopPropagation(); if (window.confirm(`Delete "${b.name}" forever? This cannot be undone.`)) void records.deleteBase(b.id).then(load); }}>Delete forever</Button>
                    </Box>
                  )}
                </Card>
              ))}
            </Box>
          </>
        )}
      </Container>
      {newOpen && <NewBase templates={templates} onClose={() => setNewOpen(false)} onCreated={(b) => { setNewOpen(false); setOpen(b.id); }} />}
    </>
  );
}
