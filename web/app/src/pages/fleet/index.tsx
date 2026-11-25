import { useCallback, useEffect, useMemo, useState } from "react";
import { Box, CircularProgress, Container, List, ListItem, ListItemButton, ListItemContent, ListItemDecorator, Typography } from "@mui/joy";
import LocalShippingOutlined from "@mui/icons-material/LocalShippingOutlined";
import DashboardOutlined from "@mui/icons-material/DashboardOutlined";
import DirectionsCarOutlined from "@mui/icons-material/DirectionsCarOutlined";
import EventRepeatOutlined from "@mui/icons-material/EventRepeatOutlined";
import ReportProblemOutlined from "@mui/icons-material/ReportProblemOutlined";
import ChecklistOutlined from "@mui/icons-material/ChecklistOutlined";
import SettingsOutlined from "@mui/icons-material/SettingsOutlined";
import { Header } from "../../components/Header";
import { BizAssistant } from "../../components/BizAssistant";
import { onAiChanged } from "../../lib/ai/biz";
import { deepChoice, deepParam } from "../../lib/deepLink";
import type { User } from "../../api/types";
import { ErrorAlert } from "../hr/ui";
import { createFleetTools, FLEET_HINT, FLEET_SUGGESTIONS } from "./ai/tools";
import { fleet } from "./api";
import type { Me, Settings, Vehicle } from "./api";
import { ACCENT } from "./logic";
import { Overview } from "./Overview";
import { Vehicles } from "./Vehicles";
import { Care } from "./Care";
import { FleetSettings, Inspect, IssueList } from "./Issues";

type View = "overview" | "vehicles" | "reminders" | "issues" | "inspect" | "settings";
const VIEWS: readonly View[] = ["overview", "vehicles", "reminders", "issues", "inspect", "settings"];

function startView(): View {
  const seg = window.location.pathname.replace(/^\/fleet\/?/, "").split("/")[0];
  if ((VIEWS as readonly string[]).includes(seg)) return seg as View;
  return deepChoice<View>("view", VIEWS, "overview");
}

export default function FleetApp({ user }: { user: User }) {
  const [view, setViewState] = useState<View>(startView);
  const setView = useCallback((v: View) => {
    setViewState(v);
    window.history.replaceState(window.history.state, "", `/fleet${v === "overview" ? "" : "/" + v}`);
  }, []);
  const [me, setMe] = useState<Me | null>(null);
  const [settings, setSettings] = useState<Settings | null>(null);
  const [vehicles, setVehicles] = useState<Vehicle[]>([]);
  const [error, setError] = useState("");
  const [rev, setRev] = useState(0);
  const [openId, setOpenId] = useState(deepParam("vehicle"));
  const loadBase = useCallback(() => {
    Promise.all([fleet.me(), fleet.settings(), fleet.vehicles()]).then(([m, s, v]) => { setMe(m); setSettings(s); setVehicles(v); }).catch((e: Error) => setError(e.message));
  }, []);
  useEffect(loadBase, [loadBase]);
  useEffect(() => onAiChanged("fleet", () => { setRev((r) => r + 1); loadBase(); }), [loadBase]);
  useEffect(() => { if (openId) setView("vehicles"); }, [openId, setView]);
  const tools = useMemo(() => createFleetTools(), []);
  const nav: { id: View; label: string; icon: JSX.Element; show: boolean }[] = [
    { id: "overview", label: "Overview", icon: <DashboardOutlined />, show: true },
    { id: "vehicles", label: "Vehicles", icon: <DirectionsCarOutlined />, show: true },
    { id: "reminders", label: "Reminders and renewals", icon: <EventRepeatOutlined />, show: true },
    { id: "issues", label: "Issues", icon: <ReportProblemOutlined />, show: true },
    { id: "inspect", label: "Inspections", icon: <ChecklistOutlined />, show: true },
    { id: "settings", label: "Settings", icon: <SettingsOutlined />, show: !!me?.admin },
  ];
  return (
    <>
      <Header user={user} />
      <Container maxWidth="xl" sx={{ py: { xs: 2, sm: 3 }, px: { xs: 1.5, sm: 3 } }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, mb: 2 }}>
          <LocalShippingOutlined sx={{ fontSize: 28, color: ACCENT }} />
          <Typography level="h2" sx={{ fontSize: { xs: "xl", sm: "xl3" } }}>Fleet</Typography>
          {me && <Box sx={{ ml: "auto" }}><BizAssistant app="fleet" title="Fleet" tools={tools} suggestions={FLEET_SUGGESTIONS} systemHint={FLEET_HINT} /></Box>}
        </Box>
        <Box sx={{ display: "flex", flexDirection: { xs: "column", md: "row" }, gap: { xs: 1, md: 3 } }}>
          <Box component="nav" aria-label="Fleet sections" sx={{ width: { md: 230 }, flexShrink: 0 }}>
            <List orientation="horizontal" sx={{ flexWrap: "wrap", "@media (min-width: 900px)": { flexDirection: "column", flexWrap: "nowrap" }, gap: 0.5 }}>
              {nav.filter((n) => n.show).map((n) => (
                <ListItem key={n.id} sx={{ width: { md: "100%" } }}>
                  <ListItemButton selected={view === n.id} onClick={() => setView(n.id)} aria-current={view === n.id ? "page" : undefined} data-testid={`fleet-nav-${n.id}`} sx={{ borderRadius: "md" }}>
                    <ListItemDecorator>{n.icon}</ListItemDecorator><ListItemContent>{n.label}</ListItemContent>
                  </ListItemButton>
                </ListItem>
              ))}
            </List>
          </Box>
          <Box sx={{ flex: 1, minWidth: 0 }} key={rev}>
            <ErrorAlert err={error} />
            {!settings && !error && <CircularProgress />}
            {me && settings && view === "overview" && <Overview settings={settings} vehicles={vehicles} onOpenVehicle={(id) => setOpenId(id)} />}
            {me && settings && view === "vehicles" && <Vehicles me={me} settings={settings} openId={openId} onOpened={() => setOpenId("")} />}
            {me && settings && view === "reminders" && <Care vehicleId="" vehicles={vehicles} me={me} settings={settings} />}
            {me && settings && view === "issues" && <IssueList vehicleId="" vehicles={vehicles} me={me} />}
            {me && settings && view === "inspect" && <Inspect vehicles={vehicles.filter((v) => v.status !== "sold")} settings={settings} />}
            {me && settings && view === "settings" && me.admin && <FleetSettings settings={settings} onSaved={(s) => setSettings(s)} />}
          </Box>
        </Box>
      </Container>
    </>
  );
}
