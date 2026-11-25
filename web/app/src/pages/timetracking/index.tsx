import { useCallback, useEffect, useMemo, useState } from "react";
import { Box, CircularProgress, Container, List, ListItem, ListItemButton, ListItemContent, ListItemDecorator, Typography } from "@mui/joy";
import TimerOutlined from "@mui/icons-material/TimerOutlined";
import TodayOutlined from "@mui/icons-material/TodayOutlined";
import ViewWeekOutlined from "@mui/icons-material/ViewWeekOutlined";
import BarChartOutlined from "@mui/icons-material/BarChartOutlined";
import ReceiptLongOutlined from "@mui/icons-material/ReceiptLongOutlined";
import { Header } from "../../components/Header";
import { BizAssistant } from "../../components/BizAssistant";
import { onAiChanged } from "../../lib/ai/biz";
import { deepChoice } from "../../lib/deepLink";
import type { User } from "../../api/types";
import { ErrorAlert } from "../hr/ui";
import { createTimeTools, TT_HINT, TT_SUGGESTIONS } from "./ai/tools";
import { tt } from "./api";
import type { Me } from "./api";
import { Today } from "./Today";
import { Week } from "./Week";
import { Reports } from "./Reports";
import { Billing } from "./Billing";

type View = "today" | "week" | "reports" | "billing";
const VIEWS: readonly View[] = ["today", "week", "reports", "billing"];
const ACCENT = "#0E7490";

function startView(): View {
  const seg = window.location.pathname.replace(/^\/timetracking\/?/, "").split("/")[0];
  if ((VIEWS as readonly string[]).includes(seg)) return seg as View;
  return deepChoice<View>("view", VIEWS, "today");
}

export default function TimeTrackingApp({ user }: { user: User }) {
  const [view, setViewState] = useState<View>(startView);
  const setView = useCallback((v: View) => {
    setViewState(v);
    window.history.replaceState(window.history.state, "", `/timetracking${v === "today" ? "" : "/" + v}`);
  }, []);
  const [me, setMe] = useState<Me | null>(null);
  const [error, setError] = useState("");
  const [rev, setRev] = useState(0);
  useEffect(() => { tt.me().then(setMe).catch((e: Error) => setError(e.message)); }, []);
  useEffect(() => onAiChanged("timetracking", () => setRev((r) => r + 1)), []);
  const tools = useMemo(() => createTimeTools({ admin: !!me?.admin }), [me?.admin]);
  const nav: { id: View; label: string; icon: JSX.Element; show: boolean }[] = [
    { id: "today", label: "Timer", icon: <TodayOutlined />, show: true },
    { id: "week", label: "Weekly timesheet", icon: <ViewWeekOutlined />, show: true },
    { id: "reports", label: "Reports", icon: <BarChartOutlined />, show: true },
    { id: "billing", label: "Billing and setup", icon: <ReceiptLongOutlined />, show: !!me?.admin },
  ];
  return (
    <>
      <Header user={user} />
      <Container maxWidth="xl" sx={{ py: { xs: 2, sm: 3 }, px: { xs: 1.5, sm: 3 } }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, mb: 2 }}>
          <TimerOutlined sx={{ fontSize: 28, color: ACCENT }} />
          <Typography level="h2" sx={{ fontSize: { xs: "xl", sm: "xl3" } }}>Time tracking</Typography>
          {me && <Box sx={{ ml: "auto" }}><BizAssistant app="timetracking" title="Time tracking" tools={tools} suggestions={TT_SUGGESTIONS} systemHint={TT_HINT} /></Box>}
        </Box>
        <Box sx={{ display: "flex", flexDirection: { xs: "column", md: "row" }, gap: { xs: 1, md: 3 } }}>
          <Box component="nav" aria-label="Time tracking sections" sx={{ width: { md: 210 }, flexShrink: 0 }}>
            <List orientation="horizontal" sx={{ flexWrap: "wrap", "@media (min-width: 900px)": { flexDirection: "column", flexWrap: "nowrap" }, gap: 0.5 }}>
              {nav.filter((n) => n.show).map((n) => (
                <ListItem key={n.id} sx={{ width: { md: "100%" } }}>
                  <ListItemButton selected={view === n.id} onClick={() => setView(n.id)} aria-current={view === n.id ? "page" : undefined} data-testid={`tt-nav-${n.id}`} sx={{ borderRadius: "md" }}>
                    <ListItemDecorator>{n.icon}</ListItemDecorator><ListItemContent>{n.label}</ListItemContent>
                  </ListItemButton>
                </ListItem>
              ))}
            </List>
          </Box>
          <Box sx={{ flex: 1, minWidth: 0 }} key={rev}>
            <ErrorAlert err={error} />
            {!me && !error && <CircularProgress />}
            {me && view === "today" && <Today me={me} onChanged={() => undefined} />}
            {me && view === "week" && <Week />}
            {me && view === "reports" && <Reports me={me} />}
            {me && view === "billing" && me.admin && <Billing />}
          </Box>
        </Box>
      </Container>
    </>
  );
}
