import { useEffect, useMemo, useState } from "react";
import { Box, Typography, Button, IconButton, Modal, ModalDialog, ModalClose, DialogTitle, Select, Option, FormControl, FormLabel, Chip, CircularProgress, Dropdown, Menu, MenuButton, MenuItem } from "@mui/joy";
import AddRoundedIcon from "@mui/icons-material/AddRounded";
import LockOutlinedIcon from "@mui/icons-material/LockOutlined";
import PublicRoundedIcon from "@mui/icons-material/PublicRounded";
import GroupsRoundedIcon from "@mui/icons-material/GroupsRounded";
import DragIndicatorRoundedIcon from "@mui/icons-material/DragIndicatorRounded";
import MoreHorizRoundedIcon from "@mui/icons-material/MoreHorizRounded";
import PlaylistRemoveIcon from "@mui/icons-material/PlaylistRemove";
import { useNavigate, useParams } from "react-router-dom";
import { useLibrary } from "./library";
import { usePlayer } from "./player";
import { deletePlaylist, getPlaylistFull, listGroups, removeTrackFromPlaylist, reorderPlaylist, setPlaylistSharing, updatePlaylist } from "./api";
import { albumKey, artistOf, humanDuration, totalDuration } from "./logic";
import { Cover, coverColors, EmptyNote, MediaCard, PageTitle, SongRow, Shelf } from "./ui";
import { PlaylistFormDialog } from "./dialogs";
import { PlayShuffle, SMART_KINDS } from "./LibraryPage";
import { useDragReorder } from "./dnd";
import type { GroupRef, PlaylistInfo, Track, Visibility } from "./types";

const VIS: Record<Visibility, { label: string; icon: JSX.Element; hint: string }> = {
  private: { label: "Private", icon: <LockOutlinedIcon fontSize="small" />, hint: "Only you" },
  org: { label: "Organisation", icon: <PublicRoundedIcon fontSize="small" />, hint: "Everyone in your organisation can see it" },
  group: { label: "Group", icon: <GroupsRoundedIcon fontSize="small" />, hint: "Members of one group" },
};

export function VisibilityChip({ pl }: { pl: Pick<PlaylistInfo, "visibility" | "group_name"> }) {
  const v = VIS[pl.visibility] ?? VIS.org;
  return (
    <Chip size="sm" variant="soft" color={pl.visibility === "private" ? "neutral" : "primary"} startDecorator={v.icon} data-testid="visibility-chip">
      {pl.visibility === "group" && pl.group_name ? `Group: ${pl.group_name}` : v.label}
    </Chip>
  );
}

function PlaylistCover({ pl, size }: { pl: PlaylistInfo; size: number }) {
  const arts = pl.cover_art;
  if (arts.length >= 4) {
    return (
      <Box sx={{ width: size, height: size, borderRadius: "12px", overflow: "hidden", display: "grid", gridTemplateColumns: "1fr 1fr", flexShrink: 0 }}>
        {arts.slice(0, 4).map((a) => <img key={a} src={a} alt="" style={{ width: "100%", height: "100%", objectFit: "cover", display: "block" }} />)}
      </Box>
    );
  }
  return <Cover url={arts[0]} name={pl.name} size={size} radius={12} />;
}

