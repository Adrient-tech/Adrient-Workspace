import { useCallback, useEffect, useRef, useState } from "react";
import { Box, Button, Checkbox, Chip, FormControl, FormLabel, Input, Modal, ModalClose, ModalDialog, Option, Select, Sheet, Table, Typography } from "@mui/joy";
import {
  addBankLine,
  applyRules,
  deleteRule,
  excludeBankLine,
  finishRecon,
  getReconDraft,
  importBankCSV,
  listBankLinesPage,
  listBankMatches,
  matchBankLine,
  listRules,
  reconHistory,
  saveRule,
  setBankLineCategory,
  undoBankLine,
  undoRecon,
} from "./api";
import { STATUS_COLOR, money, todayISO } from "./logic";
import type { BankMatch } from "./api";
import type { Account, BankLine, ReconDraft, Reconciliation, Rule } from "./types";
import { AccountSelect, Empty, ErrorAlert, isBank, rightCell, useErr } from "./ui";

type Tab = "review" | "rules" | "reconcile";

export function BankingView({ accounts, version, onChanged }: { accounts: Account[]; version: number; onChanged: () => void }) {
  const banks = accounts.filter(isBank);
  const [acct, setAcct] = useState(banks.find((a) => a.system_key === "bank")?.id ?? banks[0]?.id ?? "");
  const [tab, setTab] = useState<Tab>("review");
  return (
    <Box>
      <Box sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap", mb: 1.5 }}>
        <Typography level="h3" sx={{ mr: "auto" }}>
          Banking
        </Typography>
        <AccountSelect accounts={accounts} value={acct} onChange={setAcct} filter={isBank} testid="acct-bank-account" />
        {(["review", "rules", "reconcile"] as Tab[]).map((t) => (
          <Button key={t} size="sm" variant={tab === t ? "solid" : "plain"} onClick={() => setTab(t)} data-testid={`acct-bank-tab-${t}`}>
            {t === "review" ? "Statement lines" : t === "rules" ? "Bank rules" : "Reconcile"}
          </Button>
        ))}
      </Box>
      {tab === "review" && <Review accounts={accounts} acct={acct} version={version} onChanged={onChanged} />}
      {tab === "rules" && <Rules accounts={accounts} onChanged={onChanged} />}
      {tab === "reconcile" && <Reconcile acct={acct} accounts={accounts} version={version} onChanged={onChanged} />}
    </Box>
  );
}

