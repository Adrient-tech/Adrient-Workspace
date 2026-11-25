import { useCallback, useEffect, useMemo, useState } from "react";
import { Box, Button, Checkbox, Chip, FormControl, FormLabel, Input, Modal, ModalDialog, Sheet, Tab, TabList, Tabs, Table, Typography } from "@mui/joy";
import {
  billAction,
  createDeposit,
  creditMemoAction,
  deleteBillPayment,
  deleteDeposit,
  listBills,
  listCreditMemos,
  listDeposits,
  listReceipts,
  saveBill,
  saveCreditMemo,
  writeOffReceivable,
} from "./api2";
import type { Bill, BillLine, CreditMemo, Deposit, Receipt } from "./api2";
import { ACCENT, money, todayISO } from "./logic";
import type { Account, Settings } from "./types";
import { AccountSelect, Empty, ErrorAlert, isBank, rightCell, useErr } from "./ui";

const statusColor = (s: string): "success" | "warning" | "danger" | "neutral" | "primary" =>
  s === "paid" ? "success" : s === "pending_approval" ? "warning" : s === "rejected" || s === "void" ? "danger" : s === "draft" ? "neutral" : "primary";

export function BillsView({ accounts, settings, version, onChanged, canApprove }: { accounts: Account[]; settings: Settings; version: number; onChanged: () => void; canApprove: boolean }) {
  const [tab, setTab] = useState<string>("bills");
  return (
    <Box>
      <Typography level="h3" sx={{ mb: 1 }}>
        Bills, credits and deposits
      </Typography>
      <Tabs value={tab} onChange={(_, v) => setTab(String(v))} sx={{ bgcolor: "transparent" }}>
        <TabList>
          <Tab value="bills" data-testid="acct-tab-bills">
            Bills
          </Tab>
          <Tab value="vendor_credit" data-testid="acct-tab-vcredits">
            Vendor credits
          </Tab>
          <Tab value="memos" data-testid="acct-tab-memos">
            Customer credit memos
          </Tab>
          <Tab value="deposits" data-testid="acct-tab-deposits">
            Deposits
          </Tab>
        </TabList>
      </Tabs>
      <Box sx={{ mt: 1.5 }}>
        {(tab === "bills" || tab === "vendor_credit") && <BillList kind={tab === "bills" ? "bill" : "vendor_credit"} accounts={accounts} settings={settings} version={version} onChanged={onChanged} canApprove={canApprove} />}
        {tab === "memos" && <Memos accounts={accounts} version={version} onChanged={onChanged} />}
        {tab === "deposits" && <Deposits accounts={accounts} settings={settings} version={version} onChanged={onChanged} />}
      </Box>
    </Box>
  );
}

