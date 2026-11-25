import { useCallback, useState } from "react";
import { Alert, Box, Sheet, Typography } from "@mui/joy";
import type { ReactNode } from "react";
import { receiptText, methodLabel, money, qtyText } from "./logic";
import type { PosSale, PosSettings } from "./types";

export const ACCENT = "#7B3FB5";
export const rightCell = { textAlign: "right" } as const;

export function useErr() {
  const [err, setErr] = useState("");
  const run = useCallback(async <T,>(fn: () => Promise<T>): Promise<T | undefined> => {
    try {
      setErr("");
      return await fn();
    } catch (e) {
      setErr((e as Error).message);
      return undefined;
    }
  }, []);
  return { err, setErr, run };
}

export function ErrorAlert({ err }: { err: string }) {
  return err ? (
    <Alert color="danger" role="alert" sx={{ my: 1 }}>
      {err}
    </Alert>
  ) : null;
}

export function Empty({ children }: { children: ReactNode }) {
  return (
    <Sheet variant="soft" sx={{ p: 4, textAlign: "center", borderRadius: "md" }}>
      <Typography level="title-md">{children}</Typography>
    </Sheet>
  );
}

export function Stat({ label, value, testid }: { label: string; value: string; testid?: string }) {
  return (
    <Sheet variant="outlined" sx={{ p: 1.5, borderRadius: "md", minWidth: 140, flex: "1 1 140px" }}>
      <Typography level="body-xs">{label}</Typography>
      <Typography level="h4" data-testid={testid}>
        {value}
      </Typography>
    </Sheet>
  );
}

/** Printable receipt (80mm thermal width). Prints through the browser print dialog. */
export function Receipt({ sale, settings }: { sale: PosSale; settings: PosSettings }) {
  const m = (n: number) => money(n, settings.currency);
  const row = { display: "flex", justifyContent: "space-between", gap: 1 } as const;
  return (
    <>
      <style>{`
        @media print {
          body * { visibility: hidden !important; }
          .pos-receipt, .pos-receipt * { visibility: visible !important; }
          .pos-receipt { position: absolute; left: 0; top: 0; width: 80mm; padding: 2mm; box-shadow: none !important; border: 0 !important; }
          @page { size: 80mm auto; margin: 3mm; }
        }
      `}</style>
      <Box
        className="pos-receipt"
        data-testid="pos-receipt"
        sx={{ fontFamily: "ui-monospace, Menlo, monospace", fontSize: 13, width: "100%", maxWidth: 340, mx: "auto", p: 1.5, bgcolor: "#fff", color: "#000", border: "1px dashed #bbb" }}
      >
        <Box sx={{ textAlign: "center", mb: 1 }}>
          {settings.store_name && <Box sx={{ fontWeight: 700, fontSize: 15 }}>{settings.store_name}</Box>}
          {settings.receipt_header && <Box sx={{ whiteSpace: "pre-line" }}>{settings.receipt_header}</Box>}
          <Box>Receipt #{String(sale.number)}</Box>
          <Box>{new Date(sale.completed_at || sale.created_at).toLocaleString()}</Box>
          {sale.customer_name && <Box>Customer: {sale.customer_name}</Box>}
        </Box>
        <Box sx={{ borderTop: "1px dashed #999", borderBottom: "1px dashed #999", py: 0.5 }}>
          {sale.lines.map((l) => (
            <Box key={l.id} sx={{ mb: 0.5 }}>
              <Box sx={row}>
                <span>
                  {qtyText(l.qty)} x {l.name}
                </span>
                <span>{m(l.line_total)}</span>
              </Box>
              {l.discount_amount > 0 && <Box sx={{ pl: 2, color: "#555" }}>discount -{m(l.discount_amount)}</Box>}
              {l.note && <Box sx={{ pl: 2, color: "#555" }}>{l.note}</Box>}
              {l.refunded_qty > 0 && <Box sx={{ pl: 2, color: "#555" }}>returned {qtyText(l.refunded_qty)}</Box>}
            </Box>
          ))}
        </Box>
        <Box sx={{ mt: 0.5 }}>
          <Box sx={row}>
            <span>Subtotal</span>
            <span>{m(sale.subtotal)}</span>
          </Box>
          {sale.discount_total > 0 && (
            <Box sx={row}>
              <span>Discounts</span>
              <span>-{m(sale.discount_total)}</span>
            </Box>
          )}
          {sale.tax_total > 0 && (
            <Box sx={row}>
              <span>Tax</span>
              <span>{m(sale.tax_total)}</span>
            </Box>
          )}
          <Box sx={{ ...row, fontWeight: 700, fontSize: 15 }}>
            <span>Total</span>
            <span>{m(sale.total)}</span>
          </Box>
          {sale.payments.map((p) => (
            <Box key={p.id} sx={row}>
              <span>{methodLabel(p.method)}</span>
              <span>{m(p.method === "cash" ? p.tendered : p.amount)}</span>
            </Box>
          ))}
          {sale.change_due > 0 && (
            <Box sx={row}>
              <span>Change</span>
              <span>{m(sale.change_due)}</span>
            </Box>
          )}
          {sale.refunds.map((r) => (
            <Box key={r.id} sx={row}>
              <span>Refund {r.number}</span>
              <span>-{m(r.total)}</span>
            </Box>
          ))}
        </Box>
        {settings.receipt_footer && <Box sx={{ textAlign: "center", mt: 1, whiteSpace: "pre-line" }}>{settings.receipt_footer}</Box>}
      </Box>
    </>
  );
}

export function emailReceiptHref(sale: PosSale, settings: PosSettings): string {
  const subject = `Receipt #${String(sale.number)}${settings.store_name ? ` from ${settings.store_name}` : ""}`;
  return `mailto:?subject=${encodeURIComponent(subject)}&body=${encodeURIComponent(receiptText(sale, settings))}`;
}
