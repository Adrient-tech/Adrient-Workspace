import { createContext, useContext, useEffect, useMemo, useRef, type ReactNode } from "react";
import { Alert, Box, Button, Typography } from "@mui/joy";

/** Tracks which settings forms have unsaved changes so leaving the page can ask first. */
export interface DirtyApi {
  set: (id: string, dirty: boolean) => void;
  /** True when it is fine to leave: nothing is dirty, or the person confirmed. */
  confirmLeave: () => boolean;
}

const Ctx = createContext<DirtyApi | null>(null);

export const LEAVE_MESSAGE = "You have unsaved changes. Leave this page and lose them?";

export function AdminDirtyProvider({ children, apiRef }: { children: ReactNode; apiRef?: { current: DirtyApi | null } }) {
  const dirty = useRef(new Set<string>());
  const api = useMemo<DirtyApi>(() => ({
    set: (id, d) => { if (d) dirty.current.add(id); else dirty.current.delete(id); },
    confirmLeave: () => dirty.current.size === 0 || window.confirm(LEAVE_MESSAGE),
  }), []);
  if (apiRef) apiRef.current = api;
  useEffect(() => {
    const onUnload = (e: BeforeUnloadEvent) => {
      if (dirty.current.size === 0) return;
      e.preventDefault();
      e.returnValue = "";
    };
    window.addEventListener("beforeunload", onUnload);
    return () => window.removeEventListener("beforeunload", onUnload);
  }, []);
  return <Ctx.Provider value={api}>{children}</Ctx.Provider>;
}

/** Register a form's dirty state. It is cleared when the form unmounts. */
export function useDirtyFlag(id: string, dirty: boolean) {
  const api = useContext(Ctx);
  useEffect(() => {
    api?.set(id, dirty);
    return () => api?.set(id, false);
  }, [api, id, dirty]);
}

/** Sticky bar at the bottom of a settings form. Shows only while there is something to save. */
export function SaveBar({ dirty, saving, error, onSave, onDiscard, saveLabel = "Save changes", testId }: {
  dirty: boolean;
  saving?: boolean;
  error?: string | null;
  onSave: () => void;
  onDiscard: () => void;
  saveLabel?: string;
  testId?: string;
}) {
  if (!dirty && !error) return null;
  return (
    <Box
      role="region"
      aria-label="Unsaved changes"
      data-testid={testId ? `${testId}-bar` : "save-bar"}
      sx={{
        position: "sticky", bottom: 0, zIndex: 5, mt: 2, mx: -2.5, mb: -2.5, px: 2.5, py: 1.25,
        display: "flex", alignItems: "center", gap: 1.5, flexWrap: "wrap",
        bgcolor: "background.surface", borderTop: "1px solid", borderColor: "divider", borderRadius: "0 0 12px 12px",
        boxShadow: "0 -6px 16px rgba(14,36,32,.08)",
      }}
    >
      <Typography level="body-sm" sx={{ flex: 1, minWidth: 160 }}>{dirty ? "You have unsaved changes." : ""}</Typography>
      {error && <Alert color="danger" size="sm">{error}</Alert>}
      <Button size="sm" variant="plain" color="neutral" onClick={onDiscard} disabled={saving || !dirty} data-testid={testId ? `${testId}-discard` : "save-bar-discard"}>Discard</Button>
      <Button size="sm" onClick={onSave} loading={saving} disabled={!dirty} data-testid={testId ?? "save-bar-save"}>{saveLabel}</Button>
    </Box>
  );
}
