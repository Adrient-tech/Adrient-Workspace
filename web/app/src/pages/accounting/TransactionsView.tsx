import { useCallback, useEffect, useState } from "react";
import { DataTable, type Column, type TablePage, type TableQuery } from "../../components/DataTable";
import {
  Box,
  Button,
  Chip,
  FormControl,
  FormLabel,
  IconButton,
  Input,
  Modal,
  ModalClose,
  ModalDialog,
  Option,
  Select,
  Table,
  Typography,
} from "@mui/joy";
import DeleteOutline from "@mui/icons-material/DeleteOutline";
import { createJournal, deleteJournal, getJournal, listContacts, updateJournal } from "./api";
import { SOURCE_LABEL, balanceOf, emptyLine, money, todayISO } from "./logic";
import type { Account, ContactLite, Journal, JournalInput } from "./types";
import { AccountSelect, ContactInput, ErrorAlert, rightCell, useErr } from "./ui";

export function TransactionsView({
  accounts,
  version,
  onChanged,
  openJournalId,
  accountFilter,
}: {
  accounts: Account[];
  version: number;
  onChanged: () => void;
  openJournalId?: string;
  accountFilter?: string;
}) {
  const [q, setQ] = useState("");
  const [qd, setQd] = useState("");
  const [source, setSource] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [edit, setEdit] = useState<Journal | "new" | null>(null);
  const { err } = useErr();
  useEffect(() => {
    const t = setTimeout(() => setQd(q), 250);
    return () => clearTimeout(t);
  }, [q]);
  // Keyset-paged on the server (GET /journals?cursor=), 50 per page by default.
  const load = useCallback(
    async (t: TableQuery, signal: AbortSignal): Promise<TablePage<Journal>> => {
      const p = new URLSearchParams({ limit: String(t.limit) });
      if (t.cursor) p.set("cursor", t.cursor);
      if (qd) p.set("q", qd);
      if (source) p.set("source", source);
      if (from) p.set("from", from);
      if (to) p.set("to", to);
      if (accountFilter) p.set("account_id", accountFilter);
      const r = await fetch(`/api/v1/accounting/journals?${p}`, { credentials: "same-origin", signal });
      if (!r.ok) throw new Error((await r.text()).trim() || `HTTP ${r.status}`);
      const d = (await r.json()) as { items?: Journal[]; journals?: Journal[]; next_cursor?: string };
      return { items: d.items ?? d.journals ?? [], next_cursor: d.next_cursor };
    },
    // version forces a reload after changes elsewhere
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [qd, source, from, to, accountFilter, version],
  );
  const [tick, setTick] = useState(0);
  const columns: Column<Journal>[] = [
    { key: "num", header: "#", width: 70, render: (j) => j.number },
    { key: "date", header: "Date", width: 110, render: (j) => j.date },
    { key: "desc", header: "Description", render: (j) => <>{j.memo || "(no memo)"} {j.ref && <Typography level="body-xs" component="span">{j.ref}</Typography>}</> },
    { key: "contact", header: "Contact", render: (j) => j.contact_name },
    {
      key: "source",
      header: "Source",
      width: 150,
      render: (j) => (
        <>
          <Chip size="sm" variant="soft" color={j.source === "manual" ? "neutral" : "success"}>
            {SOURCE_LABEL[j.source] ?? j.source}
          </Chip>
          {j.voided && (
            <Chip size="sm" color="danger" variant="soft" sx={{ ml: 0.5 }}>
              void
            </Chip>
          )}
        </>
      ),
    },
    { key: "amount", header: "Amount", width: 130, render: (j) => <Box sx={{ textAlign: "right" }}>{money(j.total)}</Box> },
  ];
  useEffect(() => {
    if (openJournalId) getJournal(openJournalId).then(setEdit).catch(() => undefined);
  }, [openJournalId]);

  return (
    <Box>
      <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", alignItems: "flex-end", mb: 1.5 }}>
        <Typography level="h3" sx={{ mr: "auto" }}>
          Transactions
        </Typography>
        <Input size="sm" placeholder="Search memo, ref, contact, #" value={q} onChange={(e) => setQ(e.target.value)} data-testid="acct-journal-search" />
        <Select size="sm" value={source} onChange={(_, v) => setSource((v as string) ?? "")} sx={{ minWidth: 150 }} slotProps={{ button: { "data-testid": "acct-source-filter" } as never }}>
          <Option value="">All sources</Option>
          {Object.entries(SOURCE_LABEL).map(([k, v]) => (
            <Option key={k} value={k}>
              {v}
            </Option>
          ))}
        </Select>
        <Input size="sm" type="date" value={from} onChange={(e) => setFrom(e.target.value)} aria-label="From date" />
        <Input size="sm" type="date" value={to} onChange={(e) => setTo(e.target.value)} aria-label="To date" />
        <Button size="sm" onClick={() => setEdit("new")} data-testid="acct-new-journal">
          New journal entry
        </Button>
      </Box>
      <ErrorAlert err={err} />
      <DataTable<Journal>
        testId="acct-journal-table"
        columns={columns}
        rowKey={(j) => j.id}
        load={load}
        mode="cursor"
        search={false}
        urlPrefix="tx_"
        reloadKey={tick}
        onRowClick={(j) => setEdit(j)}
        emptyText="No transactions yet. Sync from apps, add an expense, or create a journal entry."
      />
      {edit && (
        <JournalDialog
          journal={edit === "new" ? null : edit}
          accounts={accounts}
          onClose={() => setEdit(null)}
          onSaved={() => {
            setEdit(null);
            setTick((n) => n + 1);
            onChanged();
          }}
        />
      )}
    </Box>
  );
}

