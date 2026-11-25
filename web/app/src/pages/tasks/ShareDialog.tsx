import { useCallback, useEffect, useState } from "react";
import { Alert, Box, Button, IconButton, Input, Modal, ModalClose, ModalDialog, Option, Select, Typography } from "@mui/joy";
import DeleteIcon from "@mui/icons-material/DeleteOutline";
import { addShare, createPublicLink, getFeed, getPublicLink, getShares, removeShare, revokePublicLink, rotateFeed, type Share } from "./extrasApi";

/** Share a list with people in your organisation (viewer or editor). */
export function ShareDialog({ listId, listName, onClose }: { listId: string; listName: string; onClose: () => void }) {
  const [shares, setShares] = useState<Share[] | null>(null);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("editor");
  const [err, setErr] = useState("");
  const load = useCallback(() => getShares(listId).then(setShares).catch((e: Error) => { setErr(e.message); setShares([]); }), [listId]);
  useEffect(() => { void load(); }, [load]);
  const [pub, setPub] = useState("");
  const [feed, setFeed] = useState("");
  const [copied, setCopied] = useState("");
  useEffect(() => {
    getPublicLink(listId).then((l) => setPub(l.path)).catch(() => setPub(""));
    getFeed().then((f) => setFeed(f.path)).catch(() => setFeed(""));
  }, [listId]);
  const abs = (p: string) => (p ? `${window.location.origin}${p}` : "");
  async function copy(what: string, text: string) {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(what);
      setTimeout(() => setCopied(""), 1500);
    } catch {
      setErr("Could not copy. Select the link and copy it by hand.");
    }
  }

  async function add() {
    if (!email.trim()) return;
    setErr("");
    try {
      await addShare(listId, email.trim(), role);
      setEmail("");
      await load();
    } catch (e) {
      setErr((e as Error).message);
    }
  }

  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: "min(480px, 96vw)" }} data-testid="share-dialog">
        <ModalClose />
        <Typography level="title-lg">Share "{listName}"</Typography>
        <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>
          <Input sx={{ flex: 1, minWidth: 180 }} placeholder="Email of a colleague" value={email} onChange={(e) => setEmail(e.target.value)} onKeyDown={(e) => e.key === "Enter" && void add()} slotProps={{ input: { "data-testid": "share-email" } }} />
          <Select value={role} onChange={(_, v) => setRole(v ?? "editor")}>
            <Option value="editor">Can edit</Option>
            <Option value="viewer">Can view</Option>
          </Select>
          <Button onClick={add} data-testid="share-add">Share</Button>
        </Box>
        {err && <Alert color="danger">{err}</Alert>}
        {shares?.length === 0 && <Typography level="body-sm">Not shared with anyone yet.</Typography>}
        {shares?.map((s) => (
          <Box key={s.user_id} sx={{ display: "flex", alignItems: "center", gap: 1 }} data-testid="share-row">
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Typography level="body-sm" noWrap>{s.name || s.email}</Typography>
              <Typography level="body-xs" noWrap>{s.email} · {s.role === "viewer" ? "Can view" : "Can edit"}</Typography>
            </Box>
            <IconButton size="sm" variant="plain" color="danger" aria-label={`Stop sharing with ${s.email}`} onClick={async () => { await removeShare(listId, s.user_id).catch((e: Error) => setErr(e.message)); await load(); }}>
              <DeleteIcon fontSize="small" />
            </IconButton>
          </Box>
        ))}
        <Typography level="title-sm" sx={{ mt: 1 }}>Public read-only link</Typography>
        <Typography level="body-xs">Anyone with the link can view this list, without signing in. They cannot change it. You can turn it off at any time.</Typography>
        {pub ? (
          <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }} data-testid="public-link-row">
            <Input readOnly sx={{ flex: 1, minWidth: 200 }} value={abs(pub)} slotProps={{ input: { "data-testid": "public-link" } }} />
            <Button size="sm" variant="outlined" onClick={() => void copy("pub", abs(pub))}>{copied === "pub" ? "Copied" : "Copy"}</Button>
            <Button size="sm" variant="outlined" color="neutral" onClick={async () => { try { setPub((await createPublicLink(listId, true)).path); } catch (e) { setErr((e as Error).message); } }}>New link</Button>
            <Button size="sm" variant="outlined" color="danger" data-testid="public-link-off" onClick={async () => { try { await revokePublicLink(listId); setPub(""); } catch (e) { setErr((e as Error).message); } }}>Turn off</Button>
          </Box>
        ) : (
          <Box><Button size="sm" variant="outlined" data-testid="public-link-on" onClick={async () => { try { setPub((await createPublicLink(listId)).path); } catch (e) { setErr((e as Error).message); } }}>Create public link</Button></Box>
        )}
        <Typography level="title-sm" sx={{ mt: 1 }}>Calendar subscription</Typography>
        <Typography level="body-xs">Subscribe in Google, Apple or Outlook Calendar to see your open dated tasks. Keep this link private: it shows all your tasks.</Typography>
        {feed && (
          <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }} data-testid="feed-row">
            <Input readOnly sx={{ flex: 1, minWidth: 200 }} value={abs(feed)} slotProps={{ input: { "data-testid": "feed-link" } }} />
            <Button size="sm" variant="outlined" onClick={() => void copy("feed", abs(feed))}>{copied === "feed" ? "Copied" : "Copy"}</Button>
            <Button size="sm" variant="outlined" color="neutral" onClick={async () => { try { setFeed((await rotateFeed()).path); } catch (e) { setErr((e as Error).message); } }}>New link</Button>
          </Box>
        )}
      </ModalDialog>
    </Modal>
  );
}
