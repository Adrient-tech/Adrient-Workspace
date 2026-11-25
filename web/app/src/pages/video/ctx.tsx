import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";
import { Snackbar } from "@mui/joy";
import { setWatchLater } from "./api";
import type { VideoInfo } from "./types";
import { AddToPlaylistDialog } from "./AddToPlaylistDialog";

interface VideoCtx {
  userId: string;
  /** Toggle Watch later; `after` receives the new state so lists can update. */
  toggleWatchLater: (v: VideoInfo, after?: (on: boolean) => void) => Promise<void>;
  addToPlaylist: (v: VideoInfo) => void;
  notify: (msg: string) => void;
  /** Bumped when subscriptions change so the rail reloads. */
  subsVersion: number;
  bumpSubs: () => void;
}

const Ctx = createContext<VideoCtx | null>(null);
export function useVideoCtx(): VideoCtx {
  const v = useContext(Ctx);
  if (!v) throw new Error("useVideoCtx must be used inside <VideoProvider>");
  return v;
}

export function VideoProvider({ userId, children }: { userId: string; children: ReactNode }) {
  const [msg, setMsg] = useState("");
  const [plFor, setPlFor] = useState<VideoInfo | null>(null);
  const [subsVersion, setSubsVersion] = useState(0);
  const notify = useCallback((m: string) => setMsg(m), []);
  const toggleWatchLater = useCallback(
    async (v: VideoInfo, after?: (on: boolean) => void) => {
      const on = !v.watch_later;
      try {
        await setWatchLater(v.id, on);
        after?.(on);
        setMsg(on ? "Saved to Watch later" : "Removed from Watch later");
      } catch {
        setMsg("Could not update Watch later");
      }
    },
    [],
  );
  const value = useMemo<VideoCtx>(() => ({ userId, toggleWatchLater, addToPlaylist: setPlFor, notify, subsVersion, bumpSubs: () => setSubsVersion((n) => n + 1) }), [userId, toggleWatchLater, notify, subsVersion]);
  return (
    <Ctx.Provider value={value}>
      {children}
      {plFor && <AddToPlaylistDialog videoId={plFor.id} onClose={() => setPlFor(null)} />}
      <Snackbar open={!!msg} autoHideDuration={2600} onClose={() => setMsg("")} variant="soft" color="neutral" anchorOrigin={{ vertical: "bottom", horizontal: "left" }}>
        {msg}
      </Snackbar>
    </Ctx.Provider>
  );
}
