import { useState } from "react";
import {
  Box,
  Button,
  Dropdown,
  FormControl,
  FormLabel,
  IconButton,
  Input,
  Menu,
  MenuButton,
  MenuItem,
  Modal,
  ModalClose,
  ModalDialog,
  Option,
  Select,
  Sheet,
  Table,
  Textarea,
  Typography,
} from "@mui/joy";
import MoreVert from "@mui/icons-material/MoreVert";
import {
  ApiError,
  addPayment,
  convertEstimate,
  deletePayment,
  duplicateDocument,
  remindDocument,
  sendDocument,
  setEstimateStatus,
  trashDocument,
} from "./api";
import { DocumentSheet } from "./DocumentSheet";
import { useSalesActions } from "./SalesActions";
import { money, todayISO } from "./logic";
import type { Business, InvDocument, Payment, Settings } from "./types";
import { ErrorAlert, StatusChip, rightCell, useErr } from "./ui";

export function DocDetail({
  doc: initial,
  settings,
  emailReady,
  onClose,
  onChanged,
  onEdit,
  onOpen,
  onOpenOrder,
}: {
  doc: InvDocument;
  settings: Settings;
  emailReady: boolean;
  onClose: () => void;
  onChanged: () => void;
  onEdit: (d: InvDocument) => void;
  onOpen: (id: string) => void;
  onOpenOrder?: (id: string) => void;
}) {
  const [doc, setDoc] = useState(initial);
  const [pay, setPay] = useState<null | { kind: Payment["kind"]; amount: number; method: string; reference: string; note: string; paid_at: string }>(null);
  const [receipt, setReceipt] = useState<Payment | null>(null);
  const [sendOpen, setSendOpen] = useState(false);
  const [message, setMessage] = useState("");
  const [info, setInfo] = useState("");
  const { err, setErr, run } = useErr();
  const m = (n: number) => money(n, doc.currency);
  const isInv = doc.kind === "invoice";
  const business: Business = {
    ...settings,
    payment_info: settings.payment_info,
    accent: settings.sales?.pdf_accent,
    footer: settings.sales?.pdf_footer,
    state_code: settings.sales?.state_code,
    bank_details: settings.sales?.show_bank ? settings.sales?.bank_details : "",
    upi_id: settings.sales?.show_upi_qr ? settings.sales?.upi_id : "",
    upi_name: settings.sales?.upi_name,
  };
  const shareUrl = `${window.location.origin}/invoice/${doc.share_token}`;

  const apply = (d: InvDocument | undefined) => {
    if (d) {
      setDoc(d);
      onChanged();
    }
  };
  const sales = useSalesActions(doc, { apply, setInfo, onOpen, onChanged, onOpenOrder });

  const doSend = async (reminder: boolean, markOnly = false) => {
    setInfo("");
    try {
      setErr("");
      const r = reminder ? await remindDocument(doc.id, message) : await sendDocument(doc.id, { message, mark_only: markOnly });
      apply(r.document);
      setSendOpen(false);
      setInfo(
        (r.emailed ? `Emailed to ${doc.client_email}.` : "Marked as sent.") +
          (r.stock_warnings?.length ? ` Stock warnings: ${r.stock_warnings.join("; ")}` : ""),
      );
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) {
        // No server-side email configured: open the user's mail client, then mark as sent.
        const subject = `${isInv ? "Invoice" : "Estimate"} ${doc.number}${settings.business_name ? ` from ${settings.business_name}` : ""}`;
        const body = `${message || "Please find your document at the link below."}\n\n${shareUrl}`;
        window.location.href = `mailto:${encodeURIComponent(doc.client_email)}?subject=${encodeURIComponent(subject)}&body=${encodeURIComponent(body)}`;
        if (!reminder) {
          try {
            apply((await sendDocument(doc.id, { mark_only: true })).document);
          } catch (e2) {
            setErr((e2 as Error).message);
          }
        }
        setSendOpen(false);
        setInfo("Server email is not configured, so your mail app was opened. The document is marked as sent.");
      } else {
        setErr((e as Error).message);
      }
    }
  };

  const savePay = async () => {
    if (!pay) return;
    const d = await run(() => addPayment(doc.id, pay));
    if (d) {
      apply(d);
      setPay(null);
    }
  };

  const openPay = (kind: Payment["kind"]) =>
    setPay({ kind, amount: kind === "refund" ? doc.paid : doc.balance, method: "", reference: "", note: "", paid_at: todayISO() });

  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: "min(980px, 96vw)", maxHeight: "94vh", overflow: "auto", bgcolor: "#f3f4f6" }} data-testid="invc-detail">
        <ModalClose />
        <Box className="invc-noprint" sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap", pr: 4 }}>
          <Typography level="title-lg">{doc.number}</Typography>
          <StatusChip status={doc.display_status} />
          {doc.viewed_at && <Typography level="body-xs">Viewed by client</Typography>}
          <Box sx={{ flex: 1 }} />
          {!doc.deleted && (
            <>
              <Button size="sm" variant="outlined" onClick={() => onEdit(doc)} data-testid="invc-edit">
                Edit
              </Button>
              {doc.status === "draft" ? (
                <Button size="sm" onClick={() => setSendOpen(true)} data-testid="invc-send" sx={{ bgcolor: "#C2410C", "&:hover": { bgcolor: "#9A3412" } }}>
                  Send
                </Button>
              ) : (
                isInv &&
                doc.balance > 0.004 && (
                  <Button size="sm" variant="outlined" onClick={() => setSendOpen(true)} data-testid="invc-remind">
                    Send reminder
                  </Button>
                )
              )}
              {isInv && doc.balance > 0.004 && (
                <Button size="sm" onClick={() => openPay("payment")} data-testid="invc-record-payment" color="success">
                  Record payment
                </Button>
              )}
              <Button size="sm" variant="outlined" onClick={() => window.print()} data-testid="invc-print">
                Print / PDF
              </Button>
              <Dropdown>
                <MenuButton slots={{ root: IconButton }} slotProps={{ root: { size: "sm", variant: "outlined", "aria-label": "More actions", "data-testid": "invc-more" } }}>
                  <MoreVert />
                </MenuButton>
                <Menu placement="bottom-end" sx={{ zIndex: 2000 }}>
                  {doc.status !== "draft" && (
                    <MenuItem
                      onClick={() => {
                        navigator.clipboard?.writeText(shareUrl);
                        setInfo(`Link copied: ${shareUrl}`);
                      }}
                      data-testid="invc-copy-link"
                    >
                      Copy public link
                    </MenuItem>
                  )}
                  {doc.status === "draft" && <MenuItem onClick={() => doSend(false, true)}>Mark as sent (no email)</MenuItem>}
                  {isInv && doc.paid > 0 && <MenuItem onClick={() => openPay("refund")}>Refund…</MenuItem>}
                  {isInv && doc.balance > 0.004 && <MenuItem onClick={() => openPay("writeoff")}>Write off balance…</MenuItem>}
                  {sales.items}
                  {!isInv && doc.kind === "estimate" && doc.status !== "converted" && doc.status !== "superseded" && doc.status !== "declined" && (
                    <MenuItem
                      onClick={async () => {
                        const d = await run(() => convertEstimate(doc.id));
                        if (d) {
                          onChanged();
                          onOpen(d.id);
                        }
                      }}
                      data-testid="invc-convert"
                    >
                      Convert to invoice
                    </MenuItem>
                  )}
                  {doc.kind === "estimate" && ["sent", "draft"].includes(doc.status) && <MenuItem onClick={async () => apply(await run(() => setEstimateStatus(doc.id, "accepted")))}>Mark accepted (without customer sign-off)</MenuItem>}
                  {doc.kind === "estimate" && ["sent", "draft"].includes(doc.status) && <MenuItem onClick={async () => apply(await run(() => setEstimateStatus(doc.id, "declined")))}>Mark declined</MenuItem>}
                  <MenuItem
                    onClick={async () => {
                      const d = await run(() => duplicateDocument(doc.id));
                      if (d) {
                        onChanged();
                        onOpen(d.id);
                      }
                    }}
                  >
                    Duplicate
                  </MenuItem>
                  <MenuItem
                    color="danger"
                    onClick={async () => {
                      if ((await run(() => trashDocument(doc.id))) !== undefined) {
                        onChanged();
                        onClose();
                      }
                    }}
                    data-testid="invc-trash"
                  >
                    Move to trash
                  </MenuItem>
                </Menu>
              </Dropdown>
            </>
          )}
        </Box>
        <Box className="invc-noprint">
          <ErrorAlert err={err} />
          {info && (
            <Typography level="body-sm" color="success" data-testid="invc-info" sx={{ my: 0.5 }}>
              {info}
            </Typography>
          )}
        </Box>

        <DocumentSheet doc={doc} business={business} receipt={receipt ? { amount: receipt.amount, paid_at: receipt.paid_at, method: receipt.method, kind: receipt.kind } : undefined} />
        {receipt && (
          <Box className="invc-noprint" sx={{ display: "flex", gap: 1, justifyContent: "center" }}>
            <Button size="sm" onClick={() => window.print()}>
              Print receipt
            </Button>
            <Button size="sm" variant="plain" onClick={() => setReceipt(null)}>
              Back to {doc.kind}
            </Button>
          </Box>
        )}

        {doc.payments.length > 0 && (
          <Sheet className="invc-noprint" variant="outlined" sx={{ p: 1.5, borderRadius: "md" }}>
            <Typography level="title-sm" sx={{ mb: 1 }}>
              Payments
            </Typography>
            <Table size="sm" data-testid="invc-payments">
              <thead>
                <tr>
                  <th>Date</th>
                  <th>Type</th>
                  <th>Method</th>
                  <th>Reference</th>
                  <th style={rightCell}>Amount</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {doc.payments.map((p) => (
                  <tr key={p.id}>
                    <td>{p.paid_at}</td>
                    <td>{({ writeoff: "Write-off", refund: "Refund", credit: "Credit note", applied: "Applied to invoice", payment: "Payment" } as Record<string, string>)[p.kind] ?? "Payment"}</td>
                    <td>{p.method}</td>
                    <td>{p.reference || p.note}</td>
                    <td style={rightCell}>{m(p.amount)}</td>
                    <td style={rightCell}>
                      {(p.kind === "payment" || p.kind === "refund") && doc.kind === "invoice" && (
                        <Button size="sm" variant="plain" onClick={() => setReceipt(p)}>
                          Receipt
                        </Button>
                      )}
                      {p.kind !== "credit" && p.kind !== "applied" && (
                        <Button size="sm" variant="plain" color="danger" onClick={async () => apply(await run(() => deletePayment(doc.id, p.id)))}>
                          Remove
                        </Button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </Sheet>
        )}

        {pay && (
          <Modal open onClose={() => setPay(null)}>
            <ModalDialog data-testid="invc-pay-dialog" sx={{ width: 420 }}>
              <ModalClose />
              <Typography level="title-lg">{pay.kind === "refund" ? "Refund" : pay.kind === "writeoff" ? "Write off" : "Record payment"}</Typography>
              <ErrorAlert err={err} />
              <FormControl>
                <FormLabel>Amount ({doc.currency})</FormLabel>
                <Input type="number" value={pay.amount} onChange={(e) => setPay({ ...pay, amount: Number(e.target.value) })} slotProps={{ input: { "data-testid": "invc-pay-amount" } }} />
              </FormControl>
              <FormControl>
                <FormLabel>Date</FormLabel>
                <Input type="date" value={pay.paid_at} onChange={(e) => setPay({ ...pay, paid_at: e.target.value })} />
              </FormControl>
              {pay.kind !== "writeoff" && (
                <FormControl>
                  <FormLabel>Method</FormLabel>
                  <Select value={pay.method || null} placeholder="Choose" onChange={(_, v) => setPay({ ...pay, method: (v as string) ?? "" })}>
                    {["Bank transfer", "UPI", "Cash", "Card", "Cheque", "Other"].map((x) => (
                      <Option key={x} value={x}>
                        {x}
                      </Option>
                    ))}
                  </Select>
                </FormControl>
              )}
              <FormControl>
                <FormLabel>Reference / note</FormLabel>
                <Input value={pay.reference} onChange={(e) => setPay({ ...pay, reference: e.target.value })} />
              </FormControl>
              <Button onClick={savePay} data-testid="invc-pay-save">
                Save
              </Button>
            </ModalDialog>
          </Modal>
        )}

        {sales.dialogs}
        {sendOpen && (
          <Modal open onClose={() => setSendOpen(false)}>
            <ModalDialog sx={{ width: 460 }} data-testid="invc-send-dialog">
              <ModalClose />
              <Typography level="title-lg">{doc.status === "draft" ? `Send ${doc.kind}` : "Send reminder"}</Typography>
              <Typography level="body-sm">
                To: {doc.client_email || "(no email address on this client)"}
                {!emailReady && " · server email is not set up, so your mail app opens instead"}
              </Typography>
              <ErrorAlert err={err} />
              <FormControl>
                <FormLabel>Message</FormLabel>
                <Textarea minRows={3} value={message} onChange={(e) => setMessage(e.target.value)} />
              </FormControl>
              <Button onClick={() => doSend(doc.status !== "draft")} disabled={!doc.client_email} data-testid="invc-send-go">
                {emailReady ? "Send email" : "Open mail app and mark sent"}
              </Button>
              {doc.status === "draft" && (
                <Button variant="plain" onClick={() => doSend(false, true)}>
                  Just mark as sent
                </Button>
              )}
            </ModalDialog>
          </Modal>
        )}
      </ModalDialog>
    </Modal>
  );
}
