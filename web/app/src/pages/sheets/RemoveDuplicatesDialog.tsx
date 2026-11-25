import { useEffect, useState } from "react";
import { Button, Checkbox, Modal, ModalClose, ModalDialog, Stack, Typography } from "@mui/joy";

// Data ▸ Data cleanup ▸ Remove duplicates: pick the columns that identify a
// duplicate; the caller applies it to the selected range.
export function RemoveDuplicatesDialog({
  open,
  onClose,
  columns,
  firstRowLabels,
  onApply,
}: {
  open: boolean;
  onClose: () => void;
  /** Column letters of the selected range. */
  columns: string[];
  /** First-row text per column, to name checkboxes when there is a header. */
  firstRowLabels: string[];
  onApply: (opts: { header: boolean; keyCols: number[] }) => void;
}) {
  const [header, setHeader] = useState(true);
  const [sel, setSel] = useState<boolean[]>([]);
  useEffect(() => {
    if (open) {
      setHeader(true);
      setSel(columns.map(() => true));
    }
  }, [open, columns]);
  const keyCols = sel.map((on, i) => (on ? i : -1)).filter((i) => i >= 0);
  return (
    <Modal open={open} onClose={onClose}>
      <ModalDialog aria-labelledby="dedupe-title" sx={{ width: 380, maxWidth: "95vw" }}>
        <ModalClose />
        <Typography id="dedupe-title" level="title-lg">
          Remove duplicates
        </Typography>
        <Checkbox label="Data has header row" checked={header} onChange={(e) => setHeader(e.target.checked)} />
        <Typography level="body-sm">Analyse these columns:</Typography>
        <Stack spacing={0.75} sx={{ maxHeight: 240, overflow: "auto" }}>
          <Checkbox
            label="Select all"
            checked={keyCols.length === columns.length}
            indeterminate={keyCols.length > 0 && keyCols.length < columns.length}
            onChange={(e) => setSel(columns.map(() => e.target.checked))}
          />
          {columns.map((c, i) => (
            <Checkbox
              key={c}
              checked={!!sel[i]}
              onChange={(e) => setSel(sel.map((v, j) => (j === i ? e.target.checked : v)))}
              label={header && firstRowLabels[i] ? `${firstRowLabels[i]} (column ${c})` : `Column ${c}`}
            />
          ))}
        </Stack>
        <Stack direction="row" spacing={1} justifyContent="flex-end">
          <Button variant="plain" color="neutral" onClick={onClose}>
            Cancel
          </Button>
          <Button disabled={keyCols.length === 0} onClick={() => onApply({ header, keyCols })} data-testid="dedupe-apply">
            Remove duplicates
          </Button>
        </Stack>
      </ModalDialog>
    </Modal>
  );
}
