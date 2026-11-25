import { PersonSelect } from "../../components/PeoplePicker";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Box, Button, CircularProgress, Container, Dropdown, IconButton, Input, List, ListItem, ListItemButton, Menu, MenuButton, MenuItem, Modal, ModalClose, ModalDialog, DialogTitle, DialogContent, Option, Select, Textarea, Typography } from "@mui/joy";
import MenuBook from "@mui/icons-material/MenuBook";
import type { Editor } from "@tiptap/react";
import { Header } from "../../components/Header";
import { BizAssistant } from "../../components/BizAssistant";
import type { User } from "../../api/types";
import { deepParam } from "../../lib/deepLink";
import { DrivePicker } from "../drive/DrivePicker";
import { Workspace } from "../records/Workspace";
import { wiki, type Comment, type Member, type Page, type PageRef, type Space, type Version } from "./api";
import { WikiEditor } from "./Editor";
import { breadcrumb, buildTree, canMoveInto, pageLink, visibleRows, wordCount, type TreeNode } from "./logic";
import { createWikiTools, WIKI_HINT, WIKI_SUGGESTIONS } from "./ai/tools";

const ACCENT = "#126C5A";
const EMOJI = ["📄", "📝", "📚", "🎯", "✅", "📅", "💡", "🧭", "🛠️", "📊", "🚀", "🔒", "👥", "🗂️"];

function SpaceDialog({ space, onClose, onSaved }: { space: Space; onClose: () => void; onSaved: () => void }) {
  const [members, setMembers] = useState<Member[]>([]);
  const [people, setPeople] = useState<{ id: string; name: string }[]>([]);
  const [access, setAccess] = useState(space.org_access);
  const [name, setName] = useState(space.name);
  const [add, setAdd] = useState<string | null>(null);
  const [addName, setAddName] = useState("");
  const [err, setErr] = useState("");
  useEffect(() => {
    wiki.people().then((r) => setPeople(r.people)).catch(() => undefined);
    if (space.kind === "team") wiki.members(space.id).then(setMembers).catch((e: Error) => setErr(e.message));
  }, [space]);
  const nm = new Map(people.map((p) => [p.id, p.name]));
  const save = async () => {
    try {
      await wiki.updateSpace(space.id, { name, icon: space.icon, org_access: access });
      if (space.kind === "team") await wiki.setMembers(space.id, members);
      onSaved();
    } catch (e) { setErr((e as Error).message); }
  };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 460, maxWidth: "95vw" }} role="dialog" aria-label="Space settings">
        <ModalClose />
        <DialogTitle>{space.kind === "personal" ? "My pages" : "Space settings"}</DialogTitle>
        <DialogContent>
          {space.kind === "personal" ? <Typography level="body-sm">Your personal space is private to you. To share a page, move it to a shared space.</Typography> : (
            <>
              <Input value={name} onChange={(e) => setName(e.target.value)} sx={{ mb: 1.5 }} aria-label="Space name" />
              <Typography level="title-sm">Everyone in the organisation</Typography>
              <Select value={access} onChange={(_, v) => v && setAccess(v as Space["org_access"])} sx={{ mb: 1.5 }} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}><Option value="none">No access</Option><Option value="view">Can view</Option><Option value="edit">Can edit</Option></Select>
              <Typography level="title-sm">People</Typography>
              {members.map((m) => (
                <Box key={m.user_id} sx={{ display: "flex", gap: 1, alignItems: "center", mb: 0.5 }}>
                  <Typography sx={{ flex: 1 }}>{m.name ?? nm.get(m.user_id) ?? m.user_id}</Typography>
                  <Select size="sm" value={m.role} onChange={(_, v) => v && setMembers(members.map((x) => (x.user_id === m.user_id ? { ...x, role: v as Member["role"] } : x)))} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}><Option value="viewer">Viewer</Option><Option value="editor">Editor</Option><Option value="admin">Admin</Option></Select>
                  <IconButton size="sm" variant="plain" aria-label="Remove person" onClick={() => setMembers(members.filter((x) => x.user_id !== m.user_id))}>✕</IconButton>
                </Box>
              ))}
              <Box sx={{ display: "flex", gap: 1, mt: 1 }}>
                <PersonSelect value={add ?? ""} known={people} onChange={(id, item) => { setAdd(id || null); setAddName(item?.label ?? ""); }} placeholder="Add a person" ariaLabel="Add a person" />
                <Button size="sm" disabled={!add || add === space.owner_id || members.some((m) => m.user_id === add)} onClick={() => { if (add) { setMembers([...members, { user_id: add, role: "editor", name: nm.get(add) ?? addName }]); setAdd(null); } }}>Add</Button>
              </Box>
            </>
          )}
          {err && <Typography color="danger" level="body-sm" role="alert">{err}</Typography>}
          <Box sx={{ display: "flex", justifyContent: "space-between", mt: 2 }}>
            {space.kind === "team" && space.role >= 3 ? <Button color="danger" variant="soft" onClick={() => { if (window.confirm(`Delete the space "${space.name}" and all its pages?`)) void wiki.deleteSpace(space.id).then(onSaved).catch((e: Error) => setErr(e.message)); }}>Delete space</Button> : <span />}
            <Box sx={{ display: "flex", gap: 1 }}><Button variant="plain" color="neutral" onClick={onClose}>Cancel</Button>{space.kind === "team" && <Button onClick={() => void save()}>Save</Button>}</Box>
          </Box>
        </DialogContent>
      </ModalDialog>
    </Modal>
  );
}

