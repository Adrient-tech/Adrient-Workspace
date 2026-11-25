import { useEffect, useMemo, useState } from "react";
import { Alert, Box, Button, Checkbox, Chip, CircularProgress, FormControl, FormLabel, Input, Sheet, Textarea, Typography } from "@mui/joy";
import AddIcon from "@mui/icons-material/Add";
import { svc, type Article } from "./svcApi";
import { RichText } from "./TicketPane";
import { EmptyState } from "../../components/ui/EmptyState";

/** Knowledge base manager: categories, article editor with preview, reader feedback and deflection stats. */
export function DeskKb({ pid, publicToken, onClose }: { pid: string; publicToken?: string; onClose: () => void }) {
  const [list, setList] = useState<Article[] | null>(null);
  const [stats, setStats] = useState<Awaited<ReturnType<typeof svc.kbStats>> | null>(null);
  const [edit, setEdit] = useState<Partial<Article> | null>(null);
  const [q, setQ] = useState("");
  const [err, setErr] = useState("");
  const load = () => { svc.articles(pid).then(setList).catch((e) => setErr((e as Error).message)); svc.kbStats(pid).then(setStats).catch(() => undefined); };
  useEffect(load, [pid]); // eslint-disable-line react-hooks/exhaustive-deps
  const grouped = useMemo(() => {
    const m = new Map<string, Article[]>();
    for (const a of (list ?? []).filter((x) => !q || (x.title + x.body).toLowerCase().includes(q.toLowerCase()))) m.set(a.category || "Uncategorised", [...(m.get(a.category || "Uncategorised") ?? []), a]);
    return [...m.entries()].sort((a, b) => a[0].localeCompare(b[0]));
  }, [list, q]);
  const cats = useMemo(() => [...new Set((list ?? []).map((a) => a.category).filter(Boolean))], [list]);

  async function save() {
    if (!edit) return;
    setErr("");
    try { await svc.saveArticle(pid, edit); setEdit(null); load(); } catch (e) { setErr((e as Error).message); }
  }
  return (
    <Box sx={{ p: { xs: 2, md: 3 }, height: "calc(100vh - 56px)", overflow: "auto" }} data-testid="desk-kb">
      <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, flexWrap: "wrap", mb: 2 }}>
        <Button size="sm" variant="plain" color="neutral" onClick={onClose}>← Back to tickets</Button>
        <Typography level="h3" sx={{ flex: 1 }}>Knowledge base</Typography>
        {publicToken && <Button size="sm" variant="outlined" component="a" href={`/help/${publicToken}`} target="_blank" rel="noreferrer" data-testid="help-link">View help centre</Button>}
        <Button size="sm" startDecorator={<AddIcon />} onClick={() => setEdit({ title: "", body: "", published: true, category: "" })} data-testid="kb-new">New article</Button>
      </Box>
      {stats && (
        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr 1fr", md: "repeat(5, 1fr)" }, gap: 1.5, mb: 2 }}>
          {[["Articles", stats.articles], ["Published", stats.published], ["Views", stats.views], ["Helpful votes", `${stats.helpful_yes} yes / ${stats.helpful_no} no`], ["Tickets deflected", stats.deflected]].map(([l, v]) => (
            <Sheet key={String(l)} variant="outlined" sx={{ borderRadius: "lg", p: 1.5 }}><Typography level="body-xs" sx={{ color: "text.tertiary", textTransform: "uppercase" }}>{l}</Typography><Typography level="title-lg">{v}</Typography></Sheet>
          ))}
        </Box>
      )}
      {!publicToken && <Alert color="neutral" size="sm" sx={{ mb: 1.5 }}>Turn on the public link for this project to publish a help centre address. Until then, articles are used for suggestions inside the desk only.</Alert>}
      <Input size="sm" placeholder="Search articles" value={q} onChange={(e) => setQ(e.target.value)} sx={{ maxWidth: 360, mb: 2 }} />
      {err && <Alert color="danger" sx={{ mb: 1 }}>{err}</Alert>}
      {!list ? <CircularProgress /> : list.length === 0 ? <EmptyState illustration="files" title="No articles yet" hint="Write the answers to your most common questions. Requesters see them while typing a ticket." /> : (
        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", lg: "1fr 1fr" }, gap: 2 }}>
          {grouped.map(([cat, items]) => (
            <Sheet key={cat} variant="outlined" sx={{ borderRadius: "lg", overflow: "hidden" }} data-testid={`kb-cat-${cat}`}>
              <Box sx={{ px: 2, py: 1, bgcolor: "background.level1", display: "flex", gap: 1 }}><Typography level="title-sm" sx={{ flex: 1 }}>{cat}</Typography><Typography level="body-xs" sx={{ color: "text.tertiary" }}>{items.length}</Typography></Box>
              {items.map((a) => (
                <Box key={a.id} onClick={() => setEdit(a)} data-testid="kb-row" sx={{ px: 2, py: 1.1, borderTop: "1px solid", borderColor: "divider", cursor: "pointer", display: "flex", gap: 1, alignItems: "center", "&:hover": { bgcolor: "neutral.softHoverBg" } }}>
                  <Box sx={{ flex: 1, minWidth: 0 }}>
                    <Typography level="title-sm" noWrap>{a.title}</Typography>
                    <Typography level="body-xs" sx={{ color: "text.tertiary" }} noWrap>{a.body.slice(0, 90)}</Typography>
                  </Box>
                  <Chip size="sm" variant="soft" color={a.published ? "success" : "neutral"}>{a.published ? "Published" : "Draft"}</Chip>
                  <Typography level="body-xs" sx={{ color: "text.tertiary", width: 56, textAlign: "right" }}>{a.views} views</Typography>
                </Box>
              ))}
            </Sheet>
          ))}
        </Box>
      )}
      {edit && (
        <Sheet sx={{ position: "fixed", inset: 0, zIndex: 1200, bgcolor: "background.backdrop", display: "grid", placeItems: "center", p: 2 }} onClick={() => setEdit(null)}>
          <Sheet variant="outlined" onClick={(e) => e.stopPropagation()} sx={{ width: "min(980px, 98vw)", maxHeight: "94vh", overflow: "auto", borderRadius: "lg", p: 2.5 }} data-testid="kb-editor">
            <Typography level="title-lg" sx={{ mb: 1.5 }}>{edit.id ? "Edit article" : "New article"}</Typography>
            <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1fr 1fr" }, gap: 2 }}>
              <Box sx={{ display: "flex", flexDirection: "column", gap: 1.25 }}>
                <FormControl size="sm"><FormLabel>Title</FormLabel><Input value={edit.title ?? ""} onChange={(e) => setEdit({ ...edit, title: e.target.value })} slotProps={{ input: { "data-testid": "kb-title" } as never }} /></FormControl>
                <FormControl size="sm"><FormLabel>Category</FormLabel>
                  <Input value={edit.category ?? ""} onChange={(e) => setEdit({ ...edit, category: e.target.value })} placeholder="Accounts, Network, Devices…" slotProps={{ input: { list: "kb-cats" } as never }} />
                  <datalist id="kb-cats">{cats.map((c) => <option key={c} value={c} />)}</datalist>
                </FormControl>
                <FormControl size="sm"><FormLabel>Answer</FormLabel><Textarea minRows={10} value={edit.body ?? ""} onChange={(e) => setEdit({ ...edit, body: e.target.value })} placeholder="Use **bold**, _italic_, lines starting with - for lists and [text](https://link)." slotProps={{ textarea: { "data-testid": "kb-body" } as never }} /></FormControl>
                <Checkbox size="sm" label="Published on the help centre" checked={!!edit.published} onChange={(e) => setEdit({ ...edit, published: e.target.checked })} data-testid="kb-published" />
              </Box>
              <Sheet variant="soft" sx={{ borderRadius: "md", p: 2 }}>
                <Typography level="body-xs" sx={{ color: "text.tertiary", mb: 0.5 }}>Preview</Typography>
                <Typography level="title-md">{edit.title || "Untitled"}</Typography>
                <RichText text={edit.body || "The answer shows here."} />
              </Sheet>
            </Box>
            {err && <Alert color="danger" size="sm" sx={{ mt: 1 }}>{err}</Alert>}
            <Box sx={{ display: "flex", justifyContent: "space-between", mt: 2 }}>
              <Box>{edit.id && <Button variant="plain" color="danger" onClick={async () => { if (window.confirm(`Delete "${edit.title}"?`)) { await svc.deleteArticle(edit.id!).catch((e) => setErr((e as Error).message)); setEdit(null); load(); } }}>Delete</Button>}</Box>
              <Box sx={{ display: "flex", gap: 1 }}><Button variant="plain" color="neutral" onClick={() => setEdit(null)}>Cancel</Button><Button onClick={save} disabled={!(edit.title ?? "").trim()} data-testid="kb-save">Save article</Button></Box>
            </Box>
          </Sheet>
        </Sheet>
      )}
    </Box>
  );
}
