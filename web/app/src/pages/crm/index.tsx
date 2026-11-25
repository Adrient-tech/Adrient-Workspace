import { deepParam } from "../../lib/deepLink";
import { useCallback, useEffect, useMemo, useState } from "react";
import { Badge, Box, Button, CircularProgress, Container, List, ListItem, ListItemButton, ListItemContent, ListItemDecorator, Typography } from "@mui/joy";
import HandshakeIcon from "@mui/icons-material/Handshake";
import DashboardOutlined from "@mui/icons-material/DashboardOutlined";
import ViewKanbanOutlined from "@mui/icons-material/ViewKanbanOutlined";
import PeopleOutline from "@mui/icons-material/PeopleOutline";
import BusinessOutlined from "@mui/icons-material/BusinessOutlined";
import EventAvailableOutlined from "@mui/icons-material/EventAvailableOutlined";
import HistoryIcon from "@mui/icons-material/History";
import InsightsOutlined from "@mui/icons-material/InsightsOutlined";
import SettingsOutlined from "@mui/icons-material/SettingsOutlined";
import AssessmentOutlined from "@mui/icons-material/AssessmentOutlined";
import BoltOutlined from "@mui/icons-material/BoltOutlined";
import ForwardToInboxOutlined from "@mui/icons-material/ForwardToInboxOutlined";
import GroupsOutlined from "@mui/icons-material/GroupsOutlined";
import Inventory2Outlined from "@mui/icons-material/Inventory2Outlined";
import StorageOutlined from "@mui/icons-material/StorageOutlined";
import SearchIcon from "@mui/icons-material/Search";
import { Header } from "../../components/Header";
import { BizAssistant } from "../../components/BizAssistant";
import { onAiChanged, withAccess } from "../../lib/ai/biz";
import { usePermissions } from "../../lib/permissions";
import { createCrmTools, CRM_HINT, CRM_SUGGESTIONS } from "./ai/tools";
import type { User } from "../../api/types";
import * as api from "./api";
import * as api2 from "./api2";
import { Automation } from "./Automation";
import { Capture } from "./Capture";
import { CommandPalette } from "./CommandPalette";
import { DataTools } from "./DataTools";
import { Products } from "./Products";
import { Reports } from "./Reports";
import { Teams } from "./Teams";
import { ACCENT, groupFollowups } from "./logic";
import type { Data } from "./ui";
import { ErrorAlert } from "./ui";
import { Dashboard, Forecast } from "./Insights";
import { Pipeline } from "./Pipeline";
import { Companies, Leads } from "./People";
import { ActivityLog, FollowUps } from "./Agenda";
import { SettingsView } from "./SettingsView";

type View = "dashboard" | "pipeline" | "leads" | "companies" | "followups" | "activity" | "forecast" | "reports" | "automation" | "capture" | "teams" | "products" | "data" | "settings";

const NAV: { id: View; label: string; icon: JSX.Element }[] = [
  { id: "dashboard", label: "Dashboard", icon: <DashboardOutlined /> },
  { id: "pipeline", label: "Pipeline", icon: <ViewKanbanOutlined /> },
  { id: "leads", label: "Leads", icon: <PeopleOutline /> },
  { id: "companies", label: "Companies", icon: <BusinessOutlined /> },
  { id: "followups", label: "Follow-ups", icon: <EventAvailableOutlined /> },
  { id: "activity", label: "Activity", icon: <HistoryIcon /> },
  { id: "forecast", label: "Forecast", icon: <InsightsOutlined /> },
  { id: "reports", label: "Reports", icon: <AssessmentOutlined /> },
  { id: "automation", label: "Automation", icon: <BoltOutlined /> },
  { id: "capture", label: "Lead capture", icon: <ForwardToInboxOutlined /> },
  { id: "teams", label: "Teams", icon: <GroupsOutlined /> },
  { id: "products", label: "Products", icon: <Inventory2Outlined /> },
  { id: "data", label: "Data tools", icon: <StorageOutlined /> },
  { id: "settings", label: "Settings", icon: <SettingsOutlined /> },
];

type Loaded = Omit<Data, "reload">;

