// Dialogs added with the ribbon: Building blocks, Smart chips, Citations
// and bibliography, Accessibility checker, Preferences. Opened through
// openExtraDialog(kind) like the other Docs dialogs.
import { useEffect, useMemo, useState } from "react";
import {
  Alert, Box, Button, Checkbox, Chip, FormControl, FormLabel, IconButton, Input, List, ListItem, ListItemButton, Modal, ModalClose,
  ModalDialog, Option, Select, Stack, Tab, TabList, TabPanel, Tabs, Typography,
} from "@mui/joy";
import DeleteIcon from "@mui/icons-material/Delete";
import type { Editor } from "@tiptap/react";
import { BUILDING_BLOCKS, blockHtml } from "./buildingBlocks";
import { APP_CHIPS, appChip, dateChip, fileChip, personChip } from "./chips";
import { listContacts } from "../contacts/api";
import type { Contact } from "../contacts/types";
import { listRecent, listFiles } from "../drive/api";
import type { DriveFile } from "../drive/types";
import {
  STYLE_LABEL, bibliographyHtml, formatReference, inTextCitation, loadSources, refToHtml, saveSources,
  type CiteStyle, type Source, type SourceKind,
} from "./citations";
import { checkAccessibility } from "./a11y";
import { setPrefs, usePrefs } from "./docPrefs";
import { spellService } from "../../lib/spell/service";

export type ExtraDialogKind = "blocks" | "chips" | "citations" | "a11y" | "prefs";
const EVENT = "adrient-docs-extra-dialog";
export function openExtraDialog(kind: ExtraDialogKind) {
  window.dispatchEvent(new CustomEvent(EVENT, { detail: kind }));
}

export function ExtraDialogs({ editor, docId }: { editor: Editor | null; docId: string }) {
  const [kind, setKind] = useState<ExtraDialogKind | null>(null);
  useEffect(() => {
    const on = (e: Event) => setKind((e as CustomEvent<ExtraDialogKind>).detail);
    window.addEventListener(EVENT, on);
    return () => window.removeEventListener(EVENT, on);
  }, []);
  if (!kind) return null;
  const close = () => {
    setKind(null);
    editor?.commands.focus();
  };
  if (kind === "prefs") return <Prefs editor={editor} onClose={close} />;
  if (!editor) return null;
  if (kind === "blocks") return <Blocks editor={editor} onClose={close} />;
  if (kind === "chips") return <Chips editor={editor} onClose={close} />;
  if (kind === "citations") return <Citations editor={editor} docId={docId} onClose={close} />;
  return <A11y editor={editor} onClose={close} />;
}

function Blocks({ editor, onClose }: { editor: Editor; onClose: () => void }) {
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 520, maxWidth: "95vw" }} aria-labelledby="bb-title">
        <ModalClose />
        <Typography id="bb-title" level="title-lg">Building blocks</Typography>
        <Typography level="body-sm">Inserted at the cursor.</Typography>
        {(["Documents", "Parts"] as const).map((g) => (
          <Box key={g}>
            <Typography level="title-sm" sx={{ mb: 0.5 }}>{g}</Typography>
            <List size="sm" sx={{ gap: 0.25 }}>
              {BUILDING_BLOCKS.filter((b) => b.group === g).map((b) => (
                <ListItem key={b.id}>
                  <ListItemButton
                    data-testid={`block-${b.id}`}
                    onClick={() => {
                      editor.chain().focus().insertContent(blockHtml(b)).run();
                      onClose();
                    }}
                  >
                    <Box>
                      <Typography level="title-sm">{b.name}</Typography>
                      <Typography level="body-xs">{b.description}</Typography>
                    </Box>
                  </ListItemButton>
                </ListItem>
              ))}
            </List>
          </Box>
        ))}
      </ModalDialog>
    </Modal>
  );
}

