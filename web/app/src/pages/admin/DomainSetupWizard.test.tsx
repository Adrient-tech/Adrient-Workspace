import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { DomainSetupWizard } from "./DomainSetupWizard";
import { dnsApi, type DnsPlan, type DnsCheck } from "./dnsApi";

const rec = (kind: string, name: string, type: string, value: string, required = true) => ({ kind, name, type, value, ttl: 3600, required });

const plan: DnsPlan = {
  domain: "example.com",
  records: [
    rec("mx", "example.com", "MX", "10 mail.example.com."),
    rec("spf", "example.com", "TXT", "v=spf1 mx -all"),
    rec("srv", "_imaps._tcp.example.com", "SRV", "0 1 993 mail.example.com.", false),
  ],
  settings: { dmarc_policy: "none", dmarc_rua: "", mta_sts_mode: "testing", mta_sts_id: "1", tls_rpt_rua: "", mail_host: "", spf_ips: [] },
  keys: [],
  clean_days: 15,
  ramp_hint: "SPF, DKIM and DMARC have passed for 15 days. You can move DMARC to p=quarantine.",
  mail_host: "mail.example.com",
  web_host: "workspace.example.com",
  mail_backend: "imap",
};

const res = (r: ReturnType<typeof rec>, status: "pass" | "warn" | "fail", message: string, found: string[] | null) => ({
  ...r, status, expected: r.value, found, message,
});

const first: DnsCheck = {
  results: [
    res(plan.records[0], "fail", "No MX record found.", null),
    res(plan.records[1], "pass", "ok", ["v=spf1 mx -all"]),
    res(plan.records[2], "warn", "No SRV record found.", null),
  ],
  pass: 1, warn: 1, fail: 1, clean: false, clean_days: 15, ramp_hint: plan.ramp_hint,
};
const second: DnsCheck = {
  ...first,
  results: first.results.map((r) => ({ ...r, status: "pass" as const, message: "ok" })),
  pass: 3, warn: 0, fail: 0, clean: true,
};

beforeEach(() => vi.restoreAllMocks());

describe("DomainSetupWizard", () => {
  it("shows expected vs found per row and turns green after re-check", async () => {
    vi.spyOn(dnsApi, "plan").mockResolvedValue(plan);
    const check = vi.spyOn(dnsApi, "check").mockResolvedValueOnce(first).mockResolvedValue(second);
    render(<DomainSetupWizard domainId="d1" domain="example.com" pollMs={600000} />);
    await waitFor(() => expect(screen.getByTestId("rec-mx").querySelector("[data-status]")?.getAttribute("data-status")).toBe("fail"));
    expect(screen.getByTestId("why-mx").textContent).toContain("Found: nothing");
    expect(screen.getByTestId("rec-spf").querySelector("[data-status]")?.getAttribute("data-status")).toBe("pass");
    expect(screen.getByTestId("rec-srv").querySelector("[data-status]")?.getAttribute("data-status")).toBe("warn");
    fireEvent.click(screen.getByRole("button", { name: "Re-check" }));
    await waitFor(() => expect(screen.getByTestId("rec-mx").querySelector("[data-status]")?.getAttribute("data-status")).toBe("pass"));
    expect(check.mock.calls.length).toBeGreaterThanOrEqual(2);
  });

  it("shows the DMARC ramp hint after 14 clean days and applies quarantine", async () => {
    vi.spyOn(dnsApi, "plan").mockResolvedValue(plan);
    vi.spyOn(dnsApi, "check").mockResolvedValue(first);
    const put = vi.spyOn(dnsApi, "putSettings").mockResolvedValue({ ...plan.settings, dmarc_policy: "quarantine" });
    render(<DomainSetupWizard domainId="d1" domain="example.com" pollMs={600000} />);
    expect((await screen.findByTestId("dmarc-hint")).textContent).toContain("p=quarantine");
    fireEvent.click(screen.getByText("Use p=quarantine"));
    await waitFor(() => expect(put).toHaveBeenCalledWith("d1", expect.objectContaining({ dmarc_policy: "quarantine" })));
  });

  it("copies the record value", async () => {
    vi.spyOn(dnsApi, "plan").mockResolvedValue(plan);
    vi.spyOn(dnsApi, "check").mockResolvedValue(first);
    const write = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText: write } });
    render(<DomainSetupWizard domainId="d1" domain="example.com" pollMs={600000} />);
    fireEvent.click(await screen.findByLabelText("Copy value mx"));
    expect(write).toHaveBeenCalledWith("10 mail.example.com.");
  });
});
