import { useCallback, useEffect, useMemo, useState } from "react";
import { Box, CircularProgress, Container, List, ListItem, ListItemButton, ListItemContent, ListItemDecorator, Typography } from "@mui/joy";
import HomeRepairServiceOutlined from "@mui/icons-material/HomeRepairServiceOutlined";
import HomeOutlined from "@mui/icons-material/HomeOutlined";
import InboxOutlined from "@mui/icons-material/InboxOutlined";
import WorkOutline from "@mui/icons-material/WorkOutline";
import CalendarMonthOutlined from "@mui/icons-material/CalendarMonthOutlined";
import SellOutlined from "@mui/icons-material/SellOutlined";
import { Header } from "../../components/Header";
import { BizAssistant } from "../../components/BizAssistant";
import { onAiChanged } from "../../lib/ai/biz";
import { deepChoice, deepParam } from "../../lib/deepLink";
import type { User } from "../../api/types";
import { ErrorAlert } from "../hr/ui";
import { createFieldTools, FS_HINT, FS_SUGGESTIONS } from "./ai/tools";
import { fs } from "./api";
import type { Me, Service, Settings, Team } from "./api";
import { ACCENT } from "./logic";
import { Home, WeekSchedule } from "./Schedule";
import { Requests } from "./Requests";
import { Jobs } from "./Jobs";
import { Catalogue } from "./Catalogue";

type View = "home" | "requests" | "jobs" | "schedule" | "catalogue";
const VIEWS: readonly View[] = ["home", "requests", "jobs", "schedule", "catalogue"];

function startView(): View {
  const seg = window.location.pathname.replace(/^\/fieldservice\/?/, "").split("/")[0];
  if ((VIEWS as readonly string[]).includes(seg)) return seg as View;
  return deepChoice<View>("view", VIEWS, "home");
}

export default function FieldServiceApp({ user }: { user: User }) {
  const [view, setViewState] = useState<View>(startView);
  const setView = useCallback((v: View) => {
    setViewState(v);
    window.history.replaceState(window.history.state, "", `/fieldservice${v === "home" ? "" : "/" + v}`);
  }, []);
  const [me, setMe] = useState<Me | null>(null);
  const [settings, setSettings] = useState<Settings | null>(null);
  const [services, setServices] = useState<Service[]>([]);
  const [team, setTeam] = useState<Team[]>([]);
  const [error, setError] = useState("");
  const [rev, setRev] = useState(0);
  const [openJob, setOpenJob] = useState(deepParam("job"));
  const loadBase = useCallback(() => {
    Promise.all([fs.me(), fs.settings(), fs.services(), fs.team()]).then(([m, s, sv, t]) => { setMe(m); setSettings(s); setServices(sv); setTeam(t); }).catch((e: Error) => setError(e.message));
  }, []);
  useEffect(loadBase, [loadBase]);
  useEffect(() => onAiChanged("fieldservice", () => setRev((r) => r + 1)), []);
  useEffect(() => { if (openJob) setView("jobs"); }, [openJob, setView]);
  const tools = useMemo(() => createFieldTools(), []);
  const nav: { id: View; label: string; icon: JSX.Element }[] = [
    { id: "home", label: "Home", icon: <HomeOutlined /> },
    { id: "requests", label: "Requests and quotes", icon: <InboxOutlined /> },
    { id: "jobs", label: "Jobs", icon: <WorkOutline /> },
    { id: "schedule", label: "Schedule", icon: <CalendarMonthOutlined /> },
    { id: "catalogue", label: "Services and setup", icon: <SellOutlined /> },
  ];
  return (
    <>
      <Header user={user} />
      <Container maxWidth="xl" sx={{ py: { xs: 2, sm: 3 }, px: { xs: 1.5, sm: 3 } }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, mb: 2 }}>
          <HomeRepairServiceOutlined sx={{ fontSize: 28, color: ACCENT }} />
          <Typography level="h2" sx={{ fontSize: { xs: "xl", sm: "xl3" } }}>Field service</Typography>
          {me && <Box sx={{ ml: "auto" }}><BizAssistant app="fieldservice" title="Field service" tools={tools} suggestions={FS_SUGGESTIONS} systemHint={FS_HINT} /></Box>}
        </Box>
        <Box sx={{ display: "flex", flexDirection: { xs: "column", md: "row" }, gap: { xs: 1, md: 3 } }}>
          <Box component="nav" aria-label="Field service sections" sx={{ width: { md: 220 }, flexShrink: 0 }}>
            <List orientation="horizontal" sx={{ flexWrap: "wrap", "@media (min-width: 900px)": { flexDirection: "column", flexWrap: "nowrap" }, gap: 0.5 }}>
              {nav.map((n) => (
                <ListItem key={n.id} sx={{ width: { md: "100%" } }}>
                  <ListItemButton selected={view === n.id} onClick={() => setView(n.id)} aria-current={view === n.id ? "page" : undefined} data-testid={`fs-nav-${n.id}`} sx={{ borderRadius: "md" }}>
                    <ListItemDecorator>{n.icon}</ListItemDecorator><ListItemContent>{n.label}</ListItemContent>
                  </ListItemButton>
                </ListItem>
              ))}
            </List>
          </Box>
          <Box sx={{ flex: 1, minWidth: 0 }} key={rev}>
            <ErrorAlert err={error} />
            {!settings && !error && <CircularProgress />}
            {me && settings && view === "home" && <Home settings={settings} onNavigate={(v) => setView(v as View)} />}
            {me && settings && view === "requests" && <Requests settings={settings} services={services} team={team} onOpenJob={(id) => setOpenJob(id)} />}
            {me && settings && view === "jobs" && <Jobs settings={settings} services={services} team={team} openId={openJob} onOpened={() => setOpenJob("")} />}
            {me && settings && view === "schedule" && <WeekSchedule team={team} me={me.user_id} />}
            {me && settings && view === "catalogue" && <Catalogue services={services} settings={settings} admin={me.admin} onChanged={loadBase} />}
          </Box>
        </Box>
      </Container>
    </>
  );
}
