import { useMemo, useState } from "react";
import { Button, Checkbox, Modal, ModalClose, ModalDialog, Stack, Table, Typography } from "@mui/joy";
import { describeColumns, statsGrid } from "./dataAnalysis";

// Data ▸ Summary statistics for the selected block (one column of stats per
// numeric column). "Insert" writes the table at a cell the user picks next.
export function StatsDialog({ open, onClose, block, onInsert }: { open: boolean; onClose: () => void; block: unknown[][]; onInsert: (rows: (string | number | null)[][]) => void }) {
  const [headers, setHeaders] = useState(true);
  const grid = useMemo(() => statsGrid(describeColumns(block, headers)), [block, headers]);
  const empty = grid[0].length <= 1;
  const fmt = (v: unknown) => (typeof v === "number" ? Number(v.toPrecision(8)).toLocaleString(undefined, { maximumFractionDigits: 6 }) : String(v ?? ""));
  return (
    <Modal open={open} onClose={onClose}>
      <ModalDialog sx={{ width: 560, maxWidth: "96vw", maxHeight: "90vh", overflow: "auto" }}>
        <ModalClose />
        <Typography level="title-lg">Summary statistics</Typography>
        <Checkbox size="sm" label="First row is labels" checked={headers} onChange={(e) => setHeaders(e.target.checked)} />
        {empty ? (
          <Typography level="body-sm">The selection has no numbers. Select a block of numeric columns first.</Typography>
        ) : (
          <Table size="sm" borderAxis="both" data-testid="stats-table" sx={{ "& td": { textAlign: "right" } }}>
            <tbody>
              {grid.map((r, i) => (
                <tr key={i}>
                  {r.map((v, j) => (i === 0 || j === 0 ? <th key={j}>{fmt(v)}</th> : <td key={j}>{fmt(v)}</td>))}
                </tr>
              ))}
            </tbody>
          </Table>
        )}
        <Stack direction="row" spacing={1} justifyContent="flex-end">
          <Button variant="plain" onClick={onClose}>
            Close
          </Button>
          <Button disabled={empty} onClick={() => onInsert(grid)}>
            Insert as new sheet
          </Button>
        </Stack>
      </ModalDialog>
    </Modal>
  );
}
