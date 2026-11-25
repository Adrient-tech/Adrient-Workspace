// Reports a client-side data export (CSV built in the browser) to the audit
// log. Best-effort: a failure never blocks the download.

export type AuditExportService = "crm" | "invoicing" | "accounting";

export function auditExport(service: AuditExportService, kind: string, count = 0): void {
  try {
    void fetch("/api/v1/audit/client-event", {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ service, kind, count }),
      keepalive: true,
    }).catch(() => undefined);
  } catch {
    /* ignore */
  }
}
