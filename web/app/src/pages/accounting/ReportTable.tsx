import { useCallback, useEffect, useState } from "react";
import { Box, Button, Modal, ModalDialog, Sheet, Table, Typography } from "@mui/joy";
import { getRegister } from "./api";
import { reportExportURL, reportTable } from "./api2";
import type { TableData } from "./api2";
import { money } from "./logic";
import type { RegisterRow } from "./types";
import { ErrorAlert, rightCell, useErr } from "./ui";

/** Download buttons for the server-side CSV, XLSX and PDF exports of a report. */
export function ExportButtons({ name, q }: { name: string; q: Record<string, string | undefined> }) {
  return (
    <Box className="no-print" sx={{ display: "inline-flex", gap: 0.5 }}>
      {(["csv", "xlsx", "pdf"] as const).map((f) => (
        <Button key={f} size="sm" variant="soft" component="a" href={reportExportURL(name, f, q)} download data-testid={`acct-export-${f}`}>
          {f.toUpperCase()}
        </Button>
      ))}
    </Box>
  );
}

/** Journals behind an account amount, opened from a report row. */
export function DrillDown({ accountId, name, from, to, onClose, onOpenJournal }: { accountId: string; name: string; from?: string; to?: string; onClose: () => void; onOpenJournal?: (id: string) => void }) {
  const [rows, setRows] = useState<RegisterRow[] | null>(null);
  const [opening, setOpening] = useState(0);
  const { err, run } = useErr();
  useEffect(() => {
    run(() => getRegister(accountId, from ?? "", to ?? "")).then((r) => {
      if (r) {
        setRows(r.rows);
        setOpening(r.opening);
      }
    });
  }, [accountId, from, to, run]);
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 820, maxWidth: "96vw", maxHeight: "90vh", overflow: "auto" }} data-testid="acct-drilldown">
        <Typography level="title-lg">{name}</Typography>
        <Typography level="body-sm">
          {from ? `${from} to ${to}` : `Up to ${to}`}. Opening balance {money(opening)}.
        </Typography>
        <ErrorAlert err={err} />
        {rows && rows.length === 0 && <Typography>No entries in this period.</Typography>}
        {rows && rows.length > 0 && (
          <Table size="sm" hoverRow>
            <thead>
              <tr>
                <th>Date</th>
                <th>#</th>
                <th>Memo</th>
                <th>Contact</th>
                <th style={rightCell}>Debit</th>
                <th style={rightCell}>Credit</th>
                <th style={rightCell}>Balance</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r, i) => (
                <tr key={i} onClick={() => onOpenJournal?.(r.journal_id)} style={{ cursor: onOpenJournal ? "pointer" : undefined }}>
                  <td>{r.date}</td>
                  <td>{r.number}</td>
                  <td>{r.memo}</td>
                  <td>{r.contact}</td>
                  <td style={rightCell}>{r.debit ? money(r.debit) : ""}</td>
                  <td style={rightCell}>{r.credit ? money(r.credit) : ""}</td>
                  <td style={rightCell}>{money(r.balance)}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
        <Button variant="plain" color="neutral" onClick={onClose}>
          Close
        </Button>
      </ModalDialog>
    </Modal>
  );
}

/** Renders any report through the generic table view the API provides, with drill-down and export. */
export function GenericReport({
  name,
  q,
  version,
  drillFrom,
  drillTo,
  onOpenJournal,
}: {
  name: string;
  q: Record<string, string | undefined>;
  version: number;
  drillFrom?: string;
  drillTo?: string;
  onOpenJournal?: (id: string) => void;
}) {
  const [data, setData] = useState<TableData | null>(null);
  const [drill, setDrill] = useState<{ id: string; name: string } | null>(null);
  const { err, run } = useErr();
  const key = JSON.stringify(q);
  const load = useCallback(async () => {
    const d = await run(() => reportTable(name, JSON.parse(key)));
    if (d) setData(d);
  }, [name, key, run]);
  useEffect(() => {
    load();
  }, [load, version]);
  return (
    <Box>
      <ErrorAlert err={err} />
      {data && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", p: 2, overflow: "auto" }} data-testid="acct-report-body">
          <Typography level="title-md">{data.title}</Typography>
          <Typography level="body-sm" sx={{ mb: 1 }}>
            {data.subtitle}
          </Typography>
          <Table size="sm" data-testid="acct-generic-report">
            <thead>
              <tr>
                {data.headers.map((h, i) => (
                  <th key={i} style={i > 0 ? rightCell : undefined}>
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {data.rows.map((r, ri) => {
                const link = data.links?.[ri];
                const isHead = r.slice(1).every((c) => c === "" || c === null);
                return (
                  <tr key={ri} onClick={link ? () => setDrill({ id: link, name: String(r[0]) }) : undefined} style={{ cursor: link ? "pointer" : undefined, fontWeight: isHead ? 700 : undefined }}>
                    {r.map((c, ci) => (
                      <td key={ci} style={ci > 0 && typeof c === "number" ? rightCell : undefined}>
                        {typeof c === "number" ? (/%/.test(data.headers[ci] ?? "") ? `${c}%` : /qty|rate|^no\.?$/i.test(data.headers[ci] ?? "") ? String(c) : money(c)) : c}
                      </td>
                    ))}
                  </tr>
                );
              })}
            </tbody>
          </Table>
        </Sheet>
      )}
      {drill && <DrillDown accountId={drill.id} name={drill.name} from={drillFrom} to={drillTo} onClose={() => setDrill(null)} onOpenJournal={onOpenJournal} />}
    </Box>
  );
}
