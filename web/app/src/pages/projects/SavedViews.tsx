import { useCallback, useEffect, useState } from "react";
import { Box, Button, Checkbox, Dropdown, IconButton, Input, Menu, MenuButton, MenuItem, Modal, ModalClose, ModalDialog, Typography } from "@mui/joy";
import DeleteIcon from "@mui/icons-material/DeleteOutline";
import * as api from "./planApi";

export interface ViewState {
  status: string[];
  priority: number[];
  assignee: string[];
  groupBy: string;
}

/** Filters stored with a saved view are read back defensively: the server stores any JSON. */
export function readFilters(f: Record<string, unknown>): ViewState {
  const strs = (v: unknown) => (Array.isArray(v) ? v.filter((x): x is string => typeof x === "string") : []);
  const nums = (v: unknown) => (Array.isArray(v) ? v.filter((x): x is number => typeof x === "number") : []);
  const g = typeof f.groupBy === "string" && ["status", "priority", "assignee"].includes(f.groupBy) ? f.groupBy : "status";
  return { status: strs(f.status), priority: nums(f.priority), assignee: strs(f.assignee), groupBy: g };
}

const MODES = new Set(["list", "board", "my", "timeline"]);

/** Saved views menu: apply, save the current filters, delete. */
export function SavedViewsMenu({ mode, state, onApply }: { mode: string; state: ViewState; onApply: (mode: string, s: ViewState) => void }) {
  const [views, setViews] = useState<api.SavedView[]>([]);
  const [saving, setSaving] = useState(false);
  const [name, setName] = useState("");
  const [shared, setShared] = useState(false);
  const [err, setErr] = useState("");
  const load = useCallback(() => api.listViews().then(setViews).catch(() => setViews([])), []);
  useEffect(() => { void load(); }, [load]);
  async function save() {
    setErr("");
    try {
      await api.createView({ name, mode, filters: { ...state }, shared });
      setSaving(false);
      setName("");
      await load();
    } catch (e) {
      setErr((e as Error).message);
    }
  }
  return (
    <>
      <Dropdown>
        <MenuButton size="sm" variant="outlined" color="neutral" data-testid="views-menu">Views</MenuButton>
        <Menu size="sm" placement="bottom-end" sx={{ minWidth: 220 }}>
          {views.length === 0 && <Typography level="body-xs" sx={{ px: 1.5, py: 0.5 }}>No saved views</Typography>}
          {views.map((v) => (
            <MenuItem key={v.id} onClick={() => onApply(MODES.has(v.mode) ? v.mode : "list", readFilters(v.filters))} data-testid="view-item">
              <Box sx={{ flex: 1 }}>{v.name}{v.shared && !v.mine ? " (shared)" : ""}</Box>
              {v.mine && (
                <IconButton size="sm" variant="plain" color="danger" aria-label={`Delete view ${v.name}`} onClick={async (e) => { e.stopPropagation(); await api.deleteView(v.id).catch(() => undefined); await load(); }}>
                  <DeleteIcon fontSize="small" />
                </IconButton>
              )}
            </MenuItem>
          ))}
          <MenuItem onClick={() => setSaving(true)} data-testid="view-save-current">Save current view...</MenuItem>
        </Menu>
      </Dropdown>
      {saving && (
        <Modal open onClose={() => setSaving(false)}>
          <ModalDialog sx={{ width: "min(420px, 96vw)" }}>
            <ModalClose />
            <Typography level="title-lg">Save view</Typography>
            <Input autoFocus placeholder="View name" value={name} onChange={(e) => setName(e.target.value)} slotProps={{ input: { "data-testid": "view-name" } }} />
            <Checkbox size="sm" label="Share with everyone in the organisation" checked={shared} onChange={(e) => setShared(e.target.checked)} />
            {err && <Typography color="danger" level="body-sm">{err}</Typography>}
            <Button disabled={!name.trim()} onClick={save} data-testid="view-save">Save</Button>
          </ModalDialog>
        </Modal>
      )}
    </>
  );
}