function Chips({ editor, onClose }: { editor: Editor; onClose: () => void }) {
  const put = (html: string) => {
    editor.chain().focus().insertContent(html).run();
    onClose();
  };
  const [contacts, setContacts] = useState<Contact[] | null>(null);
  const [files, setFiles] = useState<DriveFile[] | null>(null);
  const [q, setQ] = useState("");
  const [err, setErr] = useState("");
  const [date, setDate] = useState(() => new Date().toISOString().slice(0, 10));
  useEffect(() => {
    listContacts().then(setContacts).catch(() => setErr("Could not load contacts."));
    listRecent()
      .catch(() => listFiles(""))
      .then((f) => setFiles(f))
      .catch(() => setErr((e) => e || "Could not load Drive files."));
  }, []);
  const ql = q.trim().toLowerCase();
  const people = useMemo(
    () => (contacts ?? []).filter((c) => c.emails?.[0] && (!ql || `${c.display_name} ${c.company ?? ""} ${c.emails[0]}`.toLowerCase().includes(ql))).slice(0, 50),
    [contacts, ql],
  );
  const fl = useMemo(() => (files ?? []).filter((f) => !ql || f.name.toLowerCase().includes(ql)).slice(0, 50), [files, ql]);
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 520, maxWidth: "95vw" }} aria-labelledby="chips-title">
        <ModalClose />
        <Typography id="chips-title" level="title-lg">Smart chips</Typography>
        {err && <Alert color="warning" variant="soft">{err}</Alert>}
        <Tabs defaultValue="people">
          <TabList>
            <Tab value="people">People</Tab>
            <Tab value="date">Date</Tab>
            <Tab value="files">Drive files</Tab>
            <Tab value="apps">Apps</Tab>
          </TabList>
          <TabPanel value="people">
            <Input placeholder="Search" value={q} onChange={(e) => setQ(e.target.value)} sx={{ mb: 1 }} />
            <List size="sm" sx={{ maxHeight: 260, overflow: "auto" }}>
              {contacts === null && <Typography level="body-sm">Loading…</Typography>}
              {contacts && !people.length && <Typography level="body-sm">No contacts with an email address.</Typography>}
              {people.map((c) => (
                <ListItemButton key={c.id} data-testid="chip-person" onClick={() => put(personChip(c.display_name, c.emails[0]))}>
                  {c.display_name || c.emails[0]} <Typography level="body-xs" sx={{ ml: 1 }}>{c.emails[0]}</Typography>
                </ListItemButton>
              ))}
            </List>
          </TabPanel>
          <TabPanel value="date">
            <Stack direction="row" spacing={1}>
              <Input type="date" value={date} onChange={(e) => setDate(e.target.value)} slotProps={{ input: { "aria-label": "Date" } }} />
              <Button disabled={!date} data-testid="chip-date" onClick={() => put(dateChip(new Date(date + "T12:00:00")))}>Insert date chip</Button>
            </Stack>
            <Typography level="body-xs" sx={{ mt: 1 }}>The chip opens that day in Calendar.</Typography>
          </TabPanel>
          <TabPanel value="files">
            <Input placeholder="Search recent files" value={q} onChange={(e) => setQ(e.target.value)} sx={{ mb: 1 }} />
            <List size="sm" sx={{ maxHeight: 260, overflow: "auto" }}>
              {files === null && <Typography level="body-sm">Loading…</Typography>}
              {files && !fl.length && <Typography level="body-sm">No files found.</Typography>}
              {fl.map((f) => (
                <ListItemButton key={f.id} data-testid="chip-file" onClick={() => put(fileChip(f))}>{f.name}</ListItemButton>
              ))}
            </List>
          </TabPanel>
          <TabPanel value="apps">
            <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
              {APP_CHIPS.map((a) => (
                <Chip key={a.id} variant="soft" color="primary" onClick={() => put(appChip(a.label, a.href))} data-testid={`chip-app-${a.id}`}>
                  {a.label}
                </Chip>
              ))}
            </Box>
            <Typography level="body-xs" sx={{ mt: 1 }}>The chip opens that app in the workspace.</Typography>
          </TabPanel>
        </Tabs>
      </ModalDialog>
    </Modal>
  );
}