/** PlaylistsPage: your and shared playlists, plus the smart ones. */
export function PlaylistsPage() {
  const lib = useLibrary();
  const nav = useNavigate();
  const [creating, setCreating] = useState(false);
  useEffect(() => void lib.refreshPlaylists(), []); // eslint-disable-line react-hooks/exhaustive-deps
  const pls = lib.playlists;
  const mine = (pls ?? []).filter((p) => p.mine);
  const shared = (pls ?? []).filter((p) => !p.mine);
  const card = (p: PlaylistInfo) => (
    <Box key={p.id} data-testid={`playlist-${p.id}`} onClick={() => nav(`/music/playlists/${p.id}`)} sx={{ width: 168, cursor: "pointer" }}>
      <PlaylistCover pl={p} size={168} />
      <Typography level="title-sm" noWrap sx={{ mt: 1, fontWeight: 600 }}>{p.name || "Untitled playlist"}</Typography>
      <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, color: "text.tertiary" }}>
        {VIS[p.visibility]?.icon}
        <Typography level="body-xs" noWrap>{p.track_count} songs{p.mine ? "" : ` · ${p.owner_name}`}</Typography>
      </Box>
    </Box>
  );
  return (
    <Box data-testid="music-playlists">
      <PageTitle title="Playlists" sub="Yours, and the ones shared with you" action={<Button startDecorator={<AddRoundedIcon />} onClick={() => setCreating(true)} data-testid="new-playlist">New playlist</Button>} />
      <Typography level="title-lg" sx={{ mb: 1.5 }}>Smart playlists</Typography>
      <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1, mb: 4 }}>
        {SMART_KINDS.map((s) => (
          <Chip key={s.kind} size="lg" variant="outlined" color="neutral" onClick={() => nav(`/music/smart/${s.kind}`)} sx={{ cursor: "pointer", "--Chip-radius": "999px" }}>{s.title}</Chip>
        ))}
      </Box>
      {pls === null && <CircularProgress />}
      {pls !== null && mine.length === 0 && <EmptyNote title="No playlists yet" text="Create one, then add songs from any song menu." action={<Button onClick={() => setCreating(true)}>Create a playlist</Button>} />}
      {mine.length > 0 && (
        <>
          <Typography level="title-lg" sx={{ mb: 1.5 }}>Your playlists</Typography>
          <Box sx={{ display: "flex", flexWrap: "wrap", gap: 2.5, mb: 4 }}>{mine.map(card)}</Box>
        </>
      )}
      {shared.length > 0 && (
        <>
          <Typography level="title-lg" sx={{ mb: 1.5 }}>Shared with you</Typography>
          <Box sx={{ display: "flex", flexWrap: "wrap", gap: 2.5 }}>{shared.map(card)}</Box>
        </>
      )}
      {creating && (
        <PlaylistFormDialog
          onClose={() => setCreating(false)}
          onSave={async (v) => {
            setCreating(false);
            const id = await lib.newPlaylist(v.name, v.description);
            nav(`/music/playlists/${id}`);
          }}
        />
      )}
    </Box>
  );
}

/** ShareDialog sets who can see a playlist. */
function ShareDialog({ pl, onClose, onSaved }: { pl: PlaylistInfo; onClose: () => void; onSaved: () => void }) {
  const [vis, setVis] = useState<Visibility>(pl.visibility);
  const [group, setGroup] = useState(pl.group_id);
  const [groups, setGroups] = useState<GroupRef[] | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    listGroups().then(setGroups).catch(() => setGroups([]));
  }, []);
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: { xs: "100vw", sm: 440 }, maxWidth: "100vw" }}>
        <ModalClose />
        <DialogTitle>Share “{pl.name}”</DialogTitle>
        <FormControl>
          <FormLabel>Who can see this playlist</FormLabel>
          <Select value={vis} onChange={(_, v) => v && setVis(v)} slotProps={{ button: { "aria-label": "Visibility" } }}>
            {(Object.keys(VIS) as Visibility[]).map((k) => (
              <Option key={k} value={k}>{VIS[k].label} · {VIS[k].hint}</Option>
            ))}
          </Select>
        </FormControl>
        {vis === "group" && (
          <FormControl>
            <FormLabel>Group</FormLabel>
            {groups !== null && groups.length === 0 ? (
              <Typography level="body-sm" color="warning">You are not a member of any group yet.</Typography>
            ) : (
              <Select value={group || null} placeholder="Choose a group" onChange={(_, v) => setGroup(v ?? "")} slotProps={{ button: { "aria-label": "Group" } }}>
                {(groups ?? []).map((g) => <Option key={g.id} value={g.id}>{g.name}</Option>)}
              </Select>
            )}
          </FormControl>
        )}
        {err && <Typography color="danger" level="body-sm">{err}</Typography>}
        <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1, mt: 1 }}>
          <Button variant="plain" color="neutral" onClick={onClose}>Cancel</Button>
          <Button
            loading={busy}
            disabled={vis === "group" && !group}
            onClick={async () => {
              setBusy(true);
              try {
                await setPlaylistSharing(pl.id, vis, vis === "group" ? group : "");
                onSaved();
              } catch (e) {
                setErr((e as Error).message);
                setBusy(false);
              }
            }}
          >
            Save
          </Button>
        </Box>
      </ModalDialog>
    </Modal>
  );
}

