import { useCallback, useEffect, useMemo, useState } from "react";
import { Box, CircularProgress, Container, List, ListItem, ListItemButton, ListItemContent, ListItemDecorator, Typography } from "@mui/joy";
import CalendarViewWeekOutlined from "@mui/icons-material/CalendarViewWeekOutlined";
import GroupsOutlined from "@mui/icons-material/GroupsOutlined";
import SwapHorizOutlined from "@mui/icons-material/SwapHorizOutlined";
import AccessTimeOutlined from "@mui/icons-material/AccessTimeOutlined";
import SettingsOutlined from "@mui/icons-material/SettingsOutlined";
import EventNoteOutlined from "@mui/icons-material/EventNoteOutlined";
import { Header } from "../../components/Header";
import { BizAssistant } from "../../components/BizAssistant";
import { onAiChanged } from "../../lib/ai/biz";
import { deepChoice } from "../../lib/deepLink";
import type { User } from "../../api/types";
import { ErrorAlert } from "../hr/ui";
import { createShiftsTools, SHIFTS_HINT, SHIFTS_SUGGESTIONS } from "./ai/tools";
import { shiftsApi } from "./api";
import type { Me } from "./api";
import { ACCENT } from "./logic";
import { Schedule } from "./Schedule";
import { Team } from "./Team";
import { Swaps } from "./Swaps";
import { Timesheet } from "./Timesheet";
import { ShiftSettings } from "./Settings";

type View = "schedule" | "team" | "swaps" | "timesheet" | "settings";
const VIEWS: readonly View[] = ["schedule", "team", "swaps", "timesheet", "settings"];

function startView(): View {
  const seg = window.location.pathname.replace(/^\/shifts\/?/, "").split("/")[0];
  if ((VIEWS as readonly string[]).includes(seg)) return seg as View;
  return deepChoice<View>("view", VIEWS, "schedule");
}

export default function ShiftsApp({ user }: { user: User }) {
  const [view, setViewState] = useState<View>(startView);
  const setView = useCallback((v: View) => {
    setViewState(v);
    window.history.replaceState(window.history.state, "", `/shifts${v === "schedule" ? "" : "/" + v}`);
  }, []);
  const [me, setMe] = useState<Me | null>(null);
  const [error, setError] = useState("");
  const [rev, setRev] = useState(0);
  useEffect(() => {
    shiftsApi.me().then(setMe).catch((e: Error) => setError(e.message));
  }, []);
  useEffect(() => onAiChanged("shifts", () => setRev((r) => r + 1)), []);
  const tools = useMemo(() => createShiftsTools(), []);
  const nav: { id: View; label: string; icon: JSX.Element; show: boolean }[] = [
    { id: "schedule", label: "Schedule", icon: <CalendarViewWeekOutlined />, show: true },
    { id: "team", label: "Team", icon: <GroupsOutlined />, show: true },
    { id: "swaps", label: "Swaps", icon: <SwapHorizOutlined />, show: true },
    { id: "timesheet", label: "Timesheet", icon: <AccessTimeOutlined />, show: true },
    { id: "settings", label: "Rules and coverage", icon: <SettingsOutlined />, show: !!me?.scheduler },
  ];
  return (
    <>
      <Header user={user} />
      <Container maxWidth="xl" sx={{ py: { xs: 2, sm: 3 }, px: { xs: 1.5, sm: 3 } }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, mb: 2 }}>
          <EventNoteOutlined sx={{ fontSize: 28, color: ACCENT }} />
          <Typography level="h2" sx={{ fontSize: { xs: "xl", sm: "xl3" } }}>Shifts</Typography>
          {me && (
            <Box sx={{ ml: "auto" }}>
              <BizAssistant app="shifts" title={me.scheduler ? "Rota planner" : "My shifts"} tools={tools} suggestions={SHIFTS_SUGGESTIONS} systemHint={SHIFTS_HINT} />
            </Box>
          )}
        </Box>
        <Box sx={{ display: "flex", flexDirection: { xs: "column", md: "row" }, gap: { xs: 1, md: 3 } }}>
          <Box component="nav" aria-label="Shifts sections" sx={{ width: { md: 200 }, flexShrink: 0 }}>
            <List orientation="horizontal" sx={{ flexWrap: "wrap", "@media (min-width: 900px)": { flexDirection: "column", flexWrap: "nowrap" }, gap: 0.5 }}>
              {nav.filter((n) => n.show).map((n) => (
                <ListItem key={n.id} sx={{ width: { md: "100%" } }}>
                  <ListItemButton selected={view === n.id} onClick={() => setView(n.id)} aria-current={view === n.id ? "page" : undefined} data-testid={`shifts-nav-${n.id}`} sx={{ borderRadius: "md" }}>
                    <ListItemDecorator>{n.icon}</ListItemDecorator>
                    <ListItemContent>{n.label}</ListItemContent>
                  </ListItemButton>
                </ListItem>
              ))}
            </List>
          </Box>
          <Box sx={{ flex: 1, minWidth: 0 }} key={rev}>
            <ErrorAlert err={error} />
            {!me && !error && <CircularProgress />}
            {me && view === "schedule" && <Schedule me={me} />}
            {me && view === "team" && <Team me={me} />}
            {me && view === "swaps" && <Swaps />}
            {me && view === "timesheet" && <Timesheet me={me} />}
            {me && view === "settings" && me.scheduler && <ShiftSettings />}
          </Box>
        </Box>
      </Container>
    </>
  );
}