function BillList({ kind, accounts, settings, version, onChanged, canApprove }: { kind: "bill" | "vendor_credit"; accounts: Account[]; settings: Settings; version: number; onChanged: () => void; canApprove: boolean }) {
  const [rows, setRows] = useState<Bill[] | null>(null);
  const [edit, setEdit] = useState(false);
  const [open, setOpen] = useState<Bill | null>(null);
  const { err, run } = useErr();
  const load = useCallback(async () => {
    const r = await run(() => listBills(kind));
    if (r) setRows(r);
  }, [kind, run]);
  useEffect(() => {
    load();
  }, [load, version]);
  const noun = kind === "bill" ? "bill" : "vendor credit";
  return (
    <Box>
      <Box sx={{ display: "flex", mb: 1, gap: 1, alignItems: "center" }}>
        <Typography level="body-sm" sx={{ flex: 1 }}>
          {kind === "bill"
            ? settings.approval_limit > 0
              ? `Bills over ${money(settings.approval_limit)} need approval before they are posted.`
              : "Bills post to Accounts Payable with input GST."
            : "A vendor credit (debit note) reduces what you owe a vendor. Apply it to a bill or take a refund."}
        </Typography>
        <Button size="sm" onClick={() => setEdit(true)} data-testid={kind === "bill" ? "acct-new-bill" : "acct-new-vcredit"}>
          New {noun}
        </Button>
      </Box>
      <ErrorAlert err={err} />
      {rows && rows.length === 0 && <Empty>No {noun}s yet.</Empty>}
      {rows && rows.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" hoverRow data-testid="acct-bills-table">
            <thead>
              <tr>
                <th>Date</th>
                <th>Number</th>
                <th>Vendor</th>
                <th>Status</th>
                <th style={rightCell}>Total</th>
                <th style={rightCell}>Balance</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((b) => (
                <tr key={b.id} onClick={() => setOpen(b)} style={{ cursor: "pointer" }}>
                  <td>{b.bill_date}</td>
                  <td>{b.number || "—"}</td>
                  <td>{b.vendor_name}</td>
                  <td>
                    <Chip size="sm" variant="soft" color={statusColor(b.status)}>
                      {b.status.replace("_", " ")}
                    </Chip>
                  </td>
                  <td style={rightCell}>{money(b.total, b.currency || settings.currency)}</td>
                  <td style={rightCell}>{b.status === "open" ? money(b.balance, b.currency || settings.currency) : ""}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {edit && (
        <BillEditor
          kind={kind}
          accounts={accounts}
          settings={settings}
          onClose={() => setEdit(false)}
          onSaved={() => {
            setEdit(false);
            load();
            onChanged();
          }}
        />
      )}
      {open && (
        <BillDetail
          bill={open}
          accounts={accounts}
          settings={settings}
          canApprove={canApprove}
          onClose={() => setOpen(null)}
          onChanged={(b) => {
            setOpen(b);
            load();
            onChanged();
          }}
        />
      )}
    </Box>
  );
}

function BillEditor({ kind, accounts, settings, onClose, onSaved }: { kind: "bill" | "vendor_credit"; accounts: Account[]; settings: Settings; onClose: () => void; onSaved: () => void }) {
  const [vendor, setVendor] = useState("");
  const [number, setNumber] = useState("");
  const [date, setDate] = useState(todayISO());
  const [due, setDue] = useState("");
  const [currency, setCurrency] = useState(settings.currency);
  const [notes, setNotes] = useState("");
  const [lines, setLines] = useState<Partial<BillLine>[]>([{ account_id: "", description: "", amount: 0, tax_rate: 18, class: "" }]);
  const { err, run } = useErr();
  const sub = lines.reduce((s, l) => s + (l.amount || 0), 0);
  const tax = lines.reduce((s, l) => s + ((l.amount || 0) * (l.tax_rate || 0)) / 100, 0);
  const expenseOnly = (a: Account) => a.type === "expense" || a.type === "asset" || a.type === "liability";
  const set = (i: number, p: Partial<BillLine>) => setLines((ls) => ls.map((l, j) => (j === i ? { ...l, ...p } : l)));
  const submit = async (draft: boolean) => {
    const r = await run(() => saveBill({ kind, number, vendor_name: vendor, bill_date: date, due_date: due, currency: currency === settings.currency ? "" : currency, lines, notes, draft }));
    if (r) onSaved();
  };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 820, maxWidth: "96vw", maxHeight: "92vh", overflow: "auto" }}>
        <Typography level="title-lg">New {kind === "bill" ? "bill" : "vendor credit"}</Typography>
        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr 1fr" }, gap: 1.5 }}>
          <FormControl>
            <FormLabel>Vendor</FormLabel>
            <Input value={vendor} onChange={(e) => setVendor(e.target.value)} slotProps={{ input: { "data-testid": "acct-bill-vendor" } }} />
          </FormControl>
          <FormControl>
            <FormLabel>{kind === "bill" ? "Bill number" : "Credit note number"}</FormLabel>
            <Input value={number} onChange={(e) => setNumber(e.target.value)} slotProps={{ input: { "data-testid": "acct-bill-number" } }} />
          </FormControl>
          <FormControl>
            <FormLabel>Currency</FormLabel>
            <Input value={currency} onChange={(e) => setCurrency(e.target.value.toUpperCase())} slotProps={{ input: { "data-testid": "acct-bill-currency" } }} />
          </FormControl>
          <FormControl>
            <FormLabel>Date</FormLabel>
            <Input type="date" value={date} onChange={(e) => setDate(e.target.value)} slotProps={{ input: { "data-testid": "acct-bill-date" } }} />
          </FormControl>
          <FormControl>
            <FormLabel>Due date</FormLabel>
            <Input type="date" value={due} onChange={(e) => setDue(e.target.value)} />
          </FormControl>
        </Box>
        {currency !== settings.currency && <Typography level="body-xs">The exchange rate for {currency} on the bill date comes from the Currencies page. Add one there first.</Typography>}
        <Table size="sm">
          <thead>
            <tr>
              <th style={{ width: 230 }}>Account</th>
              <th>Description</th>
              <th style={{ width: 110 }}>Amount</th>
              <th style={{ width: 80 }}>GST %</th>
              <th style={{ width: 100 }}>Class</th>
              <th style={{ width: 30 }} />
            </tr>
          </thead>
          <tbody>
            {lines.map((l, i) => (
              <tr key={i}>
                <td>
                  <AccountSelect accounts={accounts} value={l.account_id || ""} onChange={(id) => set(i, { account_id: id })} filter={expenseOnly} testid={`acct-bill-acct-${i}`} />
                </td>
                <td>
                  <Input size="sm" value={l.description || ""} onChange={(e) => set(i, { description: e.target.value })} />
                </td>
                <td>
                  <Input size="sm" type="number" value={l.amount || ""} onChange={(e) => set(i, { amount: Number(e.target.value) })} slotProps={{ input: { "data-testid": `acct-bill-amt-${i}` } }} />
                </td>
                <td>
                  <Input size="sm" type="number" value={l.tax_rate ?? 0} onChange={(e) => set(i, { tax_rate: Number(e.target.value) })} />
                </td>
                <td>
                  <Input size="sm" value={l.class || ""} onChange={(e) => set(i, { class: e.target.value })} />
                </td>
                <td>
                  {lines.length > 1 && (
                    <Button size="sm" variant="plain" color="neutral" onClick={() => setLines(lines.filter((_, j) => j !== i))} aria-label="Remove line">
                      ✕
                    </Button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
        <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
          <Button size="sm" variant="plain" onClick={() => setLines([...lines, { account_id: "", description: "", amount: 0, tax_rate: 18, class: "" }])}>
            Add line
          </Button>
          <Typography level="body-sm">
            Subtotal {money(sub, currency)} + GST {money(tax, currency)} = <b>{money(sub + tax, currency)}</b>
          </Typography>
        </Box>
        <Input placeholder="Notes" value={notes} onChange={(e) => setNotes(e.target.value)} />
        <ErrorAlert err={err} />
        <Box sx={{ display: "flex", gap: 1, justifyContent: "flex-end" }}>
          <Button variant="plain" color="neutral" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="soft" onClick={() => submit(true)}>
            Save as draft
          </Button>
          <Button onClick={() => submit(false)} data-testid="acct-bill-save">
            Save and post
          </Button>
        </Box>
      </ModalDialog>
    </Modal>
  );
}

function BillDetail({ bill, accounts, settings, canApprove, onClose, onChanged }: { bill: Bill; accounts: Account[]; settings: Settings; canApprove: boolean; onClose: () => void; onChanged: (b: Bill) => void }) {
  const [payAmt, setPayAmt] = useState(bill.balance);
  const [payAcct, setPayAcct] = useState("");
  const [payDate, setPayDate] = useState(todayISO());
  const [note, setNote] = useState("");
  const [credits, setCredits] = useState<Bill[]>([]);
  const { err, run } = useErr();
  const cur = bill.currency || settings.currency;
  useEffect(() => {
    if (bill.kind === "bill" && bill.status === "open") listBills("vendor_credit", "").then((l) => setCredits(l.filter((c) => c.status === "open" && c.balance > 0 && c.vendor_name === bill.vendor_name && c.currency === bill.currency))).catch(() => undefined);
  }, [bill]);
  const act = async (action: string, body: unknown = {}) => {
    const r = await run(() => billAction(bill.id, action, body));
    if (r) onChanged(r);
  };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 720, maxWidth: "96vw", maxHeight: "92vh", overflow: "auto" }} data-testid="acct-bill-detail">
        <Typography level="title-lg">
          {bill.kind === "bill" ? "Bill" : "Vendor credit"} {bill.number} · {bill.vendor_name}
        </Typography>
        <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", alignItems: "center" }}>
          <Chip color={statusColor(bill.status)} variant="soft">
            {bill.status.replace("_", " ")}
          </Chip>
          <Chip variant="soft">Date {bill.bill_date}</Chip>
          {bill.currency && <Chip variant="soft">1 {bill.currency} = {bill.rate} {settings.currency}</Chip>}
          {bill.approved_by && <Chip variant="soft">Approved by {bill.approved_by}</Chip>}
        </Box>
        <Table size="sm">
          <tbody>
            {bill.lines.map((l, i) => (
              <tr key={i}>
                <td>{accounts.find((a) => a.id === l.account_id)?.name ?? ""}</td>
                <td>{l.description}</td>
                <td>{l.class}</td>
                <td style={rightCell}>{money(l.amount, cur)}</td>
                <td style={rightCell}>{l.tax_rate}%</td>
              </tr>
            ))}
            <tr>
              <td colSpan={3}>
                <b>Total (GST {money(bill.tax_total, cur)})</b>
              </td>
              <td style={rightCell}>
                <b>{money(bill.total, cur)}</b>
              </td>
              <td />
            </tr>
            <tr>
              <td colSpan={3}>Paid or credited</td>
              <td style={rightCell}>{money(bill.paid, cur)}</td>
              <td />
            </tr>
          </tbody>
        </Table>
        {bill.payments.length > 0 && (
          <Box>
            <Typography level="title-sm">Payments</Typography>
            {bill.payments.map((p) => (
              <Box key={p.id} sx={{ display: "flex", gap: 1, alignItems: "center", fontSize: "sm" }}>
                <span>{p.date}</span>
                <span>{money(p.amount, cur)}</span>
                <span>{p.reference}</span>
                {bill.status !== "void" && (
                  <Button size="sm" variant="plain" color="danger" onClick={async () => { const r = await run(() => deleteBillPayment(bill.id, p.id)); if (r) onChanged(r); }}>
                    Delete
                  </Button>
                )}
              </Box>
            ))}
          </Box>
        )}
        <ErrorAlert err={err} />
        {bill.status === "pending_approval" && (
          <Sheet variant="soft" color="warning" sx={{ p: 1.5, borderRadius: "md" }}>
            <Typography level="body-sm" sx={{ mb: 1 }}>
              This bill is above the approval limit and is not posted yet.
            </Typography>
            {canApprove ? (
              <Box sx={{ display: "flex", gap: 1 }}>
                <Input size="sm" placeholder="Note" value={note} onChange={(e) => setNote(e.target.value)} sx={{ flex: 1 }} />
                <Button size="sm" color="success" onClick={() => act("approve", { note })} data-testid="acct-bill-approve">
                  Approve and post
                </Button>
                <Button size="sm" color="danger" variant="soft" onClick={() => act("reject", { note })}>
                  Reject
                </Button>
              </Box>
            ) : (
              <Typography level="body-xs">You need the accounting approver role to approve it.</Typography>
            )}
          </Sheet>
        )}
        {(bill.status === "draft" || bill.status === "rejected") && <Button onClick={() => act("post")}>Post this {bill.kind === "bill" ? "bill" : "credit"}</Button>}
        {bill.status === "open" && (
          <Sheet variant="outlined" sx={{ p: 1.5, borderRadius: "md", display: "grid", gap: 1 }}>
            <Typography level="title-sm">{bill.kind === "bill" ? "Record a payment" : "Record a refund from the vendor"}</Typography>
            <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>
              <Input size="sm" type="number" value={payAmt} onChange={(e) => setPayAmt(Number(e.target.value))} sx={{ width: 130 }} slotProps={{ input: { "data-testid": "acct-pay-amount" } }} />
              <Input size="sm" type="date" value={payDate} onChange={(e) => setPayDate(e.target.value)} slotProps={{ input: { "data-testid": "acct-pay-date" } }} />
              <AccountSelect accounts={accounts} value={payAcct} onChange={setPayAcct} filter={isBank} placeholder="Bank or cash" testid="acct-pay-account" />
              <Button size="sm" onClick={() => act("pay", { amount: payAmt, date: payDate, account_id: payAcct })} data-testid="acct-pay-save">
                Record
              </Button>
            </Box>
            {bill.currency && <Typography level="body-xs">Paid at the {bill.currency} rate on the payment date. The difference from the booking rate is booked as realised exchange gain or loss.</Typography>}
            {credits.length > 0 && (
              <Box>
                <Typography level="title-sm">Apply a vendor credit</Typography>
                {credits.map((c) => (
                  <Box key={c.id} sx={{ display: "flex", gap: 1, alignItems: "center" }}>
                    <span>
                      {c.number} · {money(c.balance, cur)} available
                    </span>
                    <Button size="sm" variant="soft" onClick={() => act("apply-credit", { credit_id: c.id, date: payDate, amount: Math.min(c.balance, bill.balance) })}>
                      Apply {money(Math.min(c.balance, bill.balance), cur)}
                    </Button>
                  </Box>
                ))}
              </Box>
            )}
            <Box sx={{ display: "flex", gap: 1 }}>
              <Button size="sm" variant="plain" color="neutral" onClick={() => act("write-off", { date: payDate, note: "write-off" })}>
                Write off the balance
              </Button>
              {bill.paid === 0 && (
                <Button size="sm" variant="plain" color="danger" onClick={() => act("void")}>
                  Void
                </Button>
              )}
            </Box>
          </Sheet>
        )}
        <Box sx={{ display: "flex", justifyContent: "flex-end" }}>
          <Button variant="plain" color="neutral" onClick={onClose}>
            Close
          </Button>
        </Box>
      </ModalDialog>
    </Modal>
  );
}

function Memos({ accounts, version, onChanged }: { accounts: Account[]; version: number; onChanged: () => void }) {
  const [rows, setRows] = useState<CreditMemo[] | null>(null);
  const [form, setForm] = useState({ number: "", customer_name: "", date: todayISO(), amount: 0, gst_rate: 18, reason: "", currency: "" });
  const [show, setShow] = useState(false);
  const [refund, setRefund] = useState<CreditMemo | null>(null);
  const [refundAcct, setRefundAcct] = useState("");
  const [wo, setWo] = useState({ customer_name: "", amount: 0, note: "" });
  const { err, run } = useErr();
  const load = useCallback(async () => {
    const r = await run(() => listCreditMemos());
    if (r) setRows(r);
  }, [run]);
  useEffect(() => {
    load();
  }, [load, version]);
  return (
    <Box>
      <Box sx={{ display: "flex", gap: 1, mb: 1, alignItems: "center" }}>
        <Typography level="body-sm" sx={{ flex: 1 }}>
          A credit memo reduces what a customer owes (Dr Sales Returns and GST, Cr Receivable). Refund it in cash or leave it to net against their next invoice.
        </Typography>
        <Button size="sm" onClick={() => setShow(true)} data-testid="acct-new-memo">
          New credit memo
        </Button>
      </Box>
      <ErrorAlert err={err} />
      {rows && rows.length === 0 && <Empty>No credit memos yet.</Empty>}
      {rows && rows.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto", mb: 2 }}>
          <Table size="sm" data-testid="acct-memos-table">
            <thead>
              <tr>
                <th>Date</th>
                <th>Number</th>
                <th>Customer</th>
                <th style={rightCell}>Total</th>
                <th style={rightCell}>Refunded</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {rows.map((m) => (
                <tr key={m.id}>
                  <td>{m.date}</td>
                  <td>{m.number}</td>
                  <td>{m.customer_name}</td>
                  <td style={rightCell}>{money(m.total)}</td>
                  <td style={rightCell}>{money(m.refunded)}</td>
                  <td>
                    {m.status === "open" && m.balance > 0 && (
                      <Button size="sm" variant="soft" onClick={() => { setRefund(m); setRefundAcct(""); }}>
                        Refund
                      </Button>
                    )}{" "}
                    {m.status === "open" && m.refunded === 0 && (
                      <Button size="sm" variant="plain" color="danger" onClick={async () => { await run(() => creditMemoAction(m.id, "void")); load(); onChanged(); }}>
                        Void
                      </Button>
                    )}
                    {m.status === "void" && <Chip size="sm">void</Chip>}
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      <Sheet variant="outlined" sx={{ p: 1.5, borderRadius: "md", maxWidth: 560 }}>
        <Typography level="title-sm" sx={{ mb: 1 }}>
          Write off a customer balance (bad debt)
        </Typography>
        <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>
          <Input size="sm" placeholder="Customer" value={wo.customer_name} onChange={(e) => setWo({ ...wo, customer_name: e.target.value })} />
          <Input size="sm" type="number" placeholder="Amount" value={wo.amount || ""} onChange={(e) => setWo({ ...wo, amount: Number(e.target.value) })} sx={{ width: 120 }} />
          <Button
            size="sm"
            variant="soft"
            color="danger"
            onClick={async () => {
              if (await run(() => writeOffReceivable({ customer_name: wo.customer_name, amount: wo.amount, note: wo.note }))) {
                setWo({ customer_name: "", amount: 0, note: "" });
                onChanged();
              }
            }}
          >
            Write off
          </Button>
        </Box>
      </Sheet>
      {show && (
        <Modal open onClose={() => setShow(false)}>
          <ModalDialog sx={{ width: 520, maxWidth: "96vw" }}>
            <Typography level="title-lg">New credit memo</Typography>
            <Input placeholder="Customer" value={form.customer_name} onChange={(e) => setForm({ ...form, customer_name: e.target.value })} slotProps={{ input: { "data-testid": "acct-memo-customer" } }} />
            <Input placeholder="Credit memo number" value={form.number} onChange={(e) => setForm({ ...form, number: e.target.value })} />
            <Input type="date" value={form.date} onChange={(e) => setForm({ ...form, date: e.target.value })} />
            <Box sx={{ display: "flex", gap: 1 }}>
              <Input type="number" placeholder="Amount before GST" value={form.amount || ""} onChange={(e) => setForm({ ...form, amount: Number(e.target.value) })} slotProps={{ input: { "data-testid": "acct-memo-amount" } }} sx={{ flex: 1 }} />
              <Input type="number" placeholder="GST %" value={form.gst_rate} onChange={(e) => setForm({ ...form, gst_rate: Number(e.target.value) })} sx={{ width: 100 }} />
            </Box>
            <Input placeholder="Reason" value={form.reason} onChange={(e) => setForm({ ...form, reason: e.target.value })} />
            <Box sx={{ display: "flex", gap: 1, justifyContent: "flex-end" }}>
              <Button variant="plain" color="neutral" onClick={() => setShow(false)}>
                Cancel
              </Button>
              <Button
                data-testid="acct-memo-save"
                onClick={async () => {
                  if (await run(() => saveCreditMemo(form))) {
                    setShow(false);
                    load();
                    onChanged();
                  }
                }}
              >
                Create
              </Button>
            </Box>
          </ModalDialog>
        </Modal>
      )}
      {refund && (
        <Modal open onClose={() => setRefund(null)}>
          <ModalDialog sx={{ width: 460, maxWidth: "96vw" }}>
            <Typography level="title-lg">Refund {refund.number}</Typography>
            <Typography level="body-sm">Open credit {money(refund.balance)}</Typography>
            <AccountSelect accounts={accounts} value={refundAcct} onChange={setRefundAcct} filter={isBank} placeholder="Pay from" />
            <Box sx={{ display: "flex", gap: 1, justifyContent: "flex-end" }}>
              <Button variant="plain" color="neutral" onClick={() => setRefund(null)}>
                Cancel
              </Button>
              <Button
                onClick={async () => {
                  if (await run(() => creditMemoAction(refund.id, "refund", { account_id: refundAcct, amount: refund.balance }))) {
                    setRefund(null);
                    load();
                    onChanged();
                  }
                }}
              >
                Refund {money(refund.balance)}
              </Button>
            </Box>
          </ModalDialog>
        </Modal>
      )}
    </Box>
  );
}

function Deposits({ accounts, settings, version, onChanged }: { accounts: Account[]; settings: Settings; version: number; onChanged: () => void }) {
  const [receipts, setReceipts] = useState<Receipt[]>([]);
  const [deps, setDeps] = useState<Deposit[]>([]);
  const [sel, setSel] = useState<Record<string, boolean>>({});
  const [acct, setAcct] = useState("");
  const [ref, setRef] = useState("");
  const [date, setDate] = useState(todayISO());
  const { err, run } = useErr();
  const load = useCallback(async () => {
    const [r, d] = (await run(() => Promise.all([listReceipts(), listDeposits()]))) ?? [[], []];
    setReceipts(r);
    setDeps(d);
  }, [run]);
  useEffect(() => {
    load();
  }, [load, version]);
  const ids = useMemo(() => receipts.filter((r) => sel[r.line_id]).map((r) => r.line_id), [receipts, sel]);
  const total = receipts.filter((r) => sel[r.line_id]).reduce((s, r) => s + r.amount, 0);
  return (
    <Box>
      <Typography level="body-sm" sx={{ mb: 1 }}>
        {settings.use_undeposited
          ? "Non-cash customer payments wait in Undeposited Funds until you group them into a bank deposit."
          : "Turn on “Hold customer payments in Undeposited Funds” in Settings to route invoice payments here. Receipts posted to Undeposited Funds by other entries also show up below."}
      </Typography>
      <ErrorAlert err={err} />
      {receipts.length === 0 ? (
        <Empty>No receipts waiting to be deposited.</Empty>
      ) : (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto", mb: 1 }}>
          <Table size="sm" data-testid="acct-undeposited">
            <thead>
              <tr>
                <th style={{ width: 40 }} />
                <th>Date</th>
                <th>From</th>
                <th>Memo</th>
                <th style={rightCell}>Amount</th>
              </tr>
            </thead>
            <tbody>
              {receipts.map((r) => (
                <tr key={r.line_id}>
                  <td>
                    <Checkbox checked={!!sel[r.line_id]} onChange={(e) => setSel({ ...sel, [r.line_id]: e.target.checked })} slotProps={{ input: { "aria-label": `Select ${r.memo}` } }} />
                  </td>
                  <td>{r.date}</td>
                  <td>{r.contact_name}</td>
                  <td>{r.memo}</td>
                  <td style={rightCell}>{money(r.amount)}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {receipts.length > 0 && (
        <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", alignItems: "center", mb: 2 }}>
          <Input size="sm" type="date" value={date} onChange={(e) => setDate(e.target.value)} />
          <AccountSelect accounts={accounts} value={acct} onChange={setAcct} filter={(a) => isBank(a)} placeholder="Deposit into" testid="acct-deposit-account" />
          <Input size="sm" placeholder="Slip / reference" value={ref} onChange={(e) => setRef(e.target.value)} />
          <Button
            size="sm"
            disabled={ids.length === 0}
            onClick={async () => {
              if (await run(() => createDeposit({ date, account_id: acct, reference: ref, line_ids: ids }))) {
                setSel({});
                load();
                onChanged();
              }
            }}
            data-testid="acct-deposit-save"
          >
            Deposit {money(total)}
          </Button>
        </Box>
      )}
      {deps.length > 0 && (
        <>
          <Typography level="title-sm" sx={{ mb: 0.5 }}>
            Deposits
          </Typography>
          <Table size="sm">
            <tbody>
              {deps.map((d) => (
                <tr key={d.id}>
                  <td>{d.date}</td>
                  <td>{accounts.find((a) => a.id === d.account_id)?.name}</td>
                  <td>{d.reference}</td>
                  <td>{d.items} receipts</td>
                  <td style={{ ...rightCell, color: ACCENT }}>{money(d.total)}</td>
                  <td>
                    <Button size="sm" variant="plain" color="danger" onClick={async () => { await run(() => deleteDeposit(d.id)); load(); onChanged(); }}>
                      Delete
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </>
      )}
    </Box>
  );
}
