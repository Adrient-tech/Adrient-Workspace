// AutoSave indicator state. Documents are saved continuously by the
// collaboration hub: a local edit is sent over the WebSocket as it happens.
// "Saving…" shows from the edit until the socket has been connected and
// quiet for a moment; "Offline" shows while disconnected (edits stay in the
// local Yjs state, and in IndexedDB when the document is available offline).
import { useEffect, useState } from "react";
import type * as Y from "yjs";
import type { WebsocketProvider } from "y-websocket";

export type SaveState = "saved" | "saving" | "offline" | "connecting";

export function nextSaveState(connected: boolean, dirty: boolean, everConnected: boolean): SaveState {
  if (!connected) return everConnected ? "offline" : "connecting";
  return dirty ? "saving" : "saved";
}

export function useSaveStatus(ydoc: Y.Doc, provider: WebsocketProvider): SaveState {
  const [state, setState] = useState<SaveState>("connecting");
  useEffect(() => {
    let connected = provider.wsconnected;
    let ever = connected;
    let dirty = false;
    let t: ReturnType<typeof setTimeout> | undefined;
    const apply = () => setState(nextSaveState(connected, dirty, ever));
    const onUpdate = (_u: Uint8Array, origin: unknown) => {
      if (origin === provider) return; // remote change
      dirty = true;
      apply();
      clearTimeout(t);
      t = setTimeout(() => {
        dirty = false;
        apply();
      }, 900);
    };
    const onStatus = (e: { status: string }) => {
      connected = e.status === "connected";
      ever = ever || connected;
      apply();
    };
    apply();
    ydoc.on("update", onUpdate);
    provider.on("status", onStatus);
    return () => {
      clearTimeout(t);
      ydoc.off("update", onUpdate);
      provider.off("status", onStatus);
    };
  }, [ydoc, provider]);
  return state;
}
