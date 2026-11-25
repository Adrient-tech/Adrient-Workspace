// Insert ▸ From app…: pick an Adrient module, then add its data to the
// current slide as a table or as a summary chart.
import { useEffect, useState } from "react";
import { Alert, Box, Button, Input, Modal, ModalClose, ModalDialog, Radio, RadioGroup, Stack, Typography } from "@mui/joy";
import { AppIcon } from "../../components/AppIcon";
import { APP_SOURCES, fetchAppTable, type AppSource, type AppTable } from "../sheets/appData";
import { appTableElement, summaryChart, summaryChartElement } from "./appSlides";
import type { SlideElement } from "./model";
import { listSheets, getSheet } from "../sheets/api";
import { listDocs, getDoc } from "../docs/api";
import type { Sheet } from "../sheets/types";
import type { Doc } from "../docs/types";
import { docParagraphs, docTextElement, sheetNames, sheetTable, tableChart } from "./adrientSources";

interface Props {
  onClose: () => void;
  onInsert: (el: SlideElement) => void;
}

/** Sheets range or Docs page picker (snapshots; the object links back to its source). */
function WorkspacePicker({ kind, onInsert }: { kind: "sheets" | "docs"; onInsert: (el: SlideElement) => void }) {
  const [sheets, setSheets] = useState<Sheet[]>([]);
  const [docs, setDocs] = useState<Doc[]>([]);
  const [pick, setPick] = useState("");
  const [json, setJson] = useState("");
  const [tab, setTab] = useState(0);
  const [range, setRange] = useState("");
  const [as, setAs] = useState<"table" | "chart">("table");
  const [html, setHtml] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(true);
  useEffect(() => {
    setBusy(true);
    (kind === "sheets" ? listSheets().then(setSheets) : listDocs().then(setDocs))
      .catch((e) => setErr((e as Error).message))
      .finally(() => setBusy(false));
  }, [kind]);
  async function choose(id: string) {
    setPick(id);
    setErr("");
    setBusy(true);
    try {
      if (kind === "sheets") setJson((await getSheet(id)).data ?? "");
      else setHtml((await getDoc(id)).preview_html ?? "");
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  const title = (kind === "sheets" ? sheets.find((x) => x.id === pick)?.title : docs.find((x) => x.id === pick)?.title) || "Untitled";
  const table = kind === "sheets" && json ? sheetTable(json, tab, range) : null;
  const paras = kind === "docs" && html ? docParagraphs(html) : [];
  const chart = table ? tableChart(table, `${title} · ${table.title}`) : null;
  const ok = kind === "sheets" ? (as === "table" ? !!table : !!chart) : paras.length > 0;
  function insert() {
    if (kind === "docs") return onInsert(docTextElement(title, paras, pick));
    if (as === "table" && table) {
      const t = appTableElement(table);
      onInsert({ ...t.el, name: `Sheets: ${title}`, url: `/sheets/d/${pick}` });
    } else if (chart) onInsert({ ...summaryChartElement(chart), name: `Sheets: ${title}`, url: `/sheets/d/${pick}` });
  }
  const list = kind === "sheets" ? sheets.map((x) => ({ id: x.id, title: x.title })) : docs.map((x) => ({ id: x.id, title: x.title }));
  return (
    <>
      <Box sx={{ maxHeight: 180, overflowY: "auto", display: "flex", flexDirection: "column", gap: 0.5 }} data-testid={`workspace-list-${kind}`}>
        {list.map((x) => (
          <Button key={x.id} size="sm" variant={pick === x.id ? "soft" : "outlined"} color={pick === x.id ? "primary" : "neutral"} onClick={() => void choose(x.id)} sx={{ justifyContent: "flex-start" }}>
            {x.title || "Untitled"}
          </Button>
        ))}
        {!busy && !list.length && !err && <Typography level="body-sm">No {kind === "sheets" ? "spreadsheets" : "documents"} yet.</Typography>}
      </Box>
      {err && <Alert color="danger" variant="soft">{err}</Alert>}
      {kind === "sheets" && pick && json && (
        <Box sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap" }}>
          <select aria-label="Sheet tab" value={tab} onChange={(e) => setTab(Number(e.target.value))}>
            {sheetNames(json).map((n, i) => <option key={i} value={i}>{n}</option>)}
          </select>
          <Input size="sm" placeholder="Range, e.g. A1:D10 (empty = all)" value={range} onChange={(e) => setRange(e.target.value)} sx={{ flex: 1, minWidth: 160 }} slotProps={{ input: { "aria-label": "Cell range" } }} />
          <RadioGroup orientation="horizontal" value={as} onChange={(e) => setAs(e.target.value as "table" | "chart")}>
            <Radio value="table" label="Table" />
            <Radio value="chart" label="Chart" />
          </RadioGroup>
        </Box>
      )}
      {pick && !busy && (
        <Typography level="body-sm" data-testid="workspace-status">
          {kind === "sheets"
            ? table ? `${table.rows.length} rows × ${table.header.length} columns found.` : "No data in that range."
            : paras.length ? `${paras.length} paragraphs from the document's first page.` : "This document has no text yet."}
        </Typography>
      )}
      <Typography level="body-xs">Inserted as a snapshot. Clicking it in the show opens the source.</Typography>
      <Stack direction="row" spacing={1} justifyContent="flex-end">
        <Button disabled={!ok || busy} onClick={insert} data-testid="workspace-insert">Insert</Button>
      </Stack>
    </>
  );
}

export function InsertFromAppDialog({ onClose, onInsert }: Props) {
  const [mode, setMode] = useState<"apps" | "sheets" | "docs">("apps");
  const [picked, setPicked] = useState<AppSource | null>(null);
  const [table, setTable] = useState<AppTable | null>(null);
  const [as, setAs] = useState<"table" | "chart">("table");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

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
  const tbl = table && picked ? appTableElement(table) : null;
  const chart = table && picked ? summaryChart(picked.id, table) : null;
  const canInsert = as === "table" ? !!tbl && tbl.shown > 0 : !!chart;

  function insert() {
    if (as === "table" && tbl) onInsert(tbl.el);
    else if (chart) onInsert(summaryChartElement(chart));
  }

  return (
    <Modal open onClose={onClose}>
      <ModalDialog aria-labelledby="slideapp-title" sx={{ width: 560, maxWidth: "95vw" }}>
        <ModalClose />
        <Typography id="slideapp-title" level="title-lg">
          Insert from app
        </Typography>
        <RadioGroup orientation="horizontal" value={mode} onChange={(e) => setMode(e.target.value as typeof mode)} data-testid="from-mode">
          <Radio value="apps" label="CRM, Inventory, POS, Invoicing" />
          <Radio value="sheets" label="Sheets range or chart" />
          <Radio value="docs" label="Docs page" />
        </RadioGroup>
        {mode !== "apps" && <WorkspacePicker key={mode} kind={mode} onInsert={onInsert} />}
        {mode === "apps" && (
          <>
        <Typography level="body-sm">Add live business data to the current slide. Pick a source:</Typography>
        <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1 }}>
          {APP_SOURCES.map((s) => (
            <Button
              key={s.id}
              variant={picked?.id === s.id ? "soft" : "outlined"}
              color={picked?.id === s.id ? "primary" : "neutral"}
              onClick={() => void pick(s)}
              data-testid={`slideapp-${s.id}`}
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
          <>
            <RadioGroup orientation="horizontal" value={as} onChange={(e) => setAs(e.target.value as "table" | "chart")}>
              <Radio value="table" label="Table" data-testid="slideapp-as-table" />
              <Radio value="chart" label="Summary chart" data-testid="slideapp-as-chart" />
            </RadioGroup>
            <Typography level="body-sm" data-testid="slideapp-status">
              {busy
                ? "Loading…"
                : !table
                  ? ""
                  : as === "table" && tbl
                    ? tbl.shown === 0
                      ? "No rows in this source."
                      : `${tbl.total} rows available${tbl.total > tbl.shown ? `; the first ${tbl.shown} will be inserted` : ""}.`
                    : chart
                      ? `Chart: ${chart.title} (${chart.data.length - 1} groups).`
                      : "Not enough data for a chart."}
            </Typography>
          </>
        )}
        <Stack direction="row" spacing={1} justifyContent="flex-end">
          <Button variant="plain" color="neutral" onClick={onClose}>
            Cancel
          </Button>
          <Button disabled={!canInsert || busy} onClick={insert} data-testid="slideapp-insert">
            Insert
          </Button>
        </Stack>
          </>
        )}
      </ModalDialog>
    </Modal>
  );
}
