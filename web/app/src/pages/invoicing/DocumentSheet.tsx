import { useEffect, useState } from "react";
import QRCode from "qrcode";
import { Box, Typography } from "@mui/joy";
import { ACCENT, money, upiLink } from "./logic";
import type { Business, InvDocument } from "./types";

/**
 * The printable document: shared by the in-app preview, the print dialog and
 * the public share page. Prints through the browser (Save as PDF).
 */
export function DocumentSheet({ doc, business, receipt }: { doc: InvDocument; business: Business; receipt?: { amount: number; paid_at: string; method: string; kind: string } }) {
  const m = (n: number) => money(n, doc.currency);
  const isEstimate = doc.kind === "estimate";
  const isCredit = doc.kind === "credit_note";
  const title = receipt ? (receipt.kind === "refund" ? "REFUND RECEIPT" : "RECEIPT") : isEstimate ? "ESTIMATE" : isCredit ? "CREDIT NOTE" : "INVOICE";
  const accent = business.accent || ACCENT;
  const gst = doc.gst;
  const showHSN = doc.lines.some((l) => l.hsn_sac);
  const upiAmount = doc.balance;
  const [qr, setQr] = useState("");
  useEffect(() => {
    if (doc.kind !== "invoice" || !business.upi_id || upiAmount <= 0 || receipt) {
      setQr("");
      return;
    }
    QRCode.toDataURL(upiLink(business.upi_id, business.upi_name || business.business_name, upiAmount, `Invoice ${doc.number}`), { margin: 1, width: 160 })
      .then(setQr)
      .catch(() => setQr(""));
  }, [doc.kind, doc.number, business.upi_id, business.upi_name, business.business_name, upiAmount, receipt]);
  const cell = { py: 0.75, px: 1, borderBottom: "1px solid #e5e5e5", fontSize: 13 } as const;
  return (
    <>
      <style>{`
        @media print {
          body * { visibility: hidden !important; }
          .invc-sheet, .invc-sheet * { visibility: visible !important; }
          .invc-sheet { position: absolute !important; inset: 0 auto auto 0 !important; width: 100% !important; box-shadow: none !important; border: 0 !important; }
          .invc-noprint { display: none !important; }
          @page { margin: 12mm; }
        }
      `}</style>
      <Box
        className="invc-sheet"
        data-testid="invc-sheet"
        sx={{ bgcolor: "#fff", color: "#111", p: { xs: 2, sm: 4 }, maxWidth: 820, mx: "auto", border: "1px solid #ddd", borderRadius: "8px", boxShadow: "sm" }}
      >
        <Box sx={{ display: "flex", justifyContent: "space-between", gap: 2, flexWrap: "wrap", mb: 3 }}>
          <Box>
            {business.logo_url && <img src={business.logo_url} alt="" style={{ maxHeight: 56, maxWidth: 200, display: "block", marginBottom: 8 }} />}
            <Typography level="title-lg" sx={{ color: "#111" }}>
              {business.business_name}
            </Typography>
            <Typography level="body-sm" sx={{ whiteSpace: "pre-line", color: "#444" }}>
              {business.address}
            </Typography>
            {business.tax_id && <Typography level="body-sm" sx={{ color: "#444" }}>{business.state_code ? "GSTIN" : "Tax ID"}: {business.tax_id}</Typography>}
            {(business.email || business.phone) && (
              <Typography level="body-sm" sx={{ color: "#444" }}>
                {[business.email, business.phone].filter(Boolean).join(" · ")}
              </Typography>
            )}
          </Box>
          <Box sx={{ textAlign: "right" }}>
            <Typography level="h2" sx={{ color: accent, letterSpacing: 1 }}>
              {title}
            </Typography>
            <Typography level="body-sm" sx={{ color: "#444" }}>
              # {doc.number}
            </Typography>
            <Typography level="body-sm" sx={{ color: "#444" }}>
              Date: {doc.issue_date}
            </Typography>
            {(isEstimate ? doc.valid_until : doc.due_date) && !receipt && (
              <Typography level="body-sm" sx={{ color: "#444" }}>
                {isEstimate ? "Valid until" : "Due"}: {isEstimate ? doc.valid_until : doc.due_date}
              </Typography>
            )}
            {doc.place_of_supply && gst && <Typography level="body-sm" sx={{ color: "#444" }}>Place of supply: {doc.place_of_supply}</Typography>}
          </Box>
        </Box>

        <Box sx={{ mb: 3 }}>
          <Typography level="body-xs" sx={{ color: "#777", textTransform: "uppercase" }}>
            {receipt ? "Received from" : "Bill to"}
          </Typography>
          <Typography level="title-md" sx={{ color: "#111" }}>
            {doc.client_name}
          </Typography>
          <Typography level="body-sm" sx={{ whiteSpace: "pre-line", color: "#444" }}>
            {[doc.client_address, doc.client_email].filter(Boolean).join("\n")}
          </Typography>
          {doc.client_gstin && <Typography level="body-sm" sx={{ color: "#444" }}>GSTIN: {doc.client_gstin}</Typography>}
        </Box>

        {receipt ? (
          <Box sx={{ border: "1px solid #ddd", borderRadius: "6px", p: 2, mb: 2 }}>
            <Typography level="body-sm" sx={{ color: "#111" }}>
              {receipt.kind === "refund" ? "Refunded" : "Received"} <b>{m(receipt.amount)}</b> on {receipt.paid_at}
              {receipt.method ? ` by ${receipt.method}` : ""} against {doc.number}.
            </Typography>
            <Typography level="body-sm" sx={{ color: "#444" }}>
              Invoice total {m(doc.total)} · Balance {m(doc.balance)}
            </Typography>
          </Box>
        ) : (
          <>
            <Box component="table" sx={{ width: "100%", borderCollapse: "collapse", mb: 2 }}>
              <thead>
                <tr style={{ background: "#f5f5f5" }}>
                  {["Description", ...(showHSN ? ["HSN/SAC"] : []), "Qty", "Price", "Disc.", "Tax", "Amount"].map((h, i) => (
                    <Box component="th" key={h} sx={{ ...cell, textAlign: i === 0 ? "left" : "right", fontWeight: 600 }}>
                      {h}
                    </Box>
                  ))}
                </tr>
              </thead>
              <tbody>
                {doc.lines.map((l, i) => (
                  <tr key={i}>
                    <Box component="td" sx={cell}>
                      {l.description}
                    </Box>
                    {showHSN && (
                      <Box component="td" sx={{ ...cell, textAlign: "right" }}>
                        {l.hsn_sac}
                      </Box>
                    )}
                    <Box component="td" sx={{ ...cell, textAlign: "right" }}>
                      {l.qty}
                      {l.unit ? ` ${l.unit}` : ""}
                    </Box>
                    <Box component="td" sx={{ ...cell, textAlign: "right" }}>
                      {m(l.price)}
                    </Box>
                    <Box component="td" sx={{ ...cell, textAlign: "right" }}>
                      {l.discount_pct ? `${l.discount_pct}%` : ""}
                    </Box>
                    <Box component="td" sx={{ ...cell, textAlign: "right" }}>
                      {l.tax_rate ? `${l.tax_rate}%` : ""}
                    </Box>
                    <Box component="td" sx={{ ...cell, textAlign: "right" }}>
                      {m(l.qty * l.price * (1 - (l.discount_pct || 0) / 100))}
                    </Box>
                  </tr>
                ))}
              </tbody>
            </Box>
            <Box sx={{ ml: "auto", width: 260, fontSize: 13 }}>
              <Row label="Subtotal" value={m(doc.subtotal)} />
              {doc.discount_total > 0 && <Row label="Discount" value={`- ${m(doc.discount_total)}`} />}
              {doc.tax_total > 0 && !gst && <Row label="Tax" value={m(doc.tax_total)} />}
              {gst && gst.intra && doc.tax_total > 0 && <Row label="CGST" value={m(doc.cgst_total ?? 0)} />}
              {gst && gst.intra && doc.tax_total > 0 && <Row label={gst.ut ? "UTGST" : "SGST"} value={m(doc.sgst_total ?? 0)} />}
              {gst && !gst.intra && doc.tax_total > 0 && <Row label="IGST" value={m(doc.igst_total ?? 0)} />}
              <Row label="Total" value={m(doc.total)} bold />
              {!isEstimate && doc.paid !== 0 && <Row label="Paid" value={`- ${m(doc.paid)}`} />}
              {!isEstimate && !isCredit && (doc.credited ?? 0) > 0 && <Row label="Credit notes" value={`- ${m(doc.credited ?? 0)}`} />}
              {!isEstimate && doc.written_off > 0 && <Row label="Written off" value={`- ${m(doc.written_off)}`} />}
              {!isEstimate && !isCredit && <Row label="Balance due" value={m(doc.balance)} bold accent />}
              {isCredit && <Row label="Credit left" value={m(doc.credit_remaining ?? 0)} bold accent />}
            </Box>
          </>
        )}

        {doc.schedule.length > 0 && !receipt && (
          <Box sx={{ mt: 3 }}>
            <Typography level="title-sm" sx={{ color: "#111" }}>
              Payment schedule
            </Typography>
            {(doc.instalments?.length ? doc.instalments : doc.schedule.map((s) => ({ ...s, paid: 0, open: s.amount }))).map((s, i) => (
              <Box key={i} sx={{ display: "flex", justifyContent: "space-between", fontSize: 13, py: 0.25, color: "#333" }} data-testid="doc-instalment">
                <span>
                  {s.label} · {s.due_date}
                  {doc.instalments?.length ? (s.open <= 0.004 ? " · paid" : s.paid > 0 ? ` · ${m(s.paid)} paid` : "") : ""}
                </span>
                <span>{m(s.amount)}</span>
              </Box>
            ))}
          </Box>
        )}
        {doc.tax_note && (
          <Typography level="body-sm" sx={{ mt: 2, whiteSpace: "pre-line", color: "#333" }}>
            <b>Tax note:</b> {doc.tax_note}
          </Typography>
        )}
        {doc.notes && (
          <Typography level="body-sm" sx={{ mt: 2, whiteSpace: "pre-line", color: "#333" }}>
            {doc.notes}
          </Typography>
        )}
        {isEstimate && doc.accepted_at && (
          <Box sx={{ mt: 2, p: 1.5, border: "1px solid #bbf7d0", bgcolor: "#f0fdf4", borderRadius: "6px", fontSize: 13, color: "#14532d" }} data-testid="invc-accepted-block">
            Accepted by <b>{doc.accepted_name}</b> on {doc.accepted_at.replace("T", " ").replace("Z", " UTC")}.
            <Box sx={{ fontFamily: "monospace", fontSize: 11, wordBreak: "break-all", color: "#166534" }}>Fingerprint {doc.accepted_hash}</Box>
          </Box>
        )}
        {doc.kind === "invoice" && !receipt && (qr || business.bank_details) && (
          <Box sx={{ mt: 2, display: "flex", gap: 3, flexWrap: "wrap", alignItems: "flex-start" }} data-testid="invc-pay-block">
            {qr && (
              <Box sx={{ textAlign: "center" }}>
                <img src={qr} alt={`UPI QR for ${m(upiAmount)}`} width={120} height={120} data-testid="invc-upi-qr" />
                <Typography level="body-xs" sx={{ color: "#444" }}>
                  Scan to pay {m(upiAmount)} by UPI
                  <br />
                  {business.upi_id}
                </Typography>
              </Box>
            )}
            {business.bank_details && (
              <Typography level="body-sm" sx={{ whiteSpace: "pre-line", color: "#333" }}>
                <b>Bank details:</b>
                {"\n"}
                {business.bank_details}
              </Typography>
            )}
          </Box>
        )}
        {business.payment_info && !isEstimate && !receipt && (
          <Typography level="body-sm" sx={{ mt: 2, whiteSpace: "pre-line", color: "#333" }}>
            <b>Payment details:</b> {"\n"}
            {business.payment_info}
          </Typography>
        )}
        {doc.terms && (
          <Typography level="body-xs" sx={{ mt: 2, whiteSpace: "pre-line", color: "#666" }}>
            {doc.terms}
          </Typography>
        )}
        {business.footer && (
          <Typography level="body-xs" sx={{ mt: 3, pt: 1, borderTop: `2px solid ${accent}`, textAlign: "center", color: "#666", whiteSpace: "pre-line" }}>
            {business.footer}
          </Typography>
        )}
        {doc.attachments.length > 0 && (
          <Box sx={{ mt: 2, fontSize: 13 }} className="invc-noprint">
            Attachments:{" "}
            {doc.attachments.map((a, i) => (
              <a key={i} href={a.url} target="_blank" rel="noreferrer noopener" style={{ marginRight: 12 }}>
                {a.name || a.url}
              </a>
            ))}
          </Box>
        )}
      </Box>
    </>
  );
}

function Row({ label, value, bold, accent }: { label: string; value: string; bold?: boolean; accent?: boolean }) {
  return (
    <Box sx={{ display: "flex", justifyContent: "space-between", py: 0.4, fontWeight: bold ? 700 : 400, color: accent ? ACCENT : "#111", borderTop: bold ? "1px solid #ddd" : undefined }}>
      <span>{label}</span>
      <span>{value}</span>
    </Box>
  );
}