export default function CrmApp({ user }: { user: User }) {
  const [view, setView] = useState<View>(() => (deepParam("deal") ? "pipeline" : "dashboard"));
  const [data, setData] = useState<Loaded | null>(null);
  const [error, setError] = useState("");
  const [version, setVersion] = useState(0);
  const [openDeal, setOpenDeal] = useState(() => deepParam("deal"));
  const [palette, setPalette] = useState(false);
  const [focus, setFocus] = useState<{ kind: string; id: string } | null>(null);

  // Ctrl-K / Cmd-K opens the command search.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPalette(true);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  useEffect(() => {
    let live = true;
    Promise.all([
      api.getSettings(),
      api.listCompanies(),
      api.listPeople(),
      api.listDeals(),
      api.listHistory(),
      api.listFollowups(),
      api.listActivities(),
      api.listContacts(),
      api2.getFields(),
      api2.getMe(),
      api2.getUsers(),
    ])
      .then(([settings, companies, people, deals, history, followups, activities, contacts, fields, me, users]) => {
        if (live) setData({ settings, companies, people, deals, history, followups, activities, contacts, fields, me, users });
      })
      .catch((e: Error) => live && setError(e.message));
    return () => {
      live = false;
    };
  }, [version]);

  const reload = useCallback(() => setVersion((v) => v + 1), []);
  const { can } = usePermissions();
  const canWrite = can("crm:deal:write");
  const aiTools = useMemo(() => withAccess(createCrmTools(), canWrite), [canWrite]);
  useEffect(() => onAiChanged("crm", reload), [reload]);
  const aiContext = useCallback(() => ({ label: `CRM · ${view}${openDeal ? " · deal open" : ""}`, text: openDeal ? `The user has the deal with id ${openDeal} open.` : undefined }), [view, openDeal]);
  const full: Data | null = useMemo(() => (data ? { ...data, reload } : null), [data, reload]);
  const due = useMemo(() => {
    if (!data) return 0;
    const g = groupFollowups(data.followups);
    return g.overdue.length + g.today.length;
  }, [data]);

  const go = (v: View, deal = "") => {
    setOpenDeal(deal);
    setFocus(null);
    setView(v);
  };
  const openHit = (h: api2.SearchHit) => {
    if (h.kind === "deal") return go("pipeline", h.id);
    if (h.kind === "activity") return go("activity");
    setOpenDeal("");
    setFocus({ kind: h.kind, id: h.id });
    setView(h.kind === "company" ? "companies" : "leads");
  };

  return (
    <>
      <Header user={user} />
      <Container maxWidth="xl" sx={{ py: { xs: 2, sm: 3 }, px: { xs: 1.5, sm: 3 } }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, mb: 2 }}>
          <HandshakeIcon sx={{ fontSize: 28, color: ACCENT }} />
          <Typography level="h2" sx={{ fontSize: { xs: "xl", sm: "xl3" } }}>
            CRM
          </Typography>
          <Box sx={{ ml: "auto" }}>
            <BizAssistant app="crm" title="CRM" tools={aiTools} suggestions={CRM_SUGGESTIONS} systemHint={CRM_HINT} getContext={aiContext} />
          </Box>
        </Box>
        <Box sx={{ display: "flex", flexDirection: { xs: "column", md: "row" }, gap: { xs: 1, md: 3 } }}>
          <Box component="nav" aria-label="CRM sections" sx={{ width: { md: 190 }, flexShrink: 0 }}>
            <List orientation="horizontal" sx={{ flexWrap: "wrap", "@media (min-width: 900px)": { flexDirection: "column", flexWrap: "nowrap" }, gap: 0.5 }}>
              {NAV.map((n) => (
                <ListItem key={n.id} sx={{ width: { md: "100%" } }}>
                  <ListItemButton selected={view === n.id} onClick={() => go(n.id)} aria-current={view === n.id ? "page" : undefined} data-testid={`crm-nav-${n.id}`} sx={{ borderRadius: "md" }}>
                    <ListItemDecorator>{n.icon}</ListItemDecorator>
                    <ListItemContent>{n.label}</ListItemContent>
                    {n.id === "followups" && due > 0 && (
                      <Badge badgeContent={due} color="danger" size="sm" sx={{ ml: 1 }} data-testid="crm-due-badge">
                        <span />
                      </Badge>
                    )}
                  </ListItemButton>
                </ListItem>
              ))}
            </List>
            <Button size="sm" variant="outlined" startDecorator={<SearchIcon />} onClick={() => setPalette(true)} data-testid="crm-open-palette" sx={{ mt: 1, display: { xs: "none", md: "inline-flex" } }}>
              Search (Ctrl K)
            </Button>
          </Box>
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <ErrorAlert err={error} />
            {!full && !error && <CircularProgress />}
            {full && view === "dashboard" && <Dashboard data={full} onOpenDeal={(id) => go("pipeline", id)} />}
            {full && view === "pipeline" && <Pipeline data={full} initialDeal={openDeal} />}
            {full && view === "leads" && <Leads data={full} focusId={focus?.kind === "lead" ? focus.id : ""} />}
            {full && view === "companies" && <Companies data={full} focusId={focus?.kind === "company" ? focus.id : ""} />}
            {full && view === "followups" && <FollowUps data={full} onOpenDeal={(id) => go("pipeline", id)} />}
            {full && view === "activity" && <ActivityLog data={full} />}
            {full && view === "forecast" && <Forecast data={full} />}
            {full && view === "reports" && <Reports data={full} />}
            {full && view === "automation" && <Automation data={full} />}
            {full && view === "capture" && <Capture data={full} />}
            {full && view === "teams" && <Teams data={full} />}
            {full && view === "products" && <Products data={full} />}
            {full && view === "data" && <DataTools data={full} />}
            {full && view === "settings" && <SettingsView data={full} />}
          </Box>
        </Box>
      </Container>
      {palette && <CommandPalette commands={NAV.map((n) => ({ id: n.id, label: n.label, run: () => go(n.id) }))} onOpenHit={openHit} onClose={() => setPalette(false)} />}
    </>
  );
}
