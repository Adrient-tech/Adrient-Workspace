import { useEffect, useState } from "react";
import { Alert, Box, Button, Checkbox, FormControl, FormLabel, Input, Option, Select, Table, Textarea, Typography } from "@mui/joy";
import { getGateway, getReminderLog, runReminders, saveGateway } from "./api";
import type { GatewayStatus, ReminderLogRow } from "./api";
import { GST_STATES } from "./logic";
import type { SalesSettings } from "./types";
import { ErrorAlert, useErr } from "./ui";

/** GST, UPI/bank, document look, reminders and online payment keys. Rendered inside Settings; the parent saves `sales`. */
export function SalesSettingsPanel({ sales, onChange }: { sales: SalesSettings; onChange: (s: SalesSettings) => void }) {
  const set = (p: Partial<SalesSettings>) => onChange({ ...sales, ...p });
  const [gw, setGw] = useState<GatewayStatus | null>(null);
  const [keys, setKeys] = useState({ key_id: "", key_secret: "", webhook_secret: "" });
  const [log, setLog] = useState<ReminderLogRow[]>([]);
  const [note, setNote] = useState("");
  const { err, run } = useErr();

  useEffect(() => {
    getGateway().then(setGw).catch(() => setGw(null));
    getReminderLog().then(setLog).catch(() => setLog([]));
  }, []);

  // Scheduled reminders build links from this address, so remember where the admin is working.
  useEffect(() => {
    if (!sales.public_url) onChange({ ...sales, public_url: window.location.origin });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const saveKeys = async () => {
    const g = await run(() => saveGateway(keys));
    if (g) {
      setGw(g);
      setKeys({ key_id: "", key_secret: "", webhook_secret: "" });
      setNote("Payment keys saved.");
    }
  };

  const ruleText = (d: number) => (d < 0 ? `${-d} day(s) before due` : d === 0 ? "on the due date" : `${d} day(s) after due`);

  return (
    <Box sx={{ display: "grid", gap: 1.5 }} data-testid="invc-sales-settings">
      <ErrorAlert err={err} />
      <Typography level="title-md" sx={{ mt: 2 }}>
        GST
      </Typography>
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 1.5 }}>
        <FormControl>
          <FormLabel>Your state (switches on CGST/SGST/IGST and HSN/SAC)</FormLabel>
          <Select value={sales.state_code || ""} onChange={(_, v) => set({ state_code: (v as string) || "" })} data-testid="invc-set-state">
            <Option value="">Not registered for GST</Option>
            {GST_STATES.filter(([c]) => c !== "96" && c !== "97").map(([c, n]) => (
              <Option key={c} value={c}>
                {c} {n}
              </Option>
            ))}
          </Select>
        </FormControl>
        <FormControl>
          <FormLabel>Estimate valid for (days)</FormLabel>
          <Input type="number" value={sales.estimate_valid_days} onChange={(e) => set({ estimate_valid_days: Number(e.target.value) })} />
        </FormControl>
      </Box>
      <Typography level="body-xs">Put your GSTIN in "Tax ID" above. Within your state an invoice shows CGST + SGST, to another state IGST, decided by the place of supply on each document.</Typography>

      <Typography level="title-md" sx={{ mt: 1 }}>
        Payment details and document look
      </Typography>
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 1.5 }}>
        <FormControl>
          <FormLabel>UPI id (for the QR code on invoices)</FormLabel>
          <Input placeholder="name@bank" value={sales.upi_id} onChange={(e) => set({ upi_id: e.target.value.trim() })} slotProps={{ input: { "data-testid": "invc-set-upi" } }} />
        </FormControl>
        <FormControl>
          <FormLabel>Payee name on the UPI app</FormLabel>
          <Input value={sales.upi_name} onChange={(e) => set({ upi_name: e.target.value })} />
        </FormControl>
        <FormControl>
          <FormLabel>Accent colour</FormLabel>
          <Input type="color" value={sales.pdf_accent || "#126C5A"} onChange={(e) => set({ pdf_accent: e.target.value })} sx={{ width: 90 }} />
        </FormControl>
        <FormControl>
          <FormLabel>Footer line</FormLabel>
          <Input value={sales.pdf_footer} onChange={(e) => set({ pdf_footer: e.target.value })} />
        </FormControl>
      </Box>
      <FormControl>
        <FormLabel>Bank details (printed on invoices)</FormLabel>
        <Textarea minRows={2} value={sales.bank_details} onChange={(e) => set({ bank_details: e.target.value })} slotProps={{ textarea: { "data-testid": "invc-set-bank" } }} />
      </FormControl>
      <Box sx={{ display: "flex", gap: 3, flexWrap: "wrap" }}>
        <Checkbox label="Show UPI QR on invoices" checked={sales.show_upi_qr} onChange={(e) => set({ show_upi_qr: e.target.checked })} />
        <Checkbox label="Show bank details on invoices" checked={sales.show_bank} onChange={(e) => set({ show_bank: e.target.checked })} />
      </Box>
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr 1fr", sm: "repeat(3, 1fr)" }, gap: 1.5 }}>
        <FormControl>
          <FormLabel>Sales order prefix</FormLabel>
          <Input value={sales.order_prefix} onChange={(e) => set({ order_prefix: e.target.value })} />
        </FormControl>
        <FormControl>
          <FormLabel>Delivery note prefix</FormLabel>
          <Input value={sales.delivery_prefix} onChange={(e) => set({ delivery_prefix: e.target.value })} />
        </FormControl>
        <FormControl>
          <FormLabel>Credit note prefix</FormLabel>
          <Input value={sales.credit_prefix} onChange={(e) => set({ credit_prefix: e.target.value })} />
        </FormControl>
      </Box>

      <Typography level="title-md" sx={{ mt: 1 }}>
        Payment reminders
      </Typography>
      <Checkbox label="Send reminders automatically by email" checked={sales.reminders_auto} onChange={(e) => set({ reminders_auto: e.target.checked })} slotProps={{ input: { "data-testid": "invc-set-reminders-auto" } }} />
      <Typography level="body-xs">Runs every 15 minutes on the server. Each rule fires once per invoice. If you turn this on for old overdue invoices, each gets only the latest rule it has reached. Needs server email.</Typography>
      {sales.reminder_rules.map((r, i) => (
        <Box key={i} sx={{ display: "flex", gap: 1, alignItems: "center" }}>
          <Input type="number" sx={{ width: 100 }} value={r.days} onChange={(e) => set({ reminder_rules: sales.reminder_rules.map((x, j) => (j === i ? { days: Number(e.target.value) } : x)) })} />
          <Typography level="body-sm" sx={{ flex: 1 }}>
            days from due date: {ruleText(r.days)}
          </Typography>
          <Button size="sm" variant="plain" color="danger" onClick={() => set({ reminder_rules: sales.reminder_rules.filter((_, j) => j !== i) })}>
            Remove
          </Button>
        </Box>
      ))}
      <Box>
        <Button size="sm" variant="outlined" onClick={() => set({ reminder_rules: [...sales.reminder_rules, { days: 30 }] })}>
          Add rule
        </Button>
      </Box>
      <FormControl>
        <FormLabel>Email subject</FormLabel>
        <Input value={sales.reminder_subject} onChange={(e) => set({ reminder_subject: e.target.value })} />
      </FormControl>
      <FormControl>
        <FormLabel>Email body</FormLabel>
        <Textarea minRows={5} value={sales.reminder_body} onChange={(e) => set({ reminder_body: e.target.value })} />
      </FormControl>
      <FormControl>
        <FormLabel>WhatsApp message (opens WhatsApp with this text; nothing is sent automatically)</FormLabel>
        <Textarea minRows={2} value={sales.whatsapp_body} onChange={(e) => set({ whatsapp_body: e.target.value })} />
      </FormControl>
      <Typography level="body-xs">Placeholders: {"{client} {number} {amount} {total} {due_date} {status} {link} {business}"}</Typography>
      <Box sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap" }}>
        <Button
          size="sm"
          variant="outlined"
          data-testid="invc-run-reminders"
          onClick={async () => {
            const r = await run(() => runReminders());
            if (r) {
              setNote(r.email ? `Reminder run finished: ${r.sent} sent.` : "Server email is not configured, so nothing was sent.");
              setLog(await getReminderLog().catch(() => []));
            }
          }}
        >
          Run reminders now
        </Button>
        <Typography level="body-xs">Save settings first so the run uses your latest rules.</Typography>
      </Box>
      {log.length > 0 && (
        <Table size="sm" data-testid="invc-reminder-log">
          <thead>
            <tr>
              <th>When</th>
              <th>Invoice</th>
              <th>Rule</th>
              <th>Result</th>
            </tr>
          </thead>
          <tbody>
            {log.slice(0, 15).map((l, i) => (
              <tr key={i}>
                <td>{l.at.replace("T", " ").replace("Z", "")}</td>
                <td>
                  {l.number} · {l.client_name}
                </td>
                <td>{l.rule}</td>
                <td>{l.status}</td>
              </tr>
            ))}
          </tbody>
        </Table>
      )}

      <Typography level="title-md" sx={{ mt: 1 }}>
        Online payments (Razorpay)
      </Typography>
      {gw && !gw.configured && (
        <Alert color="neutral" data-testid="invc-gateway-state">
          Not configured. Pay online buttons and payment links stay switched off until you add keys.
        </Alert>
      )}
      {gw?.configured && (
        <Alert color={gw.mode === "live" ? "success" : "warning"} data-testid="invc-gateway-state">
          Configured ({gw.mode || "unknown"} mode, key {gw.key_id}, from {gw.source === "env" ? "server environment" : "these settings"}).
          {gw.webhook_secret ? " Webhook secret set." : " No webhook secret yet, so payments are not marked paid automatically."}
        </Alert>
      )}
      {gw?.source !== "env" && (
        <>
          <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr 1fr" }, gap: 1.5 }}>
            <FormControl>
              <FormLabel>Key id</FormLabel>
              <Input placeholder="rzp_test_..." value={keys.key_id} onChange={(e) => setKeys({ ...keys, key_id: e.target.value })} slotProps={{ input: { autoComplete: "off", "data-testid": "invc-gw-key" } }} />
            </FormControl>
            <FormControl>
              <FormLabel>Key secret</FormLabel>
              <Input type="password" value={keys.key_secret} onChange={(e) => setKeys({ ...keys, key_secret: e.target.value })} slotProps={{ input: { autoComplete: "new-password", "data-testid": "invc-gw-secret" } }} />
            </FormControl>
            <FormControl>
              <FormLabel>Webhook secret</FormLabel>
              <Input type="password" value={keys.webhook_secret} onChange={(e) => setKeys({ ...keys, webhook_secret: e.target.value })} slotProps={{ input: { autoComplete: "new-password" } }} />
            </FormControl>
          </Box>
          <Box>
            <Button size="sm" variant="outlined" onClick={saveKeys} disabled={!keys.key_id} data-testid="invc-gw-save">
              Save payment keys
            </Button>
          </Box>
        </>
      )}
      <Typography level="body-xs">
        In the Razorpay dashboard add a webhook to <code>{window.location.origin}/api/v1/public/invoices/webhook/razorpay</code> for payment_link.paid. Keys can also be set with the RAZORPAY_KEY_ID and RAZORPAY_KEY_SECRET environment variables. Secrets are never shown again after saving.
      </Typography>
      {note && (
        <Typography level="body-sm" color="success">
          {note}
        </Typography>
      )}
    </Box>
  );
}