function JournalDialog({ journal, accounts, onClose, onSaved }: { journal: Journal | null; accounts: Account[]; onClose: () => void; onSaved: () => void }) {
  const managed = !!journal && journal.source !== "manual";
  const [date, setDate] = useState(journal?.date ?? todayISO());
  const [memo, setMemo] = useState(journal?.memo ?? "");
  const [ref, setRef] = useState(journal?.ref ?? "");
  const [contactId, setContactId] = useState(journal?.contact_id ?? "");
  const [contactName, setContactName] = useState(journal?.contact_name ?? "");
  const [lines, setLines] = useState(journal?.lines.length ? journal.lines.map((l) => ({ ...l })) : [emptyLine(), emptyLine()]);
  const [contacts, setContacts] = useState<ContactLite[]>([]);
  const { err, run } = useErr();
  useEffect(() => {
    listContacts().then(setContacts);
  }, []);
  const bal = balanceOf(lines);
  const setLine = (i: number, p: Partial<(typeof lines)[number]>) => setLines(lines.map((l, j) => (j === i ? { ...l, ...p } : l)));
  const save = async () => {
    const input: JournalInput = { date, memo, ref, contact_id: contactId, contact_name: contactName, lines: lines.filter((l) => l.debit || l.credit) };
    const ok = await run(() => (journal ? updateJournal(journal.id, input) : createJournal(input)));
    if (ok) onSaved();
  };
  const remove = async () => {
    if (!journal || !window.confirm("Delete this journal entry?")) return;
    if ((await run(() => deleteJournal(journal.id))) !== undefined) onSaved();
  };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: "min(920px, 96vw)", maxHeight: "92vh", overflow: "auto" }} data-testid="acct-journal-dialog">
        <ModalClose />
        <Typography level="h4">{journal ? `Journal #${journal.number}` : "New journal entry"}</Typography>
        {managed && (
          <Typography level="body-sm" color="warning">
            Posted automatically from {SOURCE_LABEL[journal!.source] ?? journal!.source}. Change it in the source app and sync again.
          </Typography>
        )}
        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "140px 1fr 160px 1fr" }, gap: 1 }}>
          <FormControl>
            <FormLabel>Date</FormLabel>
            <Input size="sm" type="date" value={date} disabled={managed} onChange={(e) => setDate(e.target.value)} data-testid="acct-journal-date" />
          </FormControl>
          <FormControl>
            <FormLabel>Memo</FormLabel>
            <Input size="sm" value={memo} disabled={managed} onChange={(e) => setMemo(e.target.value)} data-testid="acct-journal-memo" />
          </FormControl>
          <FormControl>
            <FormLabel>Reference</FormLabel>
            <Input size="sm" value={ref} disabled={managed} onChange={(e) => setRef(e.target.value)} />
          </FormControl>
          <FormControl>
            <FormLabel>Contact</FormLabel>
            {managed ? (
              <Input size="sm" value={contactName} disabled />
            ) : (
              <ContactInput contacts={contacts} id={contactId} name={contactName} onChange={(i, n) => (setContactId(i), setContactName(n))} />
            )}
          </FormControl>
        </Box>
        <Table size="sm" sx={{ mt: 1 }}>
          <thead>
            <tr>
              <th>Account</th>
              <th>Memo</th>
              <th style={{ ...rightCell, width: 120 }}>Debit</th>
              <th style={{ ...rightCell, width: 120 }}>Credit</th>
              {!managed && <th style={{ width: 40 }} />}
            </tr>
          </thead>
          <tbody>
            {lines.map((l, i) => (
              <tr key={i}>
                <td>
                  {managed ? (
                    l.account_name
                  ) : (
                    <AccountSelect accounts={accounts} value={l.account_id} onChange={(id) => setLine(i, { account_id: id })} testid={`acct-line-account-${i}`} />
                  )}
                </td>
                <td>
                  <Input size="sm" value={l.memo} disabled={managed} onChange={(e) => setLine(i, { memo: e.target.value })} />
                </td>
                <td>
                  <Input
                    size="sm"
                    type="number"
                    slotProps={{ input: { step: "0.01", min: 0, style: { textAlign: "right" }, "data-testid": `acct-line-debit-${i}` } }}
                    value={l.debit || ""}
                    disabled={managed}
                    onChange={(e) => setLine(i, { debit: Number(e.target.value) || 0, credit: Number(e.target.value) ? 0 : l.credit })}
                  />
                </td>
                <td>
                  <Input
                    size="sm"
                    type="number"
                    slotProps={{ input: { step: "0.01", min: 0, style: { textAlign: "right" }, "data-testid": `acct-line-credit-${i}` } }}
                    value={l.credit || ""}
                    disabled={managed}
                    onChange={(e) => setLine(i, { credit: Number(e.target.value) || 0, debit: Number(e.target.value) ? 0 : l.debit })}
                  />
                </td>
                {!managed && (
                  <td>
                    <IconButton size="sm" aria-label="Remove line" disabled={lines.length <= 2} onClick={() => setLines(lines.filter((_, j) => j !== i))}>
                      <DeleteOutline />
                    </IconButton>
                  </td>
                )}
              </tr>
            ))}
          </tbody>
          <tfoot>
            <tr>
              <td colSpan={2} style={rightCell}>
                Totals
              </td>
              <td style={rightCell} data-testid="acct-total-debit">
                {money(bal.debit)}
              </td>
              <td style={rightCell} data-testid="acct-total-credit">
                {money(bal.credit)}
              </td>
              {!managed && <td />}
            </tr>
          </tfoot>
        </Table>
        {!managed && (
          <Box sx={{ display: "flex", gap: 1, alignItems: "center" }}>
            <Button size="sm" variant="plain" onClick={() => setLines([...lines, emptyLine()])}>
              Add line
            </Button>
            <Typography level="body-sm" color={bal.balanced ? "success" : "danger"} data-testid="acct-balance-note">
              {bal.balanced ? "Balanced" : `Out of balance by ${money(Math.abs(bal.diff))}`}
            </Typography>
          </Box>
        )}
        <ErrorAlert err={err} />
        <Box sx={{ display: "flex", gap: 1, justifyContent: "flex-end", mt: 1 }}>
          {journal && !managed && (
            <Button color="danger" variant="plain" onClick={remove} sx={{ mr: "auto" }}>
              Delete
            </Button>
          )}
          <Button variant="plain" color="neutral" onClick={onClose}>
            {managed ? "Close" : "Cancel"}
          </Button>
          {!managed && (
            <Button onClick={save} disabled={!bal.balanced} data-testid="acct-journal-save">
              Save
            </Button>
          )}
        </Box>
      </ModalDialog>
    </Modal>
  );
}
