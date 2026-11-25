import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { getLibrary, listPlaylistInfos, createPlaylist, addTrackToPlaylist } from "./api";
import type { Track, PlaylistInfo } from "./types";
import { usePlayer } from "./player";
import { useTrackActions } from "./useTrackActions";
import { Snackbar } from "@mui/joy";

/** The shared library state of the Music app: every track of the
 *  organisation with the user's stats, the playlists the user can see, and the
 *  per-track actions (like, queue, edit, delete, add to playlist) with their
 *  dialogs. One provider sits under the PlayerProvider. */
interface LibraryState {
  userId: string;
  tracks: Track[] | null;
  playlists: PlaylistInfo[] | null;
  error: string | null;
  refresh: () => Promise<void>;
  refreshPlaylists: () => Promise<void>;
  patch: (id: string, p: Partial<Track>) => void;
  actions: ReturnType<typeof useTrackActions>["actions"];
  /** Create a playlist (optionally with a first track) and return its id. */
  newPlaylist: (name: string, description: string, firstTrack?: Track) => Promise<string>;
}

const Ctx = createContext<LibraryState | null>(null);

export function useLibrary(): LibraryState {
  const v = useContext(Ctx);
  if (!v) throw new Error("useLibrary must be used within <LibraryProvider>");
  return v;
}

export function LibraryProvider({ userId, children }: { userId: string; children: ReactNode }) {
  const player = usePlayer();
  const [tracks, setTracks] = useState<Track[] | null>(null);
  const [playlists, setPlaylists] = useState<PlaylistInfo[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      setTracks(await getLibrary());
      setError(null);
    } catch (e) {
      setError((e as Error).message);
      setTracks((t) => t ?? []);
    }
  }, []);
  const refreshPlaylists = useCallback(async () => {
    try {
      setPlaylists(await listPlaylistInfos());
    } catch (e) {
      setError((e as Error).message);
      setPlaylists((p) => p ?? []);
    }
  }, []);
  useEffect(() => {
    void refresh();
    void refreshPlaylists();
  }, [refresh, refreshPlaylists]);

  const patch = useCallback(
    (id: string, p: Partial<Track>) => {
      setTracks((cur) => (cur ? cur.map((t) => (t.id === id ? { ...t, ...p } : t)) : cur));
      player.patchTrack(id, p);
    },
    [player],
  );

  const ta = useTrackActions({
    onTrackUpdated: (t) => patch(t.id, t),
    onTrackDeleted: (id) => {
      setTracks((cur) => (cur ? cur.filter((t) => t.id !== id) : cur));
      void refreshPlaylists();
    },
    onTrackLikeToggled: (id, liked) => patch(id, { liked }),
  });

  const newPlaylist = useCallback(
    async (name: string, description: string, firstTrack?: Track) => {
      const pl = await createPlaylist({ name, description });
      if (firstTrack) await addTrackToPlaylist(pl.id, firstTrack.id);
      await refreshPlaylists();
      return pl.id;
    },
    [refreshPlaylists],
  );

  const value = useMemo<LibraryState>(
    () => ({ userId, tracks, playlists, error, refresh, refreshPlaylists, patch, actions: ta.actions, newPlaylist }),
    [userId, tracks, playlists, error, refresh, refreshPlaylists, patch, ta.actions, newPlaylist],
  );
  return (
    <Ctx.Provider value={value}>
      {children}
      {ta.dialogs}
      <Snackbar open={!!ta.error} autoHideDuration={5000} onClose={ta.clearError} color="danger" variant="soft">
        {ta.error}
      </Snackbar>
    </Ctx.Provider>
  );
}
