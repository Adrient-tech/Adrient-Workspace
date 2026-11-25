import { useEffect, useState } from "react";
import { Alert, Box, Button, Modal, ModalClose, ModalDialog, Stack, Typography } from "@mui/joy";
import { AppIcon } from "../../components/AppIcon";
import { APP_SOURCES, fetchAppTable, type AppSource, type AppTable } from "./appData";

// Insert ▸ From app…: choose a module, preview its table, and add it as a new
// sheet tab (the caller writes it).
export function InsertFromAppDialog({
  open,
  onClose,
  onInsert,
}: {
  open: boolean;
  onClose: () => void;
  onInsert: (table: AppTable) => void;
}) {
  const [picked, setPicked] = useState<AppSource | null>(null);
  const [table, setTable] = useState<AppTable | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (open) {
      setPicked(null);
      setTable(null);
      setError("");
      setBusy(false);
    }
  }, [open]);

  async function pick(src: AppSource) {
    setPicked(src);
    setTable(null);
    setError("");
    setBusy(true);
    try {
      setTable(await fetchAppTable(src));
    } catch (e) {
      setError((e as Error).message || "Could not load the data");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal open={open} onClose={onClose}>
      <ModalDialog aria-labelledby="fromapp-title" sx={{ width: 560, maxWidth: "95vw" }}>
        <ModalClose />
        <Typography id="fromapp-title" level="title-lg">
          Insert from app
        </Typography>
        <Typography level="body-sm">Adds the table as a new sheet in this workbook. Pick a source:</Typography>
        <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1 }}>
          {APP_SOURCES.map((s) => (
            <Button
              key={s.id}
              variant={picked?.id === s.id ? "soft" : "outlined"}
              color={picked?.id === s.id ? "primary" : "neutral"}
              onClick={() => void pick(s)}
              data-testid={`fromapp-${s.id}`}
              sx={{ justifyContent: "flex-start", textAlign: "left", p: 1.25, gap: 1.25, height: "auto", alignItems: "flex-start" }}
            >
              <AppIcon iconName={s.icon} color="#126C5A" size={36} />
              <Box>
                <Typography level="title-sm">{s.label}</Typography>
                <Typography level="body-xs" sx={{ fontWeight: 400 }}>
                  {s.description}
                </Typography>
              </Box>
            </Button>
          ))}
        </Box>
        {error && (
          <Alert color="danger" variant="soft" role="alert">
            {error}
          </Alert>
        )}
        {picked && !error && (
          <Typography level="body-sm" data-testid="fromapp-status">
            {busy ? "Loading…" : table ? `${table.rows.length} rows · ${table.header.length} columns: ${table.header.join(", ")}` : ""}
          </Typography>
        )}
        <Stack direction="row" spacing={1} justifyContent="flex-end">
          <Button variant="plain" color="neutral" onClick={onClose}>
            Cancel
          </Button>
          <Button disabled={!table} onClick={() => table && onInsert(table)} data-testid="fromapp-insert">
            Insert
          </Button>
        </Stack>
      </ModalDialog>
    </Modal>
  );
}