const EMPTY: Source = { id: "", kind: "book", authors: "", title: "", year: "", publisher: "", container: "", volume: "", pages: "", url: "" };

function Citations({ editor, docId, onClose }: { editor: Editor; docId: string; onClose: () => void }) {
  const [style, setStyle] = useState<CiteStyle>(() => (localStorage.getItem("adrient.docs.citeStyle") as CiteStyle) || "apa");
  const [list, setList] = useState<Source[]>(() => loadSources(docId));
  const [draft, setDraft] = useState<Source>(EMPTY);
  const [page, setPage] = useState("");
  const persist = (l: Source[]) => {
    setList(l);
    saveSources(docId, l);
  };
  const set = <K extends keyof Source>(k: K, v: Source[K]) => setDraft((d) => ({ ...d, [k]: v }));
  const valid = draft.title.trim() && (draft.authors.trim() || draft.kind === "website");
  const add = () => {
    persist([...list, { ...draft, id: `s${Date.now().toString(36)}` }]);
    setDraft(EMPTY);
  };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 640, maxWidth: "96vw", maxHeight: "92vh", overflow: "auto" }} aria-labelledby="cite-title">
        <ModalClose />
        <Typography id="cite-title" level="title-lg">Citations and bibliography</Typography>
        <FormControl orientation="horizontal" sx={{ gap: 1, alignItems: "center" }}>
          <FormLabel>Style</FormLabel>
          <Select
            size="sm"
            value={style}
            onChange={(_, v) => {
              if (!v) return;
              setStyle(v);
              localStorage.setItem("adrient.docs.citeStyle", v);
            }}
            data-testid="cite-style"
          >
            {(Object.keys(STYLE_LABEL) as CiteStyle[]).map((s) => (
              <Option key={s} value={s}>{STYLE_LABEL[s]}</Option>
            ))}
          </Select>
          <FormLabel sx={{ ml: 2 }}>Page</FormLabel>
          <Input size="sm" sx={{ width: 80 }} value={page} onChange={(e) => setPage(e.target.value)} placeholder="optional" />
        </FormControl>
        <Typography level="title-sm">Sources in this document</Typography>
        {!list.length && <Typography level="body-sm">None yet. Add one below.</Typography>}
        <List size="sm" data-testid="cite-sources">
          {list.map((s) => (
            <ListItem
              key={s.id}
              endAction={
                <IconButton size="sm" variant="plain" aria-label={`Remove ${s.title}`} onClick={() => persist(list.filter((x) => x.id !== s.id))}>
                  <DeleteIcon />
                </IconButton>
              }
            >
              <Box sx={{ minWidth: 0 }}>
                <Typography level="body-sm" dangerouslySetInnerHTML={{ __html: refToHtml(formatReference(s, style)) }} />
                <Button
                  size="sm"
                  variant="soft"
                  sx={{ mt: 0.5 }}
                  data-testid="cite-insert"
                  onClick={() => {
                    editor.chain().focus().insertContent(inTextCitation(s, style, page) + " ").run();
                    onClose();
                  }}
                >
                  Insert citation {inTextCitation(s, style, page)}
                </Button>
              </Box>
            </ListItem>
          ))}
        </List>
        <Typography level="title-sm">Add a source</Typography>
        <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1 }}>
          <Select size="sm" value={draft.kind} onChange={(_, v) => v && set("kind", v as SourceKind)} slotProps={{ button: { "aria-label": "Source type" } }}>
            <Option value="book">Book</Option>
            <Option value="article">Journal article</Option>
            <Option value="website">Website</Option>
          </Select>
          <Input size="sm" placeholder="Year" value={draft.year} onChange={(e) => set("year", e.target.value)} slotProps={{ input: { "aria-label": "Year" } }} />
          <Input size="sm" placeholder="Authors: Last, First; Last, First" value={draft.authors} onChange={(e) => set("authors", e.target.value)} sx={{ gridColumn: "1 / -1" }} slotProps={{ input: { "aria-label": "Authors" } }} />
          <Input size="sm" placeholder="Title" value={draft.title} onChange={(e) => set("title", e.target.value)} sx={{ gridColumn: "1 / -1" }} slotProps={{ input: { "aria-label": "Title" } }} />
          {draft.kind === "book" && <Input size="sm" placeholder="Publisher" value={draft.publisher} onChange={(e) => set("publisher", e.target.value)} slotProps={{ input: { "aria-label": "Publisher" } }} />}
          {draft.kind !== "book" && <Input size="sm" placeholder={draft.kind === "article" ? "Journal" : "Website name"} value={draft.container} onChange={(e) => set("container", e.target.value)} slotProps={{ input: { "aria-label": "Container" } }} />}
          {draft.kind === "article" && <Input size="sm" placeholder="Volume" value={draft.volume} onChange={(e) => set("volume", e.target.value)} slotProps={{ input: { "aria-label": "Volume" } }} />}
          {draft.kind === "article" && <Input size="sm" placeholder="Pages" value={draft.pages} onChange={(e) => set("pages", e.target.value)} slotProps={{ input: { "aria-label": "Pages" } }} />}
          <Input size="sm" placeholder="URL (optional)" value={draft.url} onChange={(e) => set("url", e.target.value)} slotProps={{ input: { "aria-label": "URL" } }} />
        </Box>
        <Stack direction="row" spacing={1} justifyContent="space-between">
          <Button variant="outlined" disabled={!valid} onClick={add} data-testid="cite-add">Add source</Button>
          <Button
            disabled={!list.length}
            data-testid="cite-bibliography"
            onClick={() => {
              editor.chain().focus().insertContent(bibliographyHtml(list, style)).run();
              onClose();
            }}
          >
            Insert bibliography
          </Button>
        </Stack>
      </ModalDialog>
    </Modal>
  );
}

