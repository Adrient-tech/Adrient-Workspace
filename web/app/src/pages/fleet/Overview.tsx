import { useCallback, useEffect, useState } from "react";
import { Box, Chip, Typography } from "@mui/joy";
import { Card, ErrorAlert, Row, Stat, useErr } from "../hr/ui";
import { fmtDay, money } from "../ops/dates";
import { fleet } from "./api";
import type { Dashboard, Settings, Vehicle } from "./api";
import { STATUS_LABEL, dueText } from "./logic";

export function Overview({ settings, vehicles, onOpenVehicle }: { settings: Settings; vehicles: Vehicle[]; onOpenVehicle: (id: string) => void }) {
  const { err, run } = useErr();
  const [d, setD] = useState<Dashboard | null>(null);
  const load = useCallback(() => void run(async () => setD(await fleet.dashboard())), [run]);
  useEffect(load, [load]);
  const name = (id: string) => vehicles.find((v) => v.id === id)?.name ?? "";
  if (!d) return <ErrorAlert err={err} />;
  const max = Math.max(1, ...d.months.map((m) => m.fuel + m.service));
  const overdue = d.overdue_reminders ?? [];
  const soon = d.due_soon_reminders ?? [];
  const ren = d.renewals_due ?? [];
  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
      <ErrorAlert err={err} />
      <Row>
        <Stat label="Vehicles" value={d.vehicles} hint={Object.entries(d.by_status).map(([k, v]) => `${v} ${STATUS_LABEL[k]?.toLowerCase() ?? k}`).join(", ")} testId="stat-vehicles" />
        <Stat label="Overdue" value={overdue.length + ren.filter((r) => r.status === "overdue").length} hint="reminders and renewals" testId="stat-overdue" />
        <Stat label="Due soon" value={soon.length + ren.filter((r) => r.status === "due_soon").length} />
        <Stat label="Open issues" value={d.open_issues} />
        <Stat label={`Cost per ${settings.meter_unit}`} value={d.cost_per_distance ? money(d.cost_per_distance, settings.currency) : "—"} hint="last 6 months" />
      </Row>
      <Card title="Needs attention">
        {!overdue.length && !soon.length && !ren.length && <Typography level="body-sm">Nothing is overdue or due soon.</Typography>}
        {[...overdue, ...soon].map((x) => (
          <Row key={x.id} sx={{ alignItems: "center", mb: 0.5 }}>
            <Chip size="sm" variant="soft" color={x.status === "overdue" ? "danger" : "warning"}>{STATUS_LABEL[x.status ?? ""]}</Chip>
            <Typography level="body-sm" sx={{ cursor: "pointer" }} onClick={() => onOpenVehicle(x.vehicle_id)}>{x.title} · <b>{name(x.vehicle_id)}</b> · {dueText(x.days_left, x.meter_left, settings.meter_unit)}</Typography>
          </Row>
        ))}
        {ren.map((x) => (
          <Row key={x.id} sx={{ alignItems: "center", mb: 0.5 }}>
            <Chip size="sm" variant="soft" color={x.status === "overdue" ? "danger" : "warning"}>{STATUS_LABEL[x.status ?? ""]}</Chip>
            <Typography level="body-sm" sx={{ cursor: "pointer" }} onClick={() => onOpenVehicle(x.vehicle_id)}>{x.title} · <b>{name(x.vehicle_id)}</b> · due {fmtDay(x.due_date)}</Typography>
          </Row>
        ))}
      </Card>
      <Card title="Operating cost, last 6 months">
        <Box sx={{ display: "flex", gap: 1, alignItems: "flex-end", height: 130, overflowX: "auto" }} role="img" aria-label="Monthly fuel and service cost">
          {d.months.map((m) => (
            <Box key={m.month} sx={{ flex: 1, minWidth: 48, textAlign: "center" }}>
              <Box sx={{ height: 100, display: "flex", flexDirection: "column", justifyContent: "flex-end" }} title={`${m.month}: fuel ${money(m.fuel, settings.currency)}, service ${money(m.service, settings.currency)}`}>
                <Box sx={{ height: `${(m.service / max) * 100}%`, bgcolor: "warning.400" }} />
                <Box sx={{ height: `${(m.fuel / max) * 100}%`, bgcolor: "primary.500" }} />
              </Box>
              <Typography level="body-xs">{m.month.slice(5)}</Typography>
            </Box>
          ))}
        </Box>
        <Typography level="body-xs" sx={{ mt: 0.5 }}>Blue: fuel · Orange: service. Inspections failed in the last 30 days: {d.failed_inspections_30d}.</Typography>
      </Card>
    </Box>
  );
}
