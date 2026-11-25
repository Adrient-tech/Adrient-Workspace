import { useEffect, useState } from "react";
import { Alert, Box, Button, Chip, Dropdown, Input, Menu, MenuButton, Typography } from "@mui/joy";
import BeachAccessIcon from "@mui/icons-material/BeachAccessOutlined";
import { svc } from "./svcApi";

/** Out of office for the desk: while it lasts, routing skips you in every team. */
export function OutOfOffice() {
  const [until, setUntil] = useState<string | null>(null);
  const [pick, setPick] = useState("");
  const [err, setErr] = useState("");
  const load = () => svc.ooo().then(setUntil).catch(() => setUntil(null));
  useEffect(() => { void load(); }, []);
  async function save(v: string | null) {
    setErr("");
    try {
      await svc.setOoo(v ? new Date(v + "T23:59:00").toISOString() : null);
      setPick("");
      await load();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  }
  return (
    <Dropdown>
      <MenuButton size="sm" variant={until ? "soft" : "plain"} color={until ? "warning" : "neutral"} startDecorator={<BeachAccessIcon fontSize="small" />} data-testid="ooo-btn">
        {until ? `Away until ${new Date(until).toLocaleDateString(undefined, { day: "numeric", month: "short" })}` : "Out of office"}
      </MenuButton>
      <Menu size="sm" placement="bottom-end" sx={{ p: 1.5, width: 280 }} onKeyDown={(e) => e.stopPropagation()}>
        <Typography level="title-sm">Out of office</Typography>
        <Typography level="body-xs" sx={{ color: "text.tertiary", mb: 1 }}>New tickets are not assigned to you in any team until the end of this day.</Typography>
        <Box sx={{ display: "flex", gap: 1 }}>
          <Input size="sm" type="date" value={pick} onChange={(e) => setPick(e.target.value)} slotProps={{ input: { min: new Date().toISOString().slice(0, 10), "data-testid": "ooo-date" } as never }} />
          <Button size="sm" disabled={!pick} onClick={() => save(pick)} data-testid="ooo-save">Set</Button>
        </Box>
        {until && <Chip size="sm" variant="soft" color="warning" sx={{ mt: 1 }} onClick={() => save(null)}>Away until {new Date(until).toLocaleDateString()} · I am back</Chip>}
        {err && <Alert color="danger" size="sm" sx={{ mt: 1 }}>{err}</Alert>}
      </Menu>
    </Dropdown>
  );
}
