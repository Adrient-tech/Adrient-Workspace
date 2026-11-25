import { Box, Button, LinearProgress, Sheet, Stack, Table, Typography } from "@mui/joy";
import { dealContacts, dealsWithoutNextStep, forecast, funnel, groupFollowups, isCold, isOpen, isStalled, lastTouch, money, stageName, weeklyProgress, winLoss, winRateBy, wonThisMonth, daysInStage } from "./logic";
import { Empty, Stat, ago, companyName, contactName, fmtDate } from "./ui";
import type { Data } from "./ui";
import { useEffect, useState } from "react";
import { getSummary, type CrmSummary } from "./api";
import { stageWeight } from "./logic";
import type { Settings } from "./types";

/** The four headline numbers from server totals (same rules as forecast()). */
function summaryForecast(sum: CrmSummary, s: Settings) {
  const open = sum.by_stage.filter((t) => t.stage !== "won" && t.stage !== "lost");
  return {
    openCount: open.reduce((a, t) => a + t.count, 0),
    openValue: open.reduce((a, t) => a + t.value, 0),
    weighted: open.reduce((a, t) => a + t.value * stageWeight(s, t.stage), 0),
    openMrr: open.reduce((a, t) => a + t.mrr, 0),
    wonMrr: sum.by_stage.filter((t) => t.stage === "won").reduce((a, t) => a + t.mrr, 0),
  };
}