function Review({ accounts, acct, version, onChanged }: { accounts: Account[]; acct: string; version: number; onChanged: () => void }) {
  const [lines, setLines] = useState<BankLine[] | null>(null);
  const [nextLines, setNextLines] = useState("");
  const [status, setStatus] = useState("pending");
  const [note, setNote] = useState("");
  const [matches, setMatches] = useState<Record<string, BankMatch>>({});
  const fileRef = useRef<HTMLInputElement>(null);
  const { err, run } = useErr();
  const load = useCallback(async () => {
    if (!acct) return;
    const r = await run(() => listBankLinesPage(acct, status));
    if (r) {
      setLines(r.items);
      setNextLines(r.next);
    }
  }, [acct, status, run]);
  useEffect(() => {
    load();
  }, [load, version]);
  useEffect(() => {
    if (!acct) return;
    listBankMatches(acct)
      .then((m) => setMatches(Object.fromEntries(m.map((x) => [x.line_id, x]))))
      .catch(() => setMatches({}));
  }, [acct, version, lines]);
  const importFile = async (f: File) => {
    const r = await run(async () => importBankCSV(acct, await f.text()));
    if (r) {
      setNote(`Imported ${r.imported} lines, skipped ${r.duplicates} already imported. Rules categorized ${r.categorized}, added ${r.added}.`);
      onChanged();
    }
  };
  const cats = accounts.filter((a) => a.id !== acct && a.active);
  if (!acct) return <Empty>Add a bank account in the chart of accounts first.</Empty>;
  return (
    <>
      <Box sx={{ display: "flex", gap: 1, mb: 1.5, flexWrap: "wrap", alignItems: "center" }}>
        <Select size="sm" value={status} onChange={(_, v) => setStatus((v as string) ?? "pending")} sx={{ minWidth: 140 }} slotProps={{ button: { "data-testid": "acct-bank-status" } as never }}>
          <Option value="pending">To review</Option>
          <Option value="added">In the books</Option>
          <Option value="matched">Matched to an existing entry</Option>
          <Option value="excluded">Excluded</Option>
          <Option value="">All</Option>
        </Select>
        <Button size="sm" variant="soft" onClick={() => fileRef.current?.click()} data-testid="acct-bank-import">
          Import CSV statement
        </Button>
        <input ref={fileRef} type="file" accept=".csv,text/csv" hidden data-testid="acct-bank-file" onChange={(e) => {
            const f = e.target.files?.[0];
            e.target.value = "";
            if (f) importFile(f);
          }}
        />
        <Button
          size="sm"
          variant="plain"
          onClick={async () => {
            const r = await run(() => applyRules());
            if (r) {
              setNote(`Rules categorized ${r.categorized}, added ${r.added}.`);
              onChanged();
            }
          }}
        >
          Apply rules
        </Button>
      </Box>
      {note && (
        <Typography level="body-sm" color="success" data-testid="acct-bank-note" sx={{ mb: 1 }}>
          {note}
        </Typography>
      )}
      <ErrorAlert err={err} />
      {lines && lines.length === 0 && <Empty>No statement lines here. Import a CSV (Date, Description, Debit/Credit or Amount columns).</Empty>}
      {lines && lines.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" data-testid="acct-bank-table">
            <thead>
              <tr>
                <th style={{ width: 105 }}>Date</th>
                <th>Description</th>
                <th style={{ ...rightCell, width: 120 }}>Amount</th>
                <th style={{ width: 230 }}>Category</th>
                <th style={{ width: 170 }} />
              </tr>
            </thead>
            <tbody>
              {lines.map((l) => (
                <tr key={l.id}>
                  <td>{l.date}</td>
                  <td>{l.description}</td>
                  <td style={{ ...rightCell, color: l.amount < 0 ? "var(--joy-palette-danger-600)" : "var(--joy-palette-success-700)" }}>{money(l.amount)}</td>
                  <td>
                    {l.status === "pending" ? (
                      <AccountSelect
                        accounts={cats}
                        value={l.category_id}
                        onChange={async (id) => {
                          await run(() => setBankLineCategory(l.id, id));
                          load();
                        }}
                        testid="acct-bank-line-category"
                      />
                    ) : (
                      <Chip size="sm" variant="soft" color={STATUS_COLOR[l.status]}>
                        {cats.find((a) => a.id === l.category_id)?.name ?? l.status}
                      </Chip>
                    )}
                  </td>
                  <td style={{ textAlign: "right" }}>
                    {l.status === "pending" && (
                      <>
                        {matches[l.id] && (
                          <Button
                            size="sm"
                            variant="soft"
                            color="success"
                            title={`${matches[l.id].memo} ${matches[l.id].contact} on ${matches[l.id].date}: ${matches[l.id].reasons.join(", ")}`}
                            data-testid="acct-bank-match"
                            onClick={async () => {
                              if (await run(() => matchBankLine(l.id, matches[l.id].journal_id))) onChanged();
                            }}
                          >
                            Match {matches[l.id].score}%
                          </Button>
                        )}{" "}
                        <Button
                          size="sm"
                          disabled={!l.category_id}
                          data-testid="acct-bank-add"
                          onClick={async () => {
                            if (await run(() => addBankLine(l.id, l.category_id))) onChanged();
                          }}
                        >
                          Add
                        </Button>{" "}
                        <Button
                          size="sm"
                          variant="plain"
                          color="neutral"
                          onClick={async () => {
                            if ((await run(() => excludeBankLine(l.id))) !== undefined) onChanged();
                          }}
                        >
                          Exclude
                        </Button>
                      </>
                    )}
                    {l.status !== "pending" && (
                      <Button
                        size="sm"
                        variant="plain"
                        onClick={async () => {
                          if ((await run(() => undoBankLine(l.id))) !== undefined) onChanged();
                        }}
                      >
                        Undo
                      </Button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
          {nextLines && (
            <Box sx={{ p: 1.5, textAlign: "center" }}>
              <Button
                size="sm"
                variant="outlined"
                data-testid="acct-bank-more"
                onClick={async () => {
                  const r = await run(() => listBankLinesPage(acct, status, Number(nextLines)));
                  if (r) {
                    setLines((l) => [...(l ?? []), ...r.items]);
                    setNextLines(r.next);
                  }
                }}
              >
                Load more ({lines?.length ?? 0} shown)
              </Button>
            </Box>
          )}
        </Sheet>
      )}
    </>
  );
}

function Rules({ accounts, onChanged }: { accounts: Account[]; onChanged: () => void }) {
  const [rules, setRules] = useState<Rule[]>([]);
  const [edit, setEdit] = useState<Partial<Rule> | null>(null);
  const { err, run } = useErr();
  const load = useCallback(async () => {
    const r = await run(() => listRules());
    if (r) setRules(r);
  }, [run]);
  useEffect(() => {
    load();
  }, [load]);
  const nameOf = (id: string) => accounts.find((a) => a.id === id)?.name ?? "";
  return (
    <>
      <Box sx={{ display: "flex", mb: 1.5 }}>
        <Typography level="body-sm" sx={{ flex: 1 }}>
          Rules categorize imported lines whose description contains the text. The lowest priority number wins.
        </Typography>
        <Button size="sm" onClick={() => setEdit({ direction: "any", priority: 100, auto_add: false })} data-testid="acct-new-rule">
          New rule
        </Button>
      </Box>
      <ErrorAlert err={err} />
      {rules.length === 0 ? (
        <Empty>No bank rules yet.</Empty>
      ) : (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" hoverRow data-testid="acct-rules-table">
            <thead>
              <tr>
                <th>If description contains</th>
                <th>Direction</th>
                <th>Assign to</th>
                <th style={rightCell}>Priority</th>
                <th>Auto-add</th>
              </tr>
            </thead>
            <tbody>
              {rules.map((r) => (
                <tr key={r.id} style={{ cursor: "pointer" }} onClick={() => setEdit(r)}>
                  <td>{r.contains}</td>
                  <td>{r.direction === "in" ? "Money in" : r.direction === "out" ? "Money out" : "Any"}</td>
                  <td>{nameOf(r.category_id)}</td>
                  <td style={rightCell}>{r.priority}</td>
                  <td>{r.auto_add ? "Yes" : "No"}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {edit && (
        <Modal open onClose={() => setEdit(null)}>
          <ModalDialog sx={{ width: "min(460px, 94vw)" }}>
            <ModalClose />
            <Typography level="h4">{edit.id ? "Edit rule" : "New bank rule"}</Typography>
            <FormControl>
              <FormLabel>Description contains</FormLabel>
              <Input value={edit.contains ?? ""} onChange={(e) => setEdit({ ...edit, contains: e.target.value })} data-testid="acct-rule-contains" autoFocus />
            </FormControl>
            <FormControl>
              <FormLabel>Applies to</FormLabel>
              <Select value={edit.direction ?? "any"} onChange={(_, v) => setEdit({ ...edit, direction: (v as Rule["direction"]) ?? "any" })}>
                <Option value="any">Money in and out</Option>
                <Option value="in">Money in</Option>
                <Option value="out">Money out</Option>
              </Select>
            </FormControl>
            <FormControl>
              <FormLabel>Assign to account</FormLabel>
              <AccountSelect accounts={accounts} value={edit.category_id ?? ""} onChange={(id) => setEdit({ ...edit, category_id: id })} filter={(a) => !isBank(a)} testid="acct-rule-category" />
            </FormControl>
            <FormControl>
              <FormLabel>Priority (lower first)</FormLabel>
              <Input type="number" value={edit.priority ?? 100} onChange={(e) => setEdit({ ...edit, priority: Number(e.target.value) || 100 })} />
            </FormControl>
            <Checkbox label="Add to the books automatically" checked={!!edit.auto_add} onChange={(e) => setEdit({ ...edit, auto_add: e.target.checked })} />
            <ErrorAlert err={err} />
            <Box sx={{ display: "flex", gap: 1, justifyContent: "flex-end" }}>
              {edit.id && (
                <Button
                  color="danger"
                  variant="plain"
                  sx={{ mr: "auto" }}
                  onClick={async () => {
                    if ((await run(() => deleteRule(edit.id!))) !== undefined) {
                      setEdit(null);
                      load();
                    }
                  }}
                >
                  Delete
                </Button>
              )}
              <Button variant="plain" color="neutral" onClick={() => setEdit(null)}>
                Cancel
              </Button>
              <Button
                data-testid="acct-rule-save"
                onClick={async () => {
                  if (await run(() => saveRule(edit))) {
                    setEdit(null);
                    load();
                    onChanged();
                  }
                }}
              >
                Save
              </Button>
            </Box>
          </ModalDialog>
        </Modal>
      )}
    </>
  );
}

function Reconcile({ acct, accounts, version, onChanged }: { acct: string; accounts: Account[]; version: number; onChanged: () => void }) {
  const [date, setDate] = useState(todayISO());
  const [ending, setEnding] = useState("");
  const [draft, setDraft] = useState<ReconDraft | null>(null);
  const [ticked, setTicked] = useState<Set<string>>(new Set());
  const [history, setHistory] = useState<Reconciliation[]>([]);
  const { err, run } = useErr();
  const load = useCallback(async () => {
    if (!acct) return;
    const d = await run(() => getReconDraft(acct, date));
    if (d) {
      setDraft(d);
      setTicked(new Set());
    }
    const h = await run(() => reconHistory(acct));
    if (h) setHistory(h);
  }, [acct, date, run]);
  useEffect(() => {
    load();
  }, [load, version]);
  const cleared = (draft?.lines ?? []).filter((l) => ticked.has(l.line_id)).reduce((s, l) => s + Math.round(l.amount * 100), 0) / 100;
  const calc = Math.round(((draft?.opening_balance ?? 0) + cleared) * 100) / 100;
  const diff = Math.round(((Number(ending) || 0) - calc) * 100) / 100;
  const acctName = accounts.find((a) => a.id === acct)?.name ?? "";
  return (
    <>
      <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap", alignItems: "flex-end", mb: 1.5 }}>
        <FormControl size="sm">
          <FormLabel>Statement date</FormLabel>
          <Input size="sm" type="date" value={date} onChange={(e) => setDate(e.target.value)} data-testid="acct-recon-date" />
        </FormControl>
        <FormControl size="sm">
          <FormLabel>Statement ending balance</FormLabel>
          <Input size="sm" type="number" slotProps={{ input: { step: "0.01", "data-testid": "acct-recon-ending" } }} value={ending} onChange={(e) => setEnding(e.target.value)} />
        </FormControl>
        <Typography level="body-sm">
          Reconciled so far {money(draft?.opening_balance ?? 0)} + ticked {money(cleared)} = {money(calc)}
        </Typography>
        <Typography level="title-sm" color={ending !== "" && diff === 0 ? "success" : "danger"} data-testid="acct-recon-diff">
          Difference {money(diff)}
        </Typography>
        <Button
          size="sm"
          disabled={ending === "" || diff !== 0}
          data-testid="acct-recon-finish"
          onClick={async () => {
            if (await run(() => finishRecon(acct, date, Number(ending), [...ticked]))) {
              setEnding("");
              onChanged();
              load();
            }
          }}
        >
          Finish reconciliation
        </Button>
      </Box>
      <ErrorAlert err={err} />
      {draft && draft.lines.length === 0 && <Empty>Nothing to reconcile in {acctName} up to this date.</Empty>}
      {draft && draft.lines.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto", mb: 2 }}>
          <Table size="sm" data-testid="acct-recon-table">
            <thead>
              <tr>
                <th style={{ width: 40 }}>
                  <Checkbox
                    size="sm"
                    slotProps={{ input: { "aria-label": "Tick all" } }}
                    checked={ticked.size === draft.lines.length}
                    onChange={(e) => setTicked(e.target.checked ? new Set(draft.lines.map((l) => l.line_id)) : new Set())}
                  />
                </th>
                <th style={{ width: 110 }}>Date</th>
                <th>Description</th>
                <th style={rightCell}>Amount</th>
              </tr>
            </thead>
            <tbody>
              {draft.lines.map((l) => (
                <tr key={l.line_id}>
                  <td>
                    <Checkbox
                      size="sm"
                      checked={ticked.has(l.line_id)}
                      slotProps={{ input: { "aria-label": `Clear ${l.memo}` } }}
                      onChange={(e) => {
                        const n = new Set(ticked);
                        if (e.target.checked) n.add(l.line_id);
                        else n.delete(l.line_id);
                        setTicked(n);
                      }}
                    />
                  </td>
                  <td>{l.date}</td>
                  <td>{l.memo}</td>
                  <td style={rightCell}>{money(l.amount)}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {history.length > 0 && (
        <>
          <Typography level="title-md" sx={{ mb: 0.5 }}>
            History
          </Typography>
          <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
            <Table size="sm">
              <tbody>
                {history.map((h, i) => (
                  <tr key={h.id}>
                    <td>{h.statement_date}</td>
                    <td style={rightCell}>{money(h.ending_balance)}</td>
                    <td>{h.cleared} lines</td>
                    <td style={{ textAlign: "right" }}>
                      {i === 0 && (
                        <Button
                          size="sm"
                          variant="plain"
                          color="danger"
                          onClick={async () => {
                            if ((await run(() => undoRecon(h.id))) !== undefined) {
                              onChanged();
                              load();
                            }
                          }}
                        >
                          Undo
                        </Button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </Sheet>
        </>
      )}
    </>
  );
}
