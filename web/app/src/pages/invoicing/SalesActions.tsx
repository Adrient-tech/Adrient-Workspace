import { useState } from "react";
import type { ReactNode } from "react";
import { Box, Button, Checkbox, FormControl, FormLabel, Input, MenuItem, Modal, ModalClose, ModalDialog, Textarea, Typography } from "@mui/joy";
import {
  ApiError,
  applyCreditNote,
  createCreditNote,
  estimateToOrder,
  getEInvoice,
  getWhatsApp,
  listDocuments,
  newVersion,
  paymentLink,
  portalLink,
  refundCreditNote,
} from "./api";
import { money } from "./logic";
import type { InvDocument, Line } from "./types";
import { ErrorAlert, useErr } from "./ui";

/**
 * The wave-3 document actions (versions, orders, credit notes, payment links,
 * WhatsApp, e-invoice, portal). Returns menu items for the "more" menu and the
 * dialogs they open, so DocDetail only has to render both.
 */
export function useSalesActions(
  doc: InvDocument,
  hooks: { apply: (d: InvDocument | undefined) => void; setInfo: (s: string) => void; onOpen: (id: string) => void; onChanged: () => void; onOpenOrder?: (id: string) => void },
): { items: ReactNode; dialogs: ReactNode } {
  const { apply, setInfo, onOpen, onChanged, onOpenOrder } = hooks;
  const { err, run, setErr } = useErr();
  const [credit, setCredit] = useState<null | { qty: number[]; reason: string; apply: boolean }>(null);
  const [applyOpen, setApplyOpen] = useState<null | InvDocument[]>(null);
  const [refund, setRefund] = useState<null | { amount: number; method: string; reference: string }>(null);
  const [einv, setEinv] = useState<null | { ready: boolean; issues: string[]; json: unknown }>(null);
  const m = (n: number) => money(n, doc.currency);
  const isInv = doc.kind === "invoice";
  const isEst = doc.kind === "estimate";
  const isCn = doc.kind === "credit_note";
  const live = doc.status !== "draft";

  const doPaymentLink = async () => {
    setErr("");
    try {
      const r = await paymentLink(doc.id);
      await navigator.clipboard?.writeText(r.url).catch(() => undefined);
      setInfo(`Payment link: ${r.url} (copied)`);
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) setInfo("Online payments are not configured. Add Razorpay keys in Settings > Payments.");
      else setErr((e as Error).message);
    }
  };

  const doWhatsApp = async () => {
    const r = await run(() => getWhatsApp(doc.id));
    if (r) window.open(r.url, "_blank", "noopener,noreferrer");
  };

  const doPortal = async () => {
    const key = doc.contact_id || `name:${doc.client_name.toLowerCase()}`;
    const r = await run(() => portalLink(key));
    if (r) {
      const url = `${window.location.origin}${r.path}`;
      await navigator.clipboard?.writeText(url).catch(() => undefined);
      setInfo(`Customer portal link copied: ${url}`);
    }
  };

  const doEinvoice = async () => {
    const r = await run(() => getEInvoice(doc.id));
    if (r) setEinv(r);
  };

  const downloadEinvoice = () => {
    if (!einv) return;
    const blob = new Blob([JSON.stringify([einv.json], null, 2)], { type: "application/json" });
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = `einvoice-${doc.number}.json`;
    a.click();
    URL.revokeObjectURL(a.href);
  };

  const openCredit = () => setCredit({ qty: doc.lines.map((l) => l.qty), reason: "", apply: true });
  const submitCredit = async () => {
    if (!credit) return;
    const lines: Line[] = doc.lines.map((l, i) => ({ ...l, qty: credit.qty[i] ?? 0 })).filter((l) => l.qty > 0);
    const cn = await run(() => createCreditNote(doc.id, { lines, reason: credit.reason, apply: credit.apply }));
    if (cn) {
      setCredit(null);
      onChanged();
      setInfo(`Credit note ${cn.number} issued for ${money(cn.total, cn.currency)}.`);
      onOpen(cn.id);
    }
  };
  const creditTotal = credit
    ? doc.lines.reduce((s, l, i) => s + (credit.qty[i] || 0) * l.price * (1 - (l.discount_pct || 0) / 100) * (1 + (l.tax_rate || 0) / 100), 0) * (doc.discount_kind === "percent" ? 1 - (doc.discount_value || 0) / 100 : 1)
    : 0;

  const items = (
    <>
      {isEst && live && doc.status !== "converted" && doc.status !== "superseded" && doc.status !== "declined" && (
        <MenuItem
          onClick={async () => {
            const o = await run(() => estimateToOrder(doc.id));
            if (o) {
              onChanged();
              setInfo(`Sales order ${o.number} created.`);
              onOpenOrder?.(o.id);
            }
          }}
          data-testid="invc-to-order"
        >
          Create sales order
        </MenuItem>
      )}
      {isEst && doc.status !== "converted" && doc.status !== "superseded" && (
        <MenuItem
          onClick={async () => {
            const d = await run(() => newVersion(doc.id));
            if (d) {
              onChanged();
              onOpen(d.id);
            }
          }}
          data-testid="invc-new-version"
        >
          Revise (new version)
        </MenuItem>
      )}
      {isInv && live && <MenuItem onClick={openCredit} data-testid="invc-credit-note">Issue credit note…</MenuItem>}
      {isInv && live && doc.balance > 0.004 && <MenuItem onClick={doPaymentLink} data-testid="invc-payment-link">Create payment link (Razorpay)</MenuItem>}
      {isInv && live && doc.balance > 0.004 && <MenuItem onClick={doWhatsApp} data-testid="invc-whatsapp">Remind on WhatsApp</MenuItem>}
      {(isInv || isCn) && live && <MenuItem onClick={doEinvoice} data-testid="invc-einvoice">E-invoice JSON…</MenuItem>}
      {(isInv || isEst) && live && <MenuItem onClick={doPortal} data-testid="invc-portal">Copy customer portal link</MenuItem>}
      {isCn && doc.credit_remaining! > 0.004 && (
        <MenuItem
          onClick={async () => {
            const open = (await run(() => listDocuments({ kind: "invoice", contact_id: doc.contact_id || undefined, limit: 500 }))) ?? [];
            setApplyOpen(open.filter((d) => d.balance > 0.004 && d.status !== "draft" && d.currency === doc.currency && (d.contact_id ? d.contact_id === doc.contact_id : d.client_name === doc.client_name)));
          }}
          data-testid="invc-cn-apply"
        >
          Apply to an invoice…
        </MenuItem>
      )}
      {isCn && doc.credit_remaining! > 0.004 && (
        <MenuItem onClick={() => setRefund({ amount: doc.credit_remaining ?? 0, method: "", reference: "" })} data-testid="invc-cn-refund">
          Refund to customer…
        </MenuItem>
      )}
    </>
  );

  const dialogs = (
    <>
      {credit && (
        <Modal open onClose={() => setCredit(null)}>
          <ModalDialog sx={{ width: "min(640px, 96vw)", maxHeight: "92vh", overflow: "auto" }} data-testid="invc-credit-dialog">
            <ModalClose />
            <Typography level="title-lg">Credit note for {doc.number}</Typography>
            <ErrorAlert err={err} />
            <Typography level="body-sm">Set how many units of each line to credit (0 skips a line).</Typography>
            {doc.lines.map((l, i) => (
              <Box key={i} sx={{ display: "flex", gap: 1, alignItems: "center" }}>
                <Typography level="body-sm" sx={{ flex: 1 }}>
                  {l.description} · {m(l.price)} × {l.qty}
                </Typography>
                <Input
                  size="sm"
                  type="number"
                  sx={{ width: 90 }}
                  value={credit.qty[i]}
                  slotProps={{ input: { min: 0, max: l.qty, "data-testid": `invc-credit-qty-${i}` } }}
                  onChange={(e) => setCredit({ ...credit, qty: credit.qty.map((q, j) => (j === i ? Math.max(0, Math.min(l.qty, Number(e.target.value))) : q)) })}
                />
              </Box>
            ))}
            <FormControl>
              <FormLabel>Reason</FormLabel>
              <Textarea minRows={2} value={credit.reason} onChange={(e) => setCredit({ ...credit, reason: e.target.value })} slotProps={{ textarea: { "data-testid": "invc-credit-reason" } }} />
            </FormControl>
            <Checkbox label={`Apply to this invoice's balance (${m(doc.balance)})`} checked={credit.apply} onChange={(e) => setCredit({ ...credit, apply: e.target.checked })} />
            <Typography level="body-xs">About {m(creditTotal)} including tax. The server computes the exact amount.</Typography>
            <Button onClick={submitCredit} data-testid="invc-credit-save">
              Issue credit note
            </Button>
          </ModalDialog>
        </Modal>
      )}
      {applyOpen && (
        <Modal open onClose={() => setApplyOpen(null)}>
          <ModalDialog sx={{ width: 460 }} data-testid="invc-apply-dialog">
            <ModalClose />
            <Typography level="title-lg">Apply {doc.number}</Typography>
            <ErrorAlert err={err} />
            {applyOpen.length === 0 && <Typography level="body-sm">This customer has no open invoice in {doc.currency}.</Typography>}
            {applyOpen.map((inv) => (
              <Button
                key={inv.id}
                variant="outlined"
                onClick={async () => {
                  const d = await run(() => applyCreditNote(doc.id, inv.id));
                  if (d) {
                    apply(d);
                    setApplyOpen(null);
                    setInfo(`Applied to ${inv.number}.`);
                  }
                }}
              >
                {inv.number} · balance {money(inv.balance, inv.currency)}
              </Button>
            ))}
          </ModalDialog>
        </Modal>
      )}
      {refund && (
        <Modal open onClose={() => setRefund(null)}>
          <ModalDialog sx={{ width: 420 }}>
            <ModalClose />
            <Typography level="title-lg">Refund credit</Typography>
            <ErrorAlert err={err} />
            <FormControl>
              <FormLabel>Amount (up to {m(doc.credit_remaining ?? 0)})</FormLabel>
              <Input type="number" value={refund.amount} onChange={(e) => setRefund({ ...refund, amount: Number(e.target.value) })} />
            </FormControl>
            <FormControl>
              <FormLabel>Method</FormLabel>
              <Input value={refund.method} placeholder="UPI, bank transfer, cash" onChange={(e) => setRefund({ ...refund, method: e.target.value })} />
            </FormControl>
            <FormControl>
              <FormLabel>Reference</FormLabel>
              <Input value={refund.reference} onChange={(e) => setRefund({ ...refund, reference: e.target.value })} />
            </FormControl>
            <Button
              data-testid="invc-refund-save"
              onClick={async () => {
                const d = await run(() => refundCreditNote(doc.id, refund));
                if (d) {
                  apply(d);
                  setRefund(null);
                }
              }}
            >
              Record refund
            </Button>
          </ModalDialog>
        </Modal>
      )}
      {einv && (
        <Modal open onClose={() => setEinv(null)}>
          <ModalDialog sx={{ width: "min(640px, 96vw)", maxHeight: "90vh", overflow: "auto" }} data-testid="invc-einvoice-dialog">
            <ModalClose />
            <Typography level="title-lg">E-invoice JSON</Typography>
            {einv.ready ? (
              <Typography color="success" level="body-sm">Ready to upload to the e-invoice portal (IRP schema 1.1).</Typography>
            ) : (
              <Box>
                <Typography color="warning" level="body-sm">Fix these before uploading:</Typography>
                <ul>
                  {einv.issues.map((x) => (
                    <li key={x}>{x}</li>
                  ))}
                </ul>
              </Box>
            )}
            <Typography level="body-xs">Adrient prepares the file. It does not submit it to the government portal.</Typography>
            <Button onClick={downloadEinvoice} data-testid="invc-einvoice-download">Download JSON</Button>
          </ModalDialog>
        </Modal>
      )}
    </>
  );
  return { items, dialogs };
}