export function Dashboard({ data, onOpenDeal }: { data: Data; onOpenDeal: (id: string) => void }) {
  const s = data.settings;
  // Totals come from the server over every deal; the loaded list is a window.
  const [sum, setSum] = useState<CrmSummary | null>(null);
  useEffect(() => {
    let live = true;
    getSummary()
      .then((x) => live && setSum(x))
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [data]);
  const local = forecast(data.deals, s);
  const f = sum ? summaryForecast(sum, s) : local;
  const won = sum ? sum.won_this_month : wonThisMonth(data.deals);
  const agenda = groupFollowups(data.followups);
  const stalled = data.deals.filter((d) => isStalled(d, s.stale_days)).sort((a, b) => daysInStage(b) - daysInStage(a)).slice(0, 5);
  const noStep = dealsWithoutNextStep(data.deals, data.followups).slice(0, 5);
  const cold = data.people
    .filter((p) => p.status === "lead")
    .map((p) => ({ p, last: lastTouch(data.activities, [p.contact_id]) }))
    .filter((x) => isCold(x.last, s.cold_days))
    .slice(0, 5);
  const noNext = data.deals.filter((d) => isOpen(d) && !data.followups.some((fu) => fu.deal_id === d.id && !fu.done)).slice(0, 5);
  const wp = weeklyProgress(data.activities, s.weekly_target);
  const goalPct = s.monthly_goal > 0 ? Math.min(100, Math.round((won / s.monthly_goal) * 100)) : 0;

  return (
    <Box data-testid="crm-dashboard">
      <Stack direction="row" gap={1.5} flexWrap="wrap" sx={{ mb: 2 }}>
        <Stat label="Open pipeline" value={money(f.openValue, s.currency)} sub={`${f.openCount} deals`} testid="crm-stat-pipeline" />
        <Stat label="Weighted forecast" value={money(f.weighted, s.currency)} />
        <Stat label="Won this month" value={money(won, s.currency)} testid="crm-stat-won" />
        <Stat label="MRR won" value={money(f.wonMrr, s.currency)} sub={`+${money(f.openMrr, s.currency)} in open deals`} testid="crm-stat-mrr" />
      </Stack>
      {s.monthly_goal > 0 && (
        <Sheet variant="outlined" sx={{ p: 1.5, borderRadius: "md", mb: 2 }}>
          <Typography level="title-sm">
            Monthly goal: {money(won, s.currency)} of {money(s.monthly_goal, s.currency)} ({goalPct}%)
          </Typography>
          <LinearProgress determinate value={goalPct} sx={{ mt: 1 }} />
        </Sheet>
      )}
      {wp.target > 0 && (
        <Typography level="body-sm" sx={{ mb: 2 }}>
          Touches this week: {wp.done} of {wp.target}
        </Typography>
      )}
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1fr 1fr" }, gap: 2 }}>
        <Panel title="Needs attention today">
          {agenda.overdue.length + agenda.today.length === 0 && <Typography level="body-sm">Nothing due. You are caught up.</Typography>}
          {[...agenda.overdue, ...agenda.today].slice(0, 6).map((fu) => (
            <Typography key={fu.id} level="body-sm" sx={{ color: fu.due_date.slice(0, 10) < new Date().toISOString().slice(0, 10) ? "danger.plainColor" : undefined }}>
              {fu.title} · {fmtDate(fu.due_date)}
            </Typography>
          ))}
        </Panel>
        <Panel title="Stalled deals">
          {stalled.length === 0 && <Typography level="body-sm">No deal is stuck past {s.stale_days} days.</Typography>}
          {stalled.map((d) => (
            <Button key={d.id} variant="plain" size="sm" sx={{ justifyContent: "flex-start" }} onClick={() => onOpenDeal(d.id)}>
              {d.title} · {daysInStage(d)}d in {stageName(s, d.stage)}
            </Button>
          ))}
        </Panel>
        <Panel title="Open deals with no next step">
          {noStep.length === 0 && <Typography level="body-sm">Every open deal has a follow-up booked.</Typography>}
          {noStep.map((d) => (
            <Button key={d.id} variant="plain" size="sm" sx={{ justifyContent: "flex-start" }} onClick={() => onOpenDeal(d.id)} data-testid="crm-no-next-step">
              {d.title} · {stageName(s, d.stage)}
            </Button>
          ))}
        </Panel>
        <Panel title="Leads going cold">
          {cold.length === 0 && <Typography level="body-sm">Every lead has had a recent touch.</Typography>}
          {cold.map(({ p, last }) => (
            <Typography key={p.id} level="body-sm">
              {p.display_name} · last touch {ago(last)}
            </Typography>
          ))}
        </Panel>
        <Panel title="Open deals with no next step">
          {noNext.length === 0 && <Typography level="body-sm">Every open deal has a follow-up.</Typography>}
          {noNext.map((d) => (
            <Button key={d.id} variant="plain" size="sm" sx={{ justifyContent: "flex-start" }} onClick={() => onOpenDeal(d.id)}>
              {d.title} · {companyName(data, d.company_id) || contactName(data, d.contact_id)}
            </Button>
          ))}
        </Panel>
      </Box>
      {data.deals.length === 0 && (
        <Box sx={{ mt: 2 }}>
          <Empty>Your CRM is empty. Add leads, companies and deals to see your numbers here.</Empty>
        </Box>
      )}
    </Box>
  );
}

function Panel({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <Sheet variant="outlined" sx={{ p: 1.5, borderRadius: "md" }}>
      <Typography level="title-md" sx={{ mb: 1 }}>
        {title}
      </Typography>
      <Stack gap={0.5} alignItems="flex-start">
        {children}
      </Stack>
    </Sheet>
  );
}

export function Forecast({ data }: { data: Data }) {
  const s = data.settings;
  const f = forecast(data.deals, s);
  const fn = funnel(data.deals, data.history, s);
  const wl = winLoss(data.deals);
  const bySource = winRateBy(data.deals, (d) => d.source);
  const max = Math.max(1, ...fn.map((r) => r.reached));
  const avgDays = s.stages.map((st) => {
    const open = data.deals.filter((d) => d.stage === st.id);
    return { st, n: open.length, avg: open.length ? Math.round(open.reduce((a, d) => a + daysInStage(d), 0) / open.length) : 0 };
  });
  return (
    <Box data-testid="crm-forecast">
      <Typography level="h4" sx={{ mb: 1.5 }}>
        Forecast
      </Typography>
      <Stack direction="row" gap={1.5} flexWrap="wrap" sx={{ mb: 2 }}>
        <Stat label="Open value" value={money(f.openValue, s.currency)} />
        <Stat label="Weighted" value={money(f.weighted, s.currency)} />
        <Stat label="Win rate" value={`${wl.winRate}%`} sub={`${wl.won} won, ${wl.lost} lost`} />
        <Stat label="Avg won deal" value={money(wl.avgWon, s.currency)} sub={`lost avg ${money(wl.avgLost, s.currency)}`} />
      </Stack>

      <Typography level="title-md" sx={{ mb: 1 }}>
        Where deals leak
      </Typography>
      <Sheet variant="outlined" sx={{ p: 1.5, borderRadius: "md", mb: 2 }} data-testid="crm-funnel">
        {fn.map((r) => (
          <Box key={r.stage.id} sx={{ mb: 1 }}>
            <Typography level="body-sm">
              {r.stage.name}: {r.reached} deals {r.toNext !== null && `· ${r.toNext}% move on`}
            </Typography>
            <LinearProgress determinate value={(r.reached / max) * 100} />
          </Box>
        ))}
        <Typography level="body-xs" sx={{ mt: 1 }}>
          Average time in each open stage: {avgDays.map((a) => `${a.st.name} ${a.avg}d`).join(", ")}
        </Typography>
      </Sheet>

      <Typography level="title-md" sx={{ mb: 1 }}>
        Expected closes by month
      </Typography>
      <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto", mb: 2 }}>
        <Table>
          <thead>
            <tr>
              <th>Month</th>
              <th style={{ textAlign: "right" }}>Open value</th>
              <th style={{ textAlign: "right" }}>Weighted</th>
            </tr>
          </thead>
          <tbody>
            {f.byMonth.map((m) => (
              <tr key={m.month}>
                <td>{m.month}</td>
                <td style={{ textAlign: "right" }}>{money(m.value, s.currency)}</td>
                <td style={{ textAlign: "right" }}>{money(m.weighted, s.currency)}</td>
              </tr>
            ))}
            {f.noDate > 0 && (
              <tr>
                <td>No close date</td>
                <td style={{ textAlign: "right" }}>{money(f.noDate, s.currency)}</td>
                <td />
              </tr>
            )}
            {f.byMonth.length === 0 && f.noDate === 0 && (
              <tr>
                <td colSpan={3}>No open deals.</td>
              </tr>
            )}
          </tbody>
        </Table>
      </Sheet>

      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1fr 1fr" }, gap: 2 }}>
        <Panel title="Why deals close">
          {wl.wonReasons.length === 0 && <Typography level="body-sm">No reasons recorded yet.</Typography>}
          {wl.wonReasons.map(([r, n]) => (
            <Typography key={r} level="body-sm">
              {r}: {n}
            </Typography>
          ))}
        </Panel>
        <Panel title="Why deals are lost">
          {wl.lostReasons.length === 0 && <Typography level="body-sm">No reasons recorded yet.</Typography>}
          {wl.lostReasons.map(([r, n]) => (
            <Typography key={r} level="body-sm">
              {r}: {n}
            </Typography>
          ))}
        </Panel>
        <Panel title="Win rate by lead source">
          {bySource.length === 0 && <Typography level="body-sm">No closed deals yet.</Typography>}
          {bySource.map((r) => (
            <Typography key={r.key} level="body-sm">
              {r.key}: {r.rate}% ({r.won}/{r.closed})
            </Typography>
          ))}
        </Panel>
        <Panel title="Open deals by committee size">
          {data.deals
            .filter(isOpen)
            .slice(0, 5)
            .map((d) => (
              <Typography key={d.id} level="body-sm">
                {d.title}: {new Set(dealContacts(d).filter(Boolean)).size} people
              </Typography>
            ))}
        </Panel>
      </Box>
    </Box>
  );
}
