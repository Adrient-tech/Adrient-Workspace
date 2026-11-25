import { Box, Typography } from "@mui/joy";
import { arcPath, fmtShares, PALETTE, type Slice } from "./logic";
import type { VestPoint } from "./api";

/** Ownership donut with a legend. Slices come from server figures; only geometry is computed here. */
export function Donut({ items, centre, size = 200 }: { items: Slice[]; centre?: string; size?: number }) {
  const total = items.reduce((s, i) => s + i.value, 0);
  let acc = 0;
  return (
    <Box sx={{ display: "flex", gap: 2, alignItems: "center", flexWrap: "wrap" }}>
      <svg width={size} height={size} viewBox="0 0 100 100" role="img" aria-label="Ownership chart" data-testid="donut">
        {total > 0 && items.map((i) => {
          const from = acc / total;
          acc += i.value;
          return <path key={i.label} d={arcPath(50, 50, 28, 46, from, acc / total)} fill={i.color}><title>{`${i.label}: ${fmtShares(i.value)}`}</title></path>;
        })}
        {centre && <text x="50" y="52" textAnchor="middle" fontSize="7" fill="currentColor">{centre}</text>}
      </svg>
      <Box sx={{ display: "grid", gap: 0.4, minWidth: 160 }}>
        {items.map((i) => (
          <Box key={i.label} sx={{ display: "flex", gap: 1, alignItems: "center" }}>
            <Box sx={{ width: 10, height: 10, borderRadius: 2, bgcolor: i.color, flexShrink: 0 }} />
            <Typography level="body-xs" sx={{ flex: 1 }}>{i.label}</Typography>
            <Typography level="body-xs">{total ? ((i.value / total) * 100).toFixed(1) : "0"}%</Typography>
          </Box>
        ))}
      </Box>
    </Box>
  );
}

/** Horizontal stacked bar: each exit's payout split by holder. Values are exact cents from the server. */
export function PayoutBars({ rows, colorOf }: { rows: { label: string; parts: { name: string; value: number }[] }[]; colorOf: (name: string) => string }) {
  return (
    <Box sx={{ display: "grid", gap: 1 }}>
      {rows.map((r) => {
        const total = r.parts.reduce((s, p) => s + p.value, 0) || 1;
        return (
          <Box key={r.label}>
            <Typography level="body-xs">{r.label}</Typography>
            <Box sx={{ display: "flex", height: 22, borderRadius: "sm", overflow: "hidden", bgcolor: "background.level2" }}>
              {r.parts.filter((p) => p.value > 0).map((p) => <Box key={p.name} title={`${p.name}`} sx={{ width: `${(p.value / total) * 100}%`, bgcolor: colorOf(p.name) }} />)}
            </Box>
          </Box>
        );
      })}
    </Box>
  );
}

/** Vesting curve: step line of vested shares over time. */
export function VestingCurve({ points, total, today }: { points: VestPoint[]; total: number; today: string }) {
  if (points.length < 2 || total <= 0) return null;
  const t0 = Date.parse(points[0].date), t1 = Date.parse(points[points.length - 1].date);
  const x = (d: string) => 8 + ((Date.parse(d) - t0) / Math.max(1, t1 - t0)) * 284;
  const y = (v: number) => 92 - (v / total) * 80;
  let d = `M${x(points[0].date)},${y(0)}`;
  let prev = 0;
  for (const p of points) { d += ` L${x(p.date).toFixed(1)},${y(prev).toFixed(1)} L${x(p.date).toFixed(1)},${y(p.vested).toFixed(1)}`; prev = p.vested; }
  const tx = Math.min(292, Math.max(8, x(today)));
  return (
    <svg viewBox="0 0 300 100" width="100%" height={120} role="img" aria-label="Vesting schedule" data-testid="vesting-curve">
      <path d={d} fill="none" stroke={PALETTE[0]} strokeWidth={2} />
      <line x1={tx} x2={tx} y1={8} y2={94} stroke="#EB5757" strokeDasharray="3 3" />
      <text x={tx + 3} y={14} fontSize="6" fill="currentColor">today</text>
      <text x="8" y="99" fontSize="6" fill="currentColor">{points[0].date}</text>
      <text x="292" y="99" fontSize="6" textAnchor="end" fill="currentColor">{points[points.length - 1].date}</text>
    </svg>
  );
}
