import { useEffect, useMemo, useState } from "react";
import { Box, CircularProgress, Container, List, ListItem, ListItemButton, ListItemContent, ListItemDecorator, Typography } from "@mui/joy";
import GroupAddOutlined from "@mui/icons-material/GroupAddOutlined";
import WorkOutline from "@mui/icons-material/WorkOutline";
import BookmarksOutlined from "@mui/icons-material/BookmarksOutlined";
import InsightsOutlined from "@mui/icons-material/InsightsOutlined";
import ChecklistOutlined from "@mui/icons-material/ChecklistOutlined";
import TrendingUpOutlined from "@mui/icons-material/TrendingUpOutlined";
import SchoolOutlined from "@mui/icons-material/SchoolOutlined";
import CampaignOutlined from "@mui/icons-material/CampaignOutlined";
import SettingsOutlined from "@mui/icons-material/SettingsOutlined";
import { Header } from "../../components/Header";
import { BizAssistant } from "../../components/BizAssistant";
import { createTalentTools, TALENT_HINT, TALENT_SUGGESTIONS } from "./ai/tools";
import type { User } from "../../api/types";
import * as api from "./api";
import { ACCENT } from "./logic";
import type { Ctx } from "./ui";
import { ErrorAlert } from "./ui";
import { Jobs, PoolView, ReportsView } from "./Jobs";
import { PipelineBoard } from "./Pipeline";
import { OnboardingView } from "./Onboarding";
import { PerformanceView } from "./Performance";
import { FeedView, LearningView, SettingsView } from "./Learning";
import { deepChoice } from "../../lib/deepLink";

type View = "jobs" | "pool" | "reports" | "onboarding" | "performance" | "learning" | "feed" | "settings";

const NAV: { id: View; label: string; icon: JSX.Element; hr?: boolean }[] = [
  { id: "jobs", label: "Recruitment", icon: <WorkOutline /> },
  { id: "pool", label: "Talent pool", icon: <BookmarksOutlined />, hr: true },
  { id: "reports", label: "Hiring reports", icon: <InsightsOutlined />, hr: true },
  { id: "onboarding", label: "Onboarding", icon: <ChecklistOutlined /> },
  { id: "performance", label: "Performance", icon: <TrendingUpOutlined /> },
  { id: "learning", label: "Learning and policies", icon: <SchoolOutlined /> },
  { id: "feed", label: "Company feed", icon: <CampaignOutlined /> },
  { id: "settings", label: "Settings", icon: <SettingsOutlined />, hr: true },
];

export default function TalentApp({ user }: { user: User }) {
  const [view, setView] = useState<View>(() => deepChoice<View>("view", ["jobs", "pool", "reports", "onboarding", "performance", "learning", "feed", "settings"], "jobs"));
  const [jobId, setJobId] = useState("");
  const [ctx, setCtx] = useState<Ctx | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    Promise.all([api.getMe(), api.directory()])
      .then(([me, people]) => setCtx({ me, people, reload: () => undefined }))
      .catch((e: Error) => setError(e.message));
  }, []);

  const aiTools = useMemo(() => createTalentTools(), []);
  const nav = NAV.filter((n) => !n.hr || ctx?.me.is_hr);
  return (
    <>
      <Header user={user} />
      <Container maxWidth="xl" sx={{ py: { xs: 2, sm: 3 }, px: { xs: 1.5, sm: 3 } }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, mb: 2 }}>
          <GroupAddOutlined sx={{ fontSize: 28, color: ACCENT }} />
          <Typography level="h2" sx={{ fontSize: { xs: "xl", sm: "xl3" } }}>Talent</Typography>
          {ctx?.me.is_hr && (
            <Box sx={{ ml: "auto" }}>
              <BizAssistant app="talent" title="Recruiting" tools={aiTools} suggestions={TALENT_SUGGESTIONS} systemHint={TALENT_HINT} />
            </Box>
          )}
        </Box>
        <Box sx={{ display: "flex", flexDirection: { xs: "column", md: "row" }, gap: { xs: 1, md: 3 } }}>
          <Box component="nav" aria-label="Talent sections" sx={{ width: { md: 210 }, flexShrink: 0 }}>
            <List orientation="horizontal" sx={{ flexWrap: "wrap", "@media (min-width: 900px)": { flexDirection: "column", flexWrap: "nowrap" }, gap: 0.5 }}>
              {nav.map((n) => (
                <ListItem key={n.id} sx={{ width: { md: "100%" } }}>
                  <ListItemButton selected={view === n.id} onClick={() => { setView(n.id); setJobId(""); }} data-testid={`talent-nav-${n.id}`} sx={{ borderRadius: "md" }}>
                    <ListItemDecorator>{n.icon}</ListItemDecorator>
                    <ListItemContent>{n.label}</ListItemContent>
                  </ListItemButton>
                </ListItem>
              ))}
            </List>
          </Box>
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <ErrorAlert err={error} />
            {!ctx && !error && <CircularProgress />}
            {ctx && view === "jobs" && !jobId && <Jobs ctx={ctx} onOpen={setJobId} />}
            {ctx && view === "jobs" && jobId && <PipelineBoard ctx={ctx} jobId={jobId} onBack={() => setJobId("")} />}
            {ctx && view === "pool" && <PoolView ctx={ctx} />}
            {ctx && view === "reports" && <ReportsView ctx={ctx} />}
            {ctx && view === "onboarding" && <OnboardingView ctx={ctx} />}
            {ctx && view === "performance" && <PerformanceView ctx={ctx} />}
            {ctx && view === "learning" && <LearningView ctx={ctx} />}
            {ctx && view === "feed" && <FeedView ctx={ctx} />}
            {ctx && view === "settings" && <SettingsView ctx={ctx} />}
          </Box>
        </Box>
      </Container>
    </>
  );
}