function A11y({ editor, onClose }: { editor: Editor; onClose: () => void }) {
  const issues = useMemo(() => checkAccessibility(editor.state.doc), [editor]);
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 520, maxWidth: "95vw", maxHeight: "85vh", overflow: "auto" }} aria-labelledby="a11y-title">
        <ModalClose />
        <Typography id="a11y-title" level="title-lg">Accessibility checker</Typography>
        {!issues.length && <Alert color="success" variant="soft" data-testid="a11y-ok">No accessibility issues found.</Alert>}
        <List size="sm" data-testid="a11y-issues">
          {issues.map((i, n) => (
            <ListItemButton
              key={n}
              onClick={() => {
                editor.chain().focus().setTextSelection(Math.min(i.pos + 1, editor.state.doc.content.size)).scrollIntoView().run();
                onClose();
              }}
            >
              <Chip size="sm" color={i.severity === "error" ? "danger" : "warning"} variant="soft" sx={{ mr: 1 }}>{i.severity}</Chip>
              {i.message}
            </ListItemButton>
          ))}
        </List>
      </ModalDialog>
    </Modal>
  );
}

function Prefs({ editor, onClose }: { editor: Editor | null; onClose: () => void }) {
  const p = usePrefs();
  const [spell, setSpell] = useState(spellService().enabled);
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 420, maxWidth: "95vw" }} aria-labelledby="prefs-title">
        <ModalClose />
        <Typography id="prefs-title" level="title-lg">Preferences</Typography>
        <Typography level="body-xs">Stored in this browser.</Typography>
        <Checkbox label="Use the ribbon (off = classic toolbar)" checked={p.ribbon} onChange={(e) => setPrefs({ ribbon: e.target.checked })} />
        <Checkbox label="Show the ruler" checked={p.ruler} onChange={(e) => setPrefs({ ruler: e.target.checked })} />
        <Checkbox
          label="Check spelling as you type"
          checked={spell}
          onChange={(e) => {
            setSpell(e.target.checked);
            editor?.commands.setSpellcheck(e.target.checked);
          }}
        />
        <Stack direction="row" justifyContent="flex-end"><Button onClick={onClose}>Done</Button></Stack>
      </ModalDialog>
    </Modal>
  );
}