/** PlaylistPage: one playlist with drag-to-reorder, sharing and edit. */
export function PlaylistPage() {
  const { id = "" } = useParams();
  const lib = useLibrary();
  const nav = useNavigate();
  const p = usePlayer();
  const [data, setData] = useState<{ playlist: PlaylistInfo; tracks: Track[] } | null>(null);
  const [missing, setMissing] = useState(false);
  const [sharing, setSharing] = useState(false);
  const [editing, setEditing] = useState(false);
  const [err, setErr] = useState("");

  const load = () =>
    getPlaylistFull(id)
      .then((d) => {
        setData(d);
        setMissing(false);
      })
      .catch(() => setMissing(true));
  useEffect(() => {
    setData(null);
    void load();
  }, [id]); // eslint-disable-line react-hooks/exhaustive-deps

  const tracks = useMemo(() => (data?.tracks ?? []).map((t) => lib.tracks?.find((x) => x.id === t.id) ?? t), [data, lib.tracks]);
  const mine = !!data?.playlist.mine;

  const move = async (from: number, to: number) => {
    if (!data || !mine) return;
    const next = data.tracks.slice();
    const [it] = next.splice(from, 1);
    next.splice(to, 0, it);
    setData({ ...data, tracks: next });
    try {
      await reorderPlaylist(id, next.map((t) => t.id));
    } catch (e) {
      setErr((e as Error).message);
      void load();
    }
  };
  const dnd = useDragReorder(move);

  if (missing) return <EmptyNote title="Playlist not found" text="It may be private, or it was deleted." action={<Button onClick={() => nav("/music/playlists")}>Back to playlists</Button>} />;
  if (!data) return <Box sx={{ display: "flex", justifyContent: "center", py: 10 }}><CircularProgress /></Box>;
  const pl = data.playlist;
  const [bg] = coverColors(pl.name);

  return (
    <Box data-testid="playlist-page">
      <Box sx={{ display: "flex", gap: { xs: 2, md: 3.5 }, alignItems: "flex-end", flexWrap: { xs: "wrap", sm: "nowrap" }, p: { xs: 2, md: 3 }, mb: 3, borderRadius: "20px", background: `linear-gradient(135deg, ${bg.replace("72%", "90%")}, #fff 70%)`, border: "1px solid", borderColor: "divider" }}>
        <PlaylistCover pl={pl} size={170} />
        <Box sx={{ minWidth: 0, flex: 1 }}>
          <Typography level="body-xs" sx={{ textTransform: "uppercase", letterSpacing: ".08em", fontWeight: 700, color: "text.tertiary" }}>Playlist</Typography>
          <Typography level="h1" sx={{ fontSize: { xs: "xl3", md: "xl4" }, letterSpacing: "-0.02em", lineHeight: 1.1 }} data-testid="playlist-name">{pl.name}</Typography>
          {pl.description && <Typography level="body-sm" sx={{ color: "text.secondary", mt: 0.5 }}>{pl.description}</Typography>}
          <Box sx={{ display: "flex", alignItems: "center", gap: 1, mt: 1, mb: 1.5, flexWrap: "wrap" }}>
            <VisibilityChip pl={pl} />
            <Typography level="body-xs" sx={{ color: "text.tertiary" }}>
              {mine ? "You" : pl.owner_name} · {tracks.length} songs · {humanDuration(totalDuration(tracks))}
            </Typography>
          </Box>
          <Box sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap" }}>
            <PlayShuffle tracks={tracks} />
            {mine && (
              <>
                <Button size="sm" variant="outlined" color="neutral" onClick={() => setSharing(true)} data-testid="share-playlist">Share</Button>
                <Dropdown>
                  <MenuButton slots={{ root: IconButton }} slotProps={{ root: { size: "sm", variant: "outlined", color: "neutral", "aria-label": "Playlist actions" } }}><MoreHorizRoundedIcon /></MenuButton>
                  <Menu size="sm" placement="bottom-end" sx={{ zIndex: 1400 }}>
                    <MenuItem onClick={() => setEditing(true)}>Rename or edit</MenuItem>
                    <MenuItem
                      color="danger"
                      onClick={async () => {
                        if (!window.confirm(`Delete the playlist “${pl.name}”? The songs stay in your library.`)) return;
                        await deletePlaylist(pl.id);
                        await lib.refreshPlaylists();
                        nav("/music/playlists");
                      }}
                    >
                      Delete playlist
                    </MenuItem>
                  </Menu>
                </Dropdown>
              </>
            )}
          </Box>
        </Box>
      </Box>
      {err && <Typography color="danger" level="body-sm" sx={{ mb: 1 }}>{err}</Typography>}
      {tracks.length === 0 ? (
        <EmptyNote title="This playlist is empty" text="Open the ... menu of any song and choose Add to playlist." action={<Button onClick={() => nav("/music/library")}>Browse the library</Button>} />
      ) : (
        <Box>
          {mine && <Typography level="body-xs" sx={{ color: "text.tertiary", mb: 0.5 }}>Drag the handle to change the order.</Typography>}
          {tracks.map((t, i) => (
            <Box
              key={t.id}
              {...(mine ? dnd.rowProps(i) : {})}
              sx={{ display: "flex", alignItems: "center", opacity: dnd.dragging === i ? 0.4 : 1, borderTop: "2px solid", borderTopColor: dnd.over === i && dnd.dragging !== null && dnd.dragging !== i ? "primary.solidBg" : "transparent" }}
            >
              {mine && <DragIndicatorRoundedIcon fontSize="small" sx={{ color: "text.tertiary", cursor: "grab", mr: 0.5 }} aria-label="Drag to reorder" />}
              <Box sx={{ flex: 1, minWidth: 0 }}>
                <SongRow
                  track={t}
                  index={i}
                  onPlay={() => p.playQueue(tracks, i)}
                  onGoAlbum={() => nav(`/music/album/${encodeURIComponent(albumKey(t))}`)}
                  onGoArtist={() => nav(`/music/artist/${encodeURIComponent(artistOf(t))}`)}
                  extra={
                    mine ? (
                      <MenuItem
                        onClick={async () => {
                          await removeTrackFromPlaylist(id, t.id);
                          void load();
                        }}
                      >
                        <PlaylistRemoveIcon fontSize="small" /> Remove from this playlist
                      </MenuItem>
                    ) : undefined
                  }
                />
              </Box>
            </Box>
          ))}
        </Box>
      )}
      {sharing && (
        <ShareDialog
          pl={pl}
          onClose={() => setSharing(false)}
          onSaved={() => {
            setSharing(false);
            void load();
            void lib.refreshPlaylists();
          }}
        />
      )}
      {editing && (
        <PlaylistFormDialog
          playlist={{ id: pl.id, name: pl.name, description: pl.description } as never}
          onClose={() => setEditing(false)}
          onSave={async (v) => {
            setEditing(false);
            await updatePlaylist(pl.id, v);
            void load();
            void lib.refreshPlaylists();
          }}
        />
      )}
    </Box>
  );
}

/** Small shelf used on Home to open playlists. */
export function PlaylistShelf({ playlists }: { playlists: PlaylistInfo[] }) {
  const nav = useNavigate();
  if (!playlists.length) return null;
  return (
    <Shelf title="Playlists" subtitle="Yours and shared">
      {playlists.map((p) => <MediaCard key={p.id} art={p.cover_art[0]} title={p.name} sub={`${p.track_count} songs`} onOpen={() => nav(`/music/playlists/${p.id}`)} coverName={p.name} />)}
    </Shelf>
  );
}

