import { useCallback, useEffect, useMemo, useState } from "react";
import { Badge, Box, CircularProgress, Container, List, ListItem, ListItemButton, ListItemContent, ListItemDecorator, Typography } from "@mui/joy";
import BadgeIcon from "@mui/icons-material/Badge";
import HomeOutlined from "@mui/icons-material/HomeOutlined";
import DashboardOutlined from "@mui/icons-material/DashboardOutlined";
import PeopleOutline from "@mui/icons-material/PeopleOutline";
import EventAvailableOutlined from "@mui/icons-material/EventAvailableOutlined";
import AccessTimeOutlined from "@mui/icons-material/AccessTimeOutlined";
import TaskAltOutlined from "@mui/icons-material/TaskAltOutlined";
import PaymentsOutlined from "@mui/icons-material/PaymentsOutlined";
import { Header } from "../../components/Header";
import { BizAssistant } from "../../components/BizAssistant";
import { onAiChanged } from "../../lib/ai/biz";
import { createHrTools, HR_ADMIN_SUGGESTIONS, HR_EMPLOYEE_SUGGESTIONS, HR_HINT } from "./ai/tools";
import type { User } from "../../api/types";
import { hr } from "./api";
import type { MyHR } from "./api";
import { ACCENT } from "./logic";
import { ErrorAlert } from "./ui";
import { deepChoice } from "../../lib/deepLink";
import { Home } from "./Home";
import { People } from "./People";
import { Leave } from "./Leave";
import { Attendance } from "./Attendance";
import { Payroll } from "./Payroll";
import { Approvals } from "./Approvals";
import { Dashboard } from "./Dashboard";

type View = "home" | "dashboard" | "people" | "leave" | "attendance" | "approvals" | "payroll";

const VIEWS: readonly View[] = ["home", "dashboard", "people", "leave", "attendance", "approvals", "payroll"];

/** startView reads the section from the path ("/hr/people") or "?view=people". */
function startView(): View {
  const seg = window.location.pathname.replace(/^\/hr\/?/, "").split("/")[0];
  if ((VIEWS as readonly string[]).includes(seg)) return seg as View;
  return deepChoice<View>("view", VIEWS, "home");
}

export default function HrApp({ user }: { user: User }) {
  const [view, setViewState] = useState<View>(startView);
  const setView = useCallback((v: View) => {
    setViewState(v);
    const q = window.location.search;
    window.history.replaceState(window.history.state, "", `/hr${v === "home" ? "" : "/" + v}${q.includes("view=") ? "" : q}`);
  }, []);
  const [me, setMe] = useState<MyHR | null>(null);
  const [error, setError] = useState("");
  const loadMe = useCallback(() => {
    hr.me().then(setMe).catch((e: Error) => setError(e.message));
  }, []);
  useEffect(loadMe, [loadMe]);

  useEffect(() => onAiChanged("hr", loadMe), [loadMe]);
  const isHR = !!me?.roles.hr;
  const aiTools = useMemo(() => createHrTools({ isHR }), [isHR]);
  const isManager = !!me?.roles.manager;
  const nav: { id: View; label: string; icon: JSX.Element; show: boolean; badge?: number }[] = [
    { id: "home", label: "My HR", icon: <HomeOutlined />, show: true },
    { id: "dashboard", label: "Dashboard", icon: <DashboardOutlined />, show: isHR || isManager },
    { id: "people", label: "People", icon: <PeopleOutline />, show: true },
    { id: "leave", label: "Leave", icon: <EventAvailableOutlined />, show: true },
    { id: "attendance", label: "Attendance", icon: <AccessTimeOutlined />, show: true },
    { id: "approvals", label: "Approvals", icon: <TaskAltOutlined />, show: isHR || isManager, badge: me?.pending_approvals },
    { id: "payroll", label: "Payroll", icon: <PaymentsOutlined />, show: isHR },
  ];
  return (
    <>
      <Header user={user} />
      <Container maxWidth="xl" sx={{ py: { xs: 2, sm: 3 }, px: { xs: 1.5, sm: 3 } }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, mb: 2 }}>
          <BadgeIcon sx={{ fontSize: 28, color: ACCENT }} />
          <Typography level="h2" sx={{ fontSize: { xs: "xl", sm: "xl3" } }}>HR</Typography>
          {me && (
            <Box sx={{ ml: "auto" }}>
              <BizAssistant app="hr" title={isHR ? "HR admin" : "My HR"} tools={aiTools} suggestions={isHR ? HR_ADMIN_SUGGESTIONS : HR_EMPLOYEE_SUGGESTIONS} systemHint={HR_HINT} />
            </Box>
          )}
        </Box>
        <Box sx={{ display: "flex", flexDirection: { xs: "column", md: "row" }, gap: { xs: 1, md: 3 } }}>
          <Box component="nav" aria-label="HR sections" sx={{ width: { md: 190 }, flexShrink: 0 }}>
            <List orientation="horizontal" sx={{ flexWrap: "wrap", "@media (min-width: 900px)": { flexDirection: "column", flexWrap: "nowrap" }, gap: 0.5 }}>
              {nav.filter((n) => n.show).map((n) => (
                <ListItem key={n.id} sx={{ width: { md: "100%" } }}>
                  <ListItemButton selected={view === n.id} onClick={() => setView(n.id)} aria-current={view === n.id ? "page" : undefined} data-testid={`hr-nav-${n.id}`} sx={{ borderRadius: "md" }}>
                    <ListItemDecorator>{n.icon}</ListItemDecorator>
                    <ListItemContent>{n.label}</ListItemContent>
                    {!!n.badge && <Badge badgeContent={n.badge} color="danger" size="sm" sx={{ ml: 1 }}><span /></Badge>}
                  </ListItemButton>
                </ListItem>
              ))}
            </List>
          </Box>
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <ErrorAlert err={error} />
            {!me && !error && <CircularProgress />}
            {me && view === "home" && <Home onNavigate={(v) => setView(v as View)} />}
            {me && view === "dashboard" && <Dashboard isHR={isHR} onNavigate={(v) => setView(v as View)} />}
            {me && view === "people" && <People isHR={isHR} />}
            {me && view === "leave" && <Leave isHR={isHR} />}
            {me && view === "attendance" && <Attendance isHR={isHR} isManager={isManager} />}
            {me && view === "approvals" && <Approvals onChanged={loadMe} />}
            {me && view === "payroll" && isHR && <Payroll />}
          </Box>
        </Box>
      </Container>
    </>
  );
}
