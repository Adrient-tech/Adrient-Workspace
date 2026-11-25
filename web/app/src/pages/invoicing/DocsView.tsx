import { useCallback, useEffect, useRef, useState } from "react";
import { Box, Button, Checkbox, Input, Option, Select, Sheet, Table, Typography } from "@mui/joy";
import { bulkAction, createDocument, getDocument,
  listDocumentsPage, purgeDocument, restoreDocument } from "./api";
import { DocDetail } from "./DocDetail";
import { DocEditor } from "./DocEditor";
import { auditExport } from "../../lib/auditExport";
import { ACCENT, blankDoc, docsCSV, downloadText, inputFromDoc, money, parseInvoicesCSV } from "./logic";
import type { InvDocument, Item, Settings, TaxRate } from "./types";
import { Empty, ErrorAlert, StatusChip, rightCell, useErr } from "./ui";

export function DocsView({
  kind,
  settings,
  items,
  taxes,
  emailReady,
  version,
  onChanged,
  openId,
  clearOpen,
  presetQuery,
  onOpenOrder,
}: {
  kind: "invoice" | "estimate" | "credit_note";
  onOpenOrder?: (id: string) => void;
  settings: Settings;
  items: Item[];
  taxes: TaxRate[];
  emailReady: boolean;
  version: number;
  onChanged: () => void;
  openId?: string;
  clearOpen?: () => void;
  presetQuery?: string;
}) {
  const [docs, setDocs] = useState<InvDocument[] | null>(null);
  const [status, setStatus] = useState("");
  const [query, setQuery] = useState(presetQuery ?? "");
  const [trash, setTrash] = useState(false);
  const [detail, setDetail] = useState<InvDocument | null>(null);
  const [editor, setEditor] = useState<{ editing: InvDocument | null } | null>(null);
  const [note, setNote] = useState("");
  const fileRef = useRef<HTMLInputElement>(null);
  const { err, run } = useErr();
  const isInv = kind === "invoice";
  const isCn = kind === "credit_note";
  const [sel, setSel] = useState<string[]>([]);
  const toggle = (id: string) => setSel((x) => (x.includes(id) ? x.filter((y) => y !== id) : [...x, id]));
  const doBulk = async (action: string) => {
    if (action === "trash" && !window.confirm(`Move ${sel.length} document(s) to the trash?`)) return;
    const res = await run(() => bulkAction(sel, action));
    if (!res) return;
    const bad = res.filter((r) => !r.ok);
    setNote(`${res.length - bad.length} of ${res.length} done.${bad.length ? " Problems: " + [...new Set(bad.map((b) => b.error))].join("; ") : ""}`);
    setSel([]);
    onChanged();
  };

  // Paged on the server, 100 per page; "Load more" appends the next page.
  const [next, setNext] = useState("");
  const [more, setMore] = useState(false);
  const load = useCallback(() => {
    listDocumentsPage({ kind, status, q: query, trash })
      .then((r) => {
        setDocs(r.items);
        setNext(r.next);
      })
      .catch(() => setDocs([]));
  }, [kind, status, query, trash]);
  const loadMore = async () => {
    setMore(true);
    try {
      const r = await listDocumentsPage({ kind, status, q: query, trash, page: Number(next) });
      setDocs((d) => [...(d ?? []), ...r.items.filter((x) => !(d ?? []).some((y) => y.id === x.id))]);
      setNext(r.next);
    } finally {
      setMore(false);
    }
  };
  useEffect(load, [load, version]);

  useEffect(() => {
    if (!openId) return;
    getDocument(openId)
      .then((d) => {
        setDetail(d);
        clearOpen?.();
      })
      .catch(() => clearOpen?.());
  }, [openId, clearOpen]);

  const openDoc = async (id: string) => {
    const d = await run(() => getDocument(id));
    if (d) {
      setEditor(null);
      setDetail(d);
    }
  };

  const importCSV = async (file: File) => {
    const parsed = parseInvoicesCSV(await file.text());
    if (parsed.length === 0) {
      setNote("No invoices found. Expected columns: number, client, date, description, qty, price (one row per line).");
      return;
    }
    let ok = 0;
    for (const p of parsed) {
      const d = await run(() => createDocument({ ...blankDoc("invoice", settings.default_currency, settings.due_days, { notes: "", terms: "", tax_note: "" }), ...p, currency: p.currency || settings.default_currency }));
      if (d) ok++;
    }
    setNote(`Imported ${ok} of ${parsed.length} invoices as drafts.`);
    onChanged();
  };

  const statuses = isInv
    ? ["draft", "sent", "partial", "overdue", "paid", "credited", "written_off"]
    : isCn
      ? ["open", "partly_used", "applied", "refunded"]
      : ["draft", "sent", "accepted", "declined", "expired", "superseded", "converted"];

  return (
    <Box>
      <Box sx={{ display: "flex", gap: 1, mb: 2, flexWrap: "wrap" }}>
        <Input placeholder="Search number, client, notes" value={query} onChange={(e) => setQuery(e.target.value)} sx={{ flex: 1, minWidth: 200 }} slotProps={{ input: { "data-testid": "invc-search" } }} />
        <Select value={status || "all"} onChange={(_, v) => setStatus(v === "all" ? "" : (v as string))} sx={{ minWidth: 140 }} disabled={trash}>
          <Option value="all">All statuses</Option>
          {statuses.map((s) => (
            <Option key={s} value={s}>
              {s.replace("_", " ")}
            </Option>
          ))}
        </Select>
        <Button variant={trash ? "solid" : "outlined"} color="neutral" onClick={() => setTrash(!trash)} data-testid="invc-trash-toggle">
          Trash
        </Button>
        <Button variant="outlined" onClick={() => {
            auditExport("invoicing", `${kind}s.csv`, docs?.length ?? 0);
            downloadText(`${kind}s.csv`, docsCSV(docs ?? []));
          }} disabled={!docs?.length}>
          Export CSV
        </Button>
        {isInv && (
          <>
            <Button variant="outlined" onClick={() => fileRef.current?.click()}>
              Import CSV
            </Button>
            <input
              ref={fileRef}
              type="file"
              accept=".csv,text/csv"
              hidden
              onChange={(e) => {
                const f = e.target.files?.[0];
                e.target.value = "";
                if (f) importCSV(f);
              }}
            />
          </>
        )}
        {!isCn && (
          <Button onClick={() => setEditor({ editing: null })} data-testid="invc-new" sx={{ bgcolor: ACCENT, "&:hover": { bgcolor: "#9A3412" } }}>
            New {kind}
          </Button>
        )}
      </Box>
      {sel.length > 0 && !trash && (
        <Box sx={{ display: "flex", gap: 1, mb: 1, alignItems: "center", flexWrap: "wrap" }} data-testid="invc-bulkbar">
          <Typography level="body-sm">{sel.length} selected</Typography>
          <Button size="sm" variant="outlined" onClick={() => doBulk("send")} data-testid="invc-bulk-send">Email</Button>
          {isInv && <Button size="sm" variant="outlined" onClick={() => doBulk("remind")} data-testid="invc-bulk-remind">Send reminders</Button>}
          <Button size="sm" variant="outlined" onClick={() => doBulk("mark_sent")} data-testid="invc-bulk-marksent">Mark as sent</Button>
          <Button size="sm" variant="outlined" color="danger" onClick={() => doBulk("trash")} data-testid="invc-bulk-trash">Move to trash</Button>
          <Button size="sm" variant="plain" onClick={() => setSel([])}>Clear</Button>
        </Box>
      )}
      <ErrorAlert err={err} />
      {note && (
        <Typography level="body-sm" sx={{ mb: 1 }} data-testid="invc-note">
          {note}
        </Typography>
      )}
      {docs && docs.length === 0 ? (
        <Empty>{trash ? "Trash is empty." : isCn ? "No credit notes yet. Open an invoice and choose Issue credit note." : `No ${kind}s yet. Create your first one.`}</Empty>
      ) : (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table hoverRow data-testid="invc-table">
            <thead>
              <tr>
                <th style={{ width: 36 }}>
                  <Checkbox size="sm" checked={!!docs?.length && sel.length === docs.length} indeterminate={sel.length > 0 && sel.length < (docs?.length ?? 0)} onChange={(e) => setSel(e.target.checked ? (docs ?? []).map((d) => d.id) : [])} slotProps={{ input: { "aria-label": "Select all", "data-testid": "invc-select-all" } }} />
                </th>
                <th>Number</th>
                <th>Client</th>
                <th>Date</th>
                <th>{isInv ? "Due" : isCn ? "Against" : "Valid until"}</th>
                <th>Status</th>
                <th style={rightCell}>Total</th>
                {isInv && <th style={rightCell}>Balance</th>}
                {trash && <th />}
              </tr>
            </thead>
            <tbody>
              {(docs ?? []).map((d) => (
                <tr key={d.id} onClick={() => openDoc(d.id)} style={{ cursor: "pointer" }} data-testid="invc-row">
                  <td onClick={(e) => e.stopPropagation()}>
                    <Checkbox size="sm" checked={sel.includes(d.id)} onChange={() => toggle(d.id)} slotProps={{ input: { "aria-label": `Select ${d.number}`, "data-testid": "invc-select" } }} />
                  </td>
                  <td>{d.number}</td>
                  <td>{d.client_name || "-"}</td>
                  <td>{d.issue_date}</td>
                  <td>{isCn ? (d.source_ref ?? "").replace("invoice:", "") : (isInv ? d.due_date : d.valid_until) || "-"}</td>
                  <td>
                    <StatusChip status={d.display_status} />
                  </td>
                  <td style={rightCell}>{money(d.total, d.currency)}</td>
                  {isInv && <td style={rightCell}>{money(d.balance, d.currency)}</td>}
                  {trash && (
                    <td style={rightCell} onClick={(e) => e.stopPropagation()}>
                      <Button size="sm" variant="plain" onClick={async () => { await run(() => restoreDocument(d.id)); onChanged(); }}>
                        Restore
                      </Button>
                      <Button size="sm" variant="plain" color="danger" onClick={async () => { if (window.confirm(`Delete ${d.number} permanently?`)) { await run(() => purgeDocument(d.id)); onChanged(); } }}>
                        Delete forever
                      </Button>
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </Table>
          {next && (
            <Box sx={{ p: 1.5, textAlign: "center" }}>
              <Button size="sm" variant="outlined" loading={more} onClick={() => void loadMore()} data-testid="invc-load-more">
                Load more ({docs?.length ?? 0} shown)
              </Button>
            </Box>
          )}
        </Sheet>
      )}
      {detail && (
        <DocDetail
          key={detail.id}
          doc={detail}
          settings={settings}
          emailReady={emailReady}
          onClose={() => setDetail(null)}
          onChanged={onChanged}
          onEdit={(d) => {
            setDetail(null);
            setEditor({ editing: d });
          }}
          onOpen={openDoc}
          onOpenOrder={onOpenOrder}
        />
      )}
      {editor && (
        <DocEditor
          initial={editor.editing ? inputFromDoc(editor.editing) : blankDoc(kind === "credit_note" ? "invoice" : kind, settings.default_currency, settings.due_days, { notes: settings.default_notes, terms: settings.default_terms, tax_note: settings.tax_note })}
          editing={editor.editing}
          settings={settings}
          items={items}
          taxes={taxes}
          onClose={() => setEditor(null)}
          onSaved={(d) => {
            setEditor(null);
            onChanged();
            setDetail(d);
          }}
        />
      )}
    </Box>
  );
}
