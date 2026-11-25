import { Box, Chip, Switch, Tooltip, Typography } from "@mui/joy";

export type SaveState = "saved" | "saving" | "unsaved" | "error";

export const SAVE_LABEL: Record<SaveState, string> = {
  saved: "Saved",
  saving: "Saving…",
  unsaved: "Unsaved changes",
  error: "Save failed",
};

export function SaveStatus({ autoSave, onAutoSave, state, onSaveNow }: { autoSave: boolean; onAutoSave: (v: boolean) => void; state: SaveState; onSaveNow: () => void }) {
  return (
    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }} data-testid="save-status">
      <Tooltip title={autoSave ? "Changes save automatically" : "AutoSave is off: save with Ctrl+S"} size="sm">
        <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
          <Typography level="body-xs">AutoSave</Typography>
          <Switch size="sm" checked={autoSave} onChange={(e) => onAutoSave(e.target.checked)} slotProps={{ input: { "aria-label": "AutoSave" } }} />
        </Box>
      </Tooltip>
      <Chip size="sm" variant="soft" color={state === "saved" ? "success" : state === "error" ? "danger" : "warning"} data-testid="save-state">
        {SAVE_LABEL[state]}
      </Chip>
      {!autoSave && state !== "saved" && (
        <Chip size="sm" variant="outlined" onClick={onSaveNow} slotProps={{ action: { "aria-label": "Save now" } }}>
          Save
        </Chip>
      )}
    </Box>
  );
}