function PagePicker({ spaceId, onPick, onClose }: { spaceId: string; onPick: (p: PageRef | Page) => void; onClose: () => void }) {
  const [q, setQ] = useState("");
  const [rows, setRows] = useState<(PageRef | Page)[]>([]);
  useEffect(() => {
    const t = setTimeout(() => { (q.trim() ? wiki.search(q) : wiki.tree(spaceId)).then(setRows).catch(() => setRows([])); }, 200);
    return () => clearTimeout(t);
  }, [q, spaceId]);
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 420, maxWidth: "95vw" }} role="dialog" aria-label="Link a page">
        <ModalClose /><DialogTitle>Link a page</DialogTitle>
        <DialogContent>
          <Input autoFocus placeholder="Search pages" value={q} onChange={(e) => setQ(e.target.value)} sx={{ mb: 1 }} />
          <List sx={{ maxHeight: 300, overflow: "auto" }}>{rows.map((r) => <ListItem key={r.id}><ListItemButton onClick={() => onPick(r)}>{r.icon || "📄"} {r.title || "Untitled"}</ListItemButton></ListItem>)}</List>
        </DialogContent>
      </ModalDialog>
    </Modal>
  );
}

export default function WikiApp({ user }: { user: User }) {
  const [spaces, setSpaces] = useState<Space[]>([]);
  const [spaceId, setSpaceId] = useState("");
  const [pages, setPages] = useState<Page[]>([]);
  const [trashed, setTrashed] = useState<Page[]>([]);
  const [showTrash, setShowTrash] = useState(false);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [pageId, setPageId] = useState(deepParam("page"));
  const [page, setPage] = useState<Page | null>(null);
  const [err, setErr] = useState("");
  const [saveState, setSaveState] = useState<"saved" | "saving" | "dirty" | "conflict">("saved");
  const [side, setSide] = useState<"" | "comments" | "history" | "backlinks">("");
  const [sidebar, setSidebar] = useState(false);
  const [favs, setFavs] = useState<PageRef[]>([]);
  const [recent, setRecent] = useState<PageRef[]>([]);
  const [q, setQ] = useState("");
  const [results, setResults] = useState<PageRef[] | null>(null);
  const [spaceDlg, setSpaceDlg] = useState(false);
  const [picker, setPicker] = useState<"" | "page" | "file">("");
  const [drag, setDrag] = useState("");
  const [comments, setComments] = useState<Comment[]>([]);
  const [versions, setVersions] = useState<Version[]>([]);
  const [backlinks, setBacklinks] = useState<PageRef[]>([]);
  const [text, setText] = useState("");
  const [loadingPage, setLoadingPage] = useState(false);
  const editorRef = useRef<Editor | null>(null);
  const pendingDoc = useRef<unknown>(null);
  const pendingTitle = useRef<string | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout>>();
  const pageRef = useRef<Page | null>(null);
  pageRef.current = page;

  const space = spaces.find((s) => s.id === spaceId);
  const canEdit = (page?.role ?? space?.role ?? 0) >= 2;
  const loadSpaces = useCallback(async () => {
    const l = await wiki.spaces();
    setSpaces(l);
    return l;
  }, []);
  const loadTree = useCallback(async (sid = spaceId) => {
    if (!sid) return;
    const [p, t] = await Promise.all([wiki.tree(sid), wiki.tree(sid, true)]);
    setPages(p);
    setTrashed(t);
  }, [spaceId]);
  const loadSide = useCallback(() => { wiki.favorites().then(setFavs).catch(() => undefined); wiki.recent().then(setRecent).catch(() => undefined); }, []);

  useEffect(() => {
    loadSpaces().then((l) => {
      if (pageId) {
        wiki.page(pageId).then((p) => { setSpaceId(p.space_id); }).catch(() => setSpaceId(l.find((s) => s.kind === "team")?.id ?? l[0]?.id ?? ""));
      } else setSpaceId(l.find((s) => s.kind === "team")?.id ?? l[0]?.id ?? "");
    }).catch((e: Error) => setErr(e.message));
    loadSide();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  useEffect(() => { if (spaceId) void loadTree(spaceId).catch((e: Error) => setErr(e.message)); }, [spaceId, loadTree]);
  // Open the first page of a space when none is selected.
  useEffect(() => {
    if (!pageId && pages.length) setPageId(pages[0].id);
  }, [pages, pageId]);

  const flush = useCallback(async () => {
    clearTimeout(timer.current);
    const cur = pageRef.current;
    if (!cur) return;
    const doc = pendingDoc.current;
    const title = pendingTitle.current;
    if (doc === null && title === null) return;
    pendingDoc.current = null;
    pendingTitle.current = null;
    setSaveState("saving");
    try {
      const saved = await wiki.save(cur.id, { title: title ?? undefined, content: doc ?? undefined, version: cur.version });
      setPage((p) => (p && p.id === saved.id ? { ...p, version: saved.version, title: saved.title, updated_at: saved.updated_at, updated_by: saved.updated_by } : p));
      setPages((l) => l.map((x) => (x.id === saved.id ? { ...x, title: saved.title, icon: saved.icon } : x)));
      setSaveState(pendingDoc.current !== null || pendingTitle.current !== null ? "dirty" : "saved");
    } catch (e) {
      const m = (e as Error).message;
      if (/changed by someone else|conflict/i.test(m)) setSaveState("conflict");
      else { setErr(m); setSaveState("dirty"); pendingDoc.current = pendingDoc.current ?? doc; pendingTitle.current = pendingTitle.current ?? title; }
    }
  }, []);
  const schedule = useCallback(() => { setSaveState("dirty"); clearTimeout(timer.current); timer.current = setTimeout(() => void flush(), 1200); }, [flush]);
  useEffect(() => {
    const h = () => { if (pendingDoc.current !== null || pendingTitle.current !== null) void flush(); };
    window.addEventListener("beforeunload", h);
    return () => window.removeEventListener("beforeunload", h);
  }, [flush]);

  // Load the open page.
  useEffect(() => {
    if (!pageId) { setPage(null); return; }
    let live = true;
    void flush().then(() => {
      setLoadingPage(true);
      wiki.page(pageId).then((p) => { if (live) { setPage(p); setSaveState("saved"); setErr(""); window.history.replaceState(null, "", `/wiki?page=${p.id}`); if (p.space_id !== spaceId) setSpaceId(p.space_id); } })
        .catch((e: Error) => live && setErr(e.message)).finally(() => live && setLoadingPage(false));
    });
    return () => { live = false; };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pageId]);
  useEffect(() => {
    if (!page) return;
    if (side === "comments") wiki.comments(page.id).then(setComments).catch(() => undefined);
    if (side === "history") wiki.versions(page.id).then(setVersions).catch(() => undefined);
    if (side === "backlinks") wiki.backlinks(page.id).then(setBacklinks).catch(() => undefined);
  }, [side, page?.id, page?.version]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (!q.trim()) { setResults(null); return; }
    const t = setTimeout(() => wiki.search(q).then(setResults).catch(() => setResults([])), 250);
    return () => clearTimeout(t);
  }, [q]);

  const tree = useMemo(() => buildTree(pages), [pages]);
  const rows = useMemo(() => visibleRows(tree, expanded), [tree, expanded]);
  const crumbs = page ? breadcrumb(pages, page.id) : [];
  useEffect(() => { if (page?.parent_id) setExpanded((e) => { const n = new Set(e); breadcrumb(pages, page.id).forEach((c) => n.add(c.id)); return n; }); }, [page?.id]); // eslint-disable-line react-hooks/exhaustive-deps
  const aiTools = useMemo(() => createWikiTools({ currentPage: () => pageRef.current, openPage: (id) => setPageId(id), refresh: () => { void loadTree(); if (pageRef.current) void wiki.page(pageRef.current.id).then(setPage); } }), [loadTree]);

  const newPage = async (parent = "", template = "", kind = "") => {
    if (!spaceId) return;
    try {
      const p = await wiki.create({ space_id: spaceId, parent_id: parent || undefined, template: template || undefined, kind: kind || undefined, title: template ? undefined : "Untitled" });
      await loadTree();
      if (parent) setExpanded((e) => new Set(e).add(parent));
      setPageId(p.id);
      setSidebar(false);
    } catch (e) { setErr((e as Error).message); }
  };
  const move = async (id: string, parentId: string, index: number) => {
    if (!canMoveInto(pages, id, parentId)) { setErr("A page cannot move into itself."); return; }
    try { await wiki.move(id, { parent_id: parentId, index }); await loadTree(); if (parentId) setExpanded((e) => new Set(e).add(parentId)); } catch (e) { setErr((e as Error).message); }
  };
  const siblings = (n: TreeNode) => pages.filter((p) => (p.parent_id ?? "") === (n.page.parent_id ?? "")).sort((a, b) => a.position - b.position);
  const trashPage = async (id: string) => {
    if (!window.confirm("Move this page and its sub-pages to the trash?")) return;
    await wiki.trash(id).catch((e: Error) => setErr(e.message));
    if (pageId === id) { setPageId(""); setPage(null); }
    await loadTree();
  };

  const onDoc = useCallback((d: unknown) => { pendingDoc.current = d; schedule(); }, [schedule]);
  const wc = page && page.kind === "page" ? wordCount(page.content) : 0;

  const treeRow = (n: TreeNode) => {
    const p = n.page;
    const has = n.children.length > 0;
    const open = expanded.has(p.id);
    return (
      <Box key={p.id} role="treeitem" aria-expanded={has ? open : undefined} aria-selected={p.id === pageId} aria-level={n.depth + 1}
        draggable={p.role === undefined ? canEdit : canEdit} onDragStart={() => setDrag(p.id)} onDragOver={(e) => e.preventDefault()}
        onDrop={() => { if (drag && drag !== p.id) void move(drag, p.id, 9999); setDrag(""); }}
        sx={{ display: "flex", alignItems: "center", pl: 0.5 + n.depth * 1.6, pr: 0.5, py: 0.25, borderRadius: "sm", bgcolor: p.id === pageId ? "primary.softBg" : "transparent", "&:hover": { bgcolor: p.id === pageId ? "primary.softBg" : "background.level1" }, "&:hover .row-actions": { opacity: 1 } }}>
        <Box component="button" onClick={() => setExpanded((e) => { const x = new Set(e); if (x.has(p.id)) x.delete(p.id); else x.add(p.id); return x; })} aria-label={open ? "Collapse" : "Expand"} sx={{ all: "unset", cursor: has ? "pointer" : "default", width: 18, textAlign: "center", fontSize: 11, visibility: has ? "visible" : "hidden" }}>{open ? "▾" : "▸"}</Box>
        <Box onClick={() => { setPageId(p.id); setSidebar(false); }} sx={{ flex: 1, minWidth: 0, cursor: "pointer", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis", fontSize: 14, py: 0.3 }}>
          {p.icon || (p.kind === "database" ? "🗃️" : "📄")} {p.title || "Untitled"}
        </Box>
        <Box className="row-actions" sx={{ opacity: { xs: 1, md: 0 }, display: "flex" }}>
          {canEdit && p.kind === "page" && <IconButton size="sm" variant="plain" aria-label={`Add a sub-page to ${p.title}`} onClick={() => void newPage(p.id)}>＋</IconButton>}
          <Dropdown>
            <MenuButton slots={{ root: IconButton }} slotProps={{ root: { size: "sm", variant: "plain", "aria-label": `Actions for ${p.title}` } }}>⋯</MenuButton>
            <Menu size="sm" placement="bottom-end" sx={{ zIndex: 2000 }}>
              <MenuItem onClick={() => void wiki.favorite(p.id, !p.favorite).then(() => { void loadTree(); loadSide(); })}>{p.favorite ? "Remove from favorites" : "Add to favorites"}</MenuItem>
              <MenuItem onClick={() => { void navigator.clipboard?.writeText(location.origin + pageLink(p.id)); }}>Copy link</MenuItem>
              {canEdit && <MenuItem onClick={() => void wiki.duplicate(p.id).then((d) => { void loadTree(); setPageId(d.id); })}>Duplicate</MenuItem>}
              {canEdit && <MenuItem disabled={siblings(n)[0]?.id === p.id} onClick={() => { const s = siblings(n); void move(p.id, p.parent_id ?? "", Math.max(0, s.findIndex((x) => x.id === p.id) - 1)); }}>Move up</MenuItem>}
              {canEdit && <MenuItem disabled={siblings(n).slice(-1)[0]?.id === p.id} onClick={() => { const s = siblings(n); void move(p.id, p.parent_id ?? "", s.findIndex((x) => x.id === p.id) + 1); }}>Move down</MenuItem>}
              {canEdit && p.parent_id && <MenuItem onClick={() => void move(p.id, "", 9999)}>Move to top level</MenuItem>}
              {canEdit && <MenuItem color="danger" onClick={() => void trashPage(p.id)}>Move to trash</MenuItem>}
            </Menu>
          </Dropdown>
        </Box>
      </Box>
    );
  };

  const sidebarEl = (
    <Box component="nav" aria-label="Pages" sx={{ width: { xs: "100%", md: 290 }, flexShrink: 0, borderRight: { md: "1px solid" }, borderColor: "divider", p: 1, overflow: "auto", maxHeight: { md: "calc(100vh - 140px)" } }} data-testid="wiki-sidebar">
      <Box sx={{ display: "flex", gap: 0.5, mb: 1 }}>
        <Select size="sm" value={spaceId} onChange={(_, v) => { if (v) { setSpaceId(v); setPageId(""); setPage(null); } }} sx={{ flex: 1 }} slotProps={{ listbox: { sx: { zIndex: 2000 } } }} aria-label="Space">
          {spaces.map((s) => <Option key={s.id} value={s.id}>{s.icon || (s.kind === "personal" ? "📝" : "🗂️")} {s.name}</Option>)}
        </Select>
        <IconButton size="sm" variant="outlined" color="neutral" aria-label="Space settings" onClick={() => setSpaceDlg(true)}>⚙</IconButton>
        <IconButton size="sm" variant="outlined" color="neutral" aria-label="New space" onClick={() => { const n = window.prompt("Name of the new shared space"); if (n?.trim()) void wiki.createSpace({ name: n.trim() }).then(async (s) => { await loadSpaces(); setSpaceId(s.id); setPageId(""); }).catch((e: Error) => setErr(e.message)); }}>＋</IconButton>
      </Box>
      <Input size="sm" placeholder="Search all pages" value={q} onChange={(e) => setQ(e.target.value)} sx={{ mb: 1 }} slotProps={{ input: { "aria-label": "Search pages" } }} />
      {results ? (
        <Box>
          {results.length === 0 && <Typography level="body-sm" sx={{ p: 1 }}>No pages match.</Typography>}
          {results.map((r) => (
            <Box key={r.id} onClick={() => { setPageId(r.id); setQ(""); setSidebar(false); }} sx={{ p: 0.8, cursor: "pointer", borderRadius: "sm", "&:hover": { bgcolor: "background.level1" } }}>
              <Typography level="title-sm">{r.icon || "📄"} {r.title || "Untitled"}</Typography>
              <Typography level="body-xs">{r.space_name}</Typography>
              {r.snippet && <Typography level="body-xs" sx={{ color: "text.secondary" }}>{r.snippet.replace(/[«»]/g, "")}</Typography>}
            </Box>
          ))}
        </Box>
      ) : (
        <>
          {favs.length > 0 && <><Typography level="body-xs" sx={{ px: 0.5, mt: 1, fontWeight: 700 }}>FAVORITES</Typography>{favs.map((f) => <Box key={f.id} onClick={() => { setPageId(f.id); setSidebar(false); }} sx={{ px: 0.8, py: 0.3, cursor: "pointer", fontSize: 14, borderRadius: "sm", "&:hover": { bgcolor: "background.level1" } }}>⭐ {f.title || "Untitled"}</Box>)}</>}
          <Box sx={{ display: "flex", alignItems: "center", mt: 1.5, px: 0.5 }}>
            <Typography level="body-xs" sx={{ fontWeight: 700, flex: 1 }}>PAGES</Typography>
            {space && space.role >= 2 && (
              <Dropdown>
                <MenuButton size="sm" variant="plain" data-testid="new-page-menu" aria-label="New page">＋ New ▾</MenuButton>
                <Menu size="sm" sx={{ zIndex: 2000 }}>
                  <MenuItem onClick={() => void newPage()} data-testid="new-page-blank">Blank page</MenuItem>
                  <MenuItem onClick={() => void newPage("", "meeting_notes")}>Meeting notes</MenuItem>
                  <MenuItem onClick={() => void newPage("", "project_brief")}>Project brief</MenuItem>
                  <MenuItem onClick={() => void newPage("", "weekly_plan")}>Weekly plan</MenuItem>
                  <MenuItem onClick={() => void newPage("", "reading_list")}>Reading list</MenuItem>
                  <MenuItem onClick={() => void newPage("", "task_board")}>Task board (database)</MenuItem>
                  <MenuItem onClick={() => void newPage("", "", "database")}>Empty database</MenuItem>
                  <MenuItem component="label">Import Markdown…<input hidden type="file" accept=".md,.markdown,.txt,text/markdown" onChange={(e) => { const f = e.target.files?.[0]; if (f) void f.text().then((t) => wiki.importMd(spaceId, t)).then(async (p) => { await loadTree(); setPageId(p.id); }).catch((er: Error) => setErr(er.message)); }} /></MenuItem>
                </Menu>
              </Dropdown>
            )}
          </Box>
          <Box role="tree" aria-label="Page tree" onDragOver={(e) => e.preventDefault()} onDrop={(e) => { if (e.target === e.currentTarget && drag) { void move(drag, "", 9999); setDrag(""); } }} sx={{ minHeight: 40 }}>
            {rows.map(treeRow)}
            {rows.length === 0 && <Typography level="body-sm" sx={{ p: 1 }}>No pages yet.</Typography>}
          </Box>
          {recent.length > 0 && <><Typography level="body-xs" sx={{ px: 0.5, mt: 1.5, fontWeight: 700 }}>RECENT</Typography>{recent.slice(0, 6).map((f) => <Box key={f.id} onClick={() => { setPageId(f.id); setSidebar(false); }} sx={{ px: 0.8, py: 0.3, cursor: "pointer", fontSize: 13, color: "text.secondary", borderRadius: "sm", "&:hover": { bgcolor: "background.level1" } }}>{f.icon || "📄"} {f.title || "Untitled"}</Box>)}</>}
          <Button size="sm" variant="plain" color="neutral" sx={{ mt: 1.5 }} onClick={() => setShowTrash(!showTrash)}>🗑 Trash ({trashed.length})</Button>
          {showTrash && trashed.map((t) => (
            <Box key={t.id} sx={{ display: "flex", gap: 0.5, alignItems: "center", px: 0.8, fontSize: 13 }}>
              <Box sx={{ flex: 1, textDecoration: "line-through" }}>{t.title || "Untitled"}</Box>
              <Button size="sm" variant="plain" onClick={() => void wiki.restore(t.id).then(() => loadTree())}>Restore</Button>
              <Button size="sm" variant="plain" color="danger" onClick={() => { if (window.confirm(`Delete "${t.title}" forever?`)) void wiki.remove(t.id).then(() => loadTree()); }}>Delete</Button>
            </Box>
          ))}
        </>
      )}
    </Box>
  );

  return (
    <>
      <Header user={user} />
      <Container maxWidth={false} sx={{ py: 1.5, px: { xs: 1, sm: 2 } }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, mb: 1 }}>
          <MenuBook sx={{ fontSize: 28, color: ACCENT }} />
          <Typography level="h2" sx={{ fontSize: { xs: "xl", sm: "xl3" } }}>Wiki</Typography>
          <Button size="sm" variant="outlined" color="neutral" sx={{ display: { md: "none" }, ml: 1 }} onClick={() => setSidebar(!sidebar)} aria-expanded={sidebar} data-testid="wiki-pages-toggle">☰ Pages</Button>
          <Box sx={{ ml: "auto" }}><BizAssistant app="wiki" title="Wiki" tools={aiTools} suggestions={WIKI_SUGGESTIONS} systemHint={WIKI_HINT} /></Box>
        </Box>
        {err && <Box sx={{ p: 1, mb: 1, bgcolor: "danger.softBg", borderRadius: "sm", fontSize: 13 }} role="alert" onClick={() => setErr("")}>{err}</Box>}
        <Box sx={{ display: "flex", flexDirection: { xs: "column", md: "row" }, gap: { md: 2 } }}>
          <Box sx={{ display: { xs: sidebar ? "block" : "none", md: "block" } }}>{sidebarEl}</Box>
          <Box sx={{ flex: 1, minWidth: 0, display: "flex", gap: 2 }}>
            <Box sx={{ flex: 1, minWidth: 0 }} data-testid="wiki-page">
              {loadingPage && !page && <CircularProgress />}
              {!page && !loadingPage && <Box sx={{ p: 4, color: "text.secondary" }}>{space && space.role >= 2 ? "Create your first page with “New” in the sidebar." : "Pick a page from the sidebar."}</Box>}
              {page && (
                <Box key={page.id}>
                  <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, fontSize: 13, color: "text.tertiary", flexWrap: "wrap", mb: 0.5 }}>
                    {crumbs.map((c) => <Box key={c.id} component="span"><Box component="a" onClick={() => setPageId(c.id)} sx={{ cursor: "pointer", color: "inherit" }}>{c.title || "Untitled"}</Box> /</Box>)}
                    <Box sx={{ flex: 1 }} />
                    <Typography level="body-xs" data-testid="save-state" role="status">{saveState === "saved" ? "Saved" : saveState === "saving" ? "Saving…" : saveState === "dirty" ? "Unsaved changes" : "Conflict"}</Typography>
                    <Button size="sm" variant={side === "comments" ? "soft" : "plain"} color="neutral" onClick={() => setSide(side === "comments" ? "" : "comments")}>💬 Comments</Button>
                    <Button size="sm" variant={side === "history" ? "soft" : "plain"} color="neutral" onClick={() => setSide(side === "history" ? "" : "history")}>🕘 History</Button>
                    <Button size="sm" variant={side === "backlinks" ? "soft" : "plain"} color="neutral" onClick={() => setSide(side === "backlinks" ? "" : "backlinks")}>↩ Backlinks</Button>
                    <Dropdown>
                      <MenuButton size="sm" variant="plain" color="neutral" aria-label="Page actions">⋯</MenuButton>
                      <Menu size="sm" placement="bottom-end" sx={{ zIndex: 2000 }}>
                        <MenuItem onClick={() => void wiki.favorite(page.id, !page.favorite).then(() => { setPage({ ...page, favorite: !page.favorite }); loadSide(); })}>{page.favorite ? "Remove from favorites" : "Add to favorites"}</MenuItem>
                        <MenuItem onClick={() => void navigator.clipboard?.writeText(location.origin + pageLink(page.id))}>Copy link</MenuItem>
                        <MenuItem component="a" href={wiki.exportUrl(page.id, "md")} download>Export as Markdown</MenuItem>
                        <MenuItem component="a" href={wiki.exportUrl(page.id, "html")} target="_blank" rel="noreferrer">Open printable version (save as PDF)</MenuItem>
                      </Menu>
                    </Dropdown>
                  </Box>
                  {saveState === "conflict" && (
                    <Box sx={{ p: 1, mb: 1, bgcolor: "warning.softBg", borderRadius: "sm" }} role="alert">
                      Someone else changed this page. <Button size="sm" onClick={() => { pendingDoc.current = null; pendingTitle.current = null; void wiki.page(page.id).then((p) => { setPage(p); setSaveState("saved"); }); }}>Reload their version</Button> (your unsaved edits are discarded)
                    </Box>
                  )}
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1 }}>
                    <Dropdown>
                      <MenuButton size="lg" variant="plain" disabled={!canEdit} aria-label="Page icon" sx={{ fontSize: 32, p: 0.5 }}>{page.icon || (page.kind === "database" ? "🗃️" : "📄")}</MenuButton>
                      <Menu size="sm" sx={{ zIndex: 2000, display: "grid", gridTemplateColumns: "repeat(7, 1fr)" }}>
                        {EMOJI.map((e) => <MenuItem key={e} onClick={() => { setPage({ ...page, icon: e }); void wiki.save(page.id, { icon: e }).then((p) => { setPages((l) => l.map((x) => (x.id === p.id ? { ...x, icon: e } : x))); }); }}>{e}</MenuItem>)}
                      </Menu>
                    </Dropdown>
                    <Input variant="plain" disabled={!canEdit} value={page.title} placeholder="Untitled" onChange={(e) => { setPage({ ...page, title: e.target.value }); pendingTitle.current = e.target.value; schedule(); }}
                      sx={{ flex: 1, fontSize: { xs: 26, sm: 34 }, fontWeight: 700, "--Input-focusedThickness": "0px", bgcolor: "transparent" }} slotProps={{ input: { "aria-label": "Page title", "data-testid": "page-title" } }} />
                  </Box>
                  {page.kind === "database" && page.records_base_id ? (
                    <Workspace baseId={page.records_base_id} embedded initialTable={page.records_table_id} />
                  ) : (
                    <WikiEditor content={page.content} editable={canEdit} onChange={onDoc} onReady={(e) => { editorRef.current = e; }} onPickPage={() => setPicker("page")} onAttach={() => setPicker("file")} />
                  )}
                  <Typography level="body-xs" sx={{ mt: 1, color: "text.tertiary" }}>{wc > 0 ? `${wc} words · ` : ""}Last edited {new Date(page.updated_at).toLocaleString()}{page.updated_by ? ` by ${page.updated_by}` : ""}{!canEdit ? " · View only" : ""}</Typography>
                </Box>
              )}
            </Box>
            {side && page && (
              <Box sx={{ width: { xs: "100%", md: 300 }, flexShrink: 0, borderLeft: { md: "1px solid" }, borderColor: "divider", pl: { md: 2 }, position: { xs: "fixed", md: "static" }, inset: { xs: "60px 0 0 0", md: "auto" }, bgcolor: "background.surface", zIndex: { xs: 1200, md: "auto" }, p: { xs: 2, md: 0 }, overflow: "auto" }} data-testid="wiki-side">
                <Box sx={{ display: "flex", mb: 1 }}><Typography level="title-md" sx={{ flex: 1 }}>{side === "comments" ? "Comments" : side === "history" ? "Page history" : "Linked from"}</Typography><IconButton size="sm" variant="plain" aria-label="Close panel" onClick={() => setSide("")}>✕</IconButton></Box>
                {side === "comments" && (
                  <>
                    {comments.map((c) => (
                      <Box key={c.id} sx={{ mb: 1.2, opacity: c.resolved ? 0.55 : 1 }}>
                        <Typography level="body-xs"><b>{c.author}</b> · {new Date(c.created_at).toLocaleString()}</Typography>
                        <Typography level="body-sm" sx={{ whiteSpace: "pre-wrap" }}>{c.body}</Typography>
                        <Button size="sm" variant="plain" onClick={() => void wiki.resolve(c.id, !c.resolved).then(() => wiki.comments(page.id).then(setComments))}>{c.resolved ? "Reopen" : "Resolve"}</Button>
                        <Button size="sm" variant="plain" color="danger" onClick={() => void wiki.delComment(c.id).then(() => wiki.comments(page.id).then(setComments)).catch((e: Error) => setErr(e.message))}>Delete</Button>
                      </Box>
                    ))}
                    {comments.length === 0 && <Typography level="body-sm">No comments yet.</Typography>}
                    <Textarea minRows={2} value={text} onChange={(e) => setText(e.target.value)} placeholder="Write a comment" sx={{ mt: 1 }} slotProps={{ textarea: { "aria-label": "New comment" } }} />
                    <Button size="sm" sx={{ mt: 0.5 }} disabled={!text.trim()} onClick={() => void wiki.addComment(page.id, text).then(() => { setText(""); return wiki.comments(page.id).then(setComments); }).catch((e: Error) => setErr(e.message))}>Comment</Button>
                  </>
                )}
                {side === "history" && (
                  <>
                    {versions.map((v) => (
                      <Box key={v.id} sx={{ mb: 1 }}>
                        <Typography level="body-sm">{new Date(v.saved_at).toLocaleString()}</Typography>
                        <Typography level="body-xs">{v.saved_by} · {v.title || "Untitled"}</Typography>
                        {canEdit && <Button size="sm" variant="soft" onClick={() => { if (window.confirm("Restore this version? The current content is kept in history.")) void wiki.restoreVersion(page.id, v.id).then((p) => { setPage(p); setPageId(""); setTimeout(() => setPageId(p.id), 0); }).catch((e: Error) => setErr(e.message)); }}>Restore</Button>}
                      </Box>
                    ))}
                    {versions.length === 0 && <Typography level="body-sm">History appears after the page is edited.</Typography>}
                  </>
                )}
                {side === "backlinks" && (
                  <>
                    {backlinks.map((b) => <Box key={b.id} onClick={() => setPageId(b.id)} sx={{ cursor: "pointer", p: 0.6, borderRadius: "sm", "&:hover": { bgcolor: "background.level1" } }}>{b.icon || "📄"} {b.title || "Untitled"}</Box>)}
                    {backlinks.length === 0 && <Typography level="body-sm">No page links here yet. Use “@ Page” in the editor.</Typography>}
                  </>
                )}
              </Box>
            )}
          </Box>
        </Box>
      </Container>
      {spaceDlg && space && <SpaceDialog space={space} onClose={() => setSpaceDlg(false)} onSaved={() => { setSpaceDlg(false); void loadSpaces().then((l) => { if (!l.find((s) => s.id === spaceId)) { setSpaceId(l[0]?.id ?? ""); setPageId(""); } }); }} />}
      {picker === "page" && <PagePicker spaceId={spaceId} onClose={() => setPicker("")} onPick={(p) => { setPicker(""); editorRef.current?.chain().focus().insertContent({ type: "text", text: p.title || "Untitled", marks: [{ type: "link", attrs: { href: pageLink(p.id) } }] }).run(); }} />}
      {picker === "file" && <DrivePicker mode="file" title="Insert a Drive file" confirmLabel="Insert" onClose={() => setPicker("")} onPick={(f) => { setPicker(""); const d = f as { id: string; name: string }; editorRef.current?.chain().focus().insertContent({ type: "text", text: `📎 ${d.name}`, marks: [{ type: "link", attrs: { href: `/drive?file=${d.id}` } }] }).run(); }} />}
    </>
  );
}
