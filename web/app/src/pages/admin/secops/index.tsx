// Admin > Security operations: sign-in risk alerts and triage, per-person security
// actions (password, second step, sessions, account), lists, delegation, reports.
import { useCallback, useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { Alert, Box, Chip, Sheet, Tab, TabList, TabPanel, Tabs, Typography } from "@mui/joy";
import { A, iam, type Me, type Summary } from "./api";
import { AlertsTab } from "./AlertsTab";
import { UserDrawer } from "./UserDrawer";
import { ActionLogTab, ApprovalsTab, EventsTab, ListsTab, PeopleTab, ReportsTab, RolesTab, SettingsTab } from "./tabs";
import { can } from "./logic";

export function SecOpsSection() {
  const [params, setParams] = useSearchParams();
  const [me, setMe] = useState<Me | null>(null);
  const [err, setErr] = useState("");
  const [tab, setTab] = useState(params.get("tab") ?? (params.get("user") ? "people" : "alerts"));
  const [user, setUser] = useState<string | null>(params.get("user"));
  const [summary, setSummary] = useState<Summary | null>(null);
  const [key, setKey] = useState(0);

  useEffect(() => {
    iam<Me>(`${A}/me`)
      .then((m) => {
        setMe(m);
        setSummary(m.summary);
      })
      .catch((e) => setErr((e as Error).message));
  }, [key]);
  const openUser = useCallback(
    (id: string | null) => {
      setUser(id);
      const p = new URLSearchParams(params);
      if (id) p.set("user", id);
      else p.delete("user");
      setParams(p, { replace: true });
    },
    [params, setParams],
  );

  if (err)
    return (
      <Alert color="warning" data-testid="secops-denied">
        {err}
      </Alert>
    );
  if (!me) return null;
  const caps = me.capabilities;
  const stat = (label: string, n: number, color: "danger" | "warning" | "neutral") => (
    <Sheet variant="outlined" sx={{ p: 1.25, borderRadius: "md", minWidth: 130 }}>
      <Typography level="h3" color={n > 0 ? color : "neutral"}>
        {n}
      </Typography>
      <Typography level="body-xs">{label}</Typography>
    </Sheet>
  );
  return (
    <Box data-testid="secops">
      <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
        <Typography level="h4">Security operations</Typography>
        {me.roles.map((r) => (
          <Chip key={r} size="sm" variant="soft">
            {r.replace("_", " ")}
          </Chip>
        ))}
        {!me.idp_configured && (
          <Chip size="sm" color="warning">
            Identity provider not connected
          </Chip>
        )}
      </Box>
      <Typography level="body-sm" sx={{ opacity: 0.7, mb: 1.5, maxWidth: 820 }}>
        Watch sign-ins for risk, investigate alerts, and act on a person's password, second step, sessions and account. Every action asks for a reason, is audited and tells the person.
      </Typography>
      {summary && can(caps, "view_alerts") && (
        <Box sx={{ display: "flex", gap: 1.5, mb: 2, flexWrap: "wrap" }} data-testid="secops-summary">
          {stat("Open alerts", summary.Open, "danger")}
          {stat("Investigating", summary.Investigating, "warning")}
          {stat("High or critical", summary.High + summary.Critical, "danger")}
          {stat("Past SLA", summary.Breached, "danger")}
          {stat("Raised in 24 hours", summary.Today, "neutral")}
        </Box>
      )}
      <Tabs value={tab} onChange={(_, v) => setTab(String(v))} sx={{ bgcolor: "transparent" }}>
        <TabList sx={{ flexWrap: "wrap" }}>
          {can(caps, "view_alerts") && (
            <Tab value="alerts" data-testid="tab-alerts">
              Alerts
            </Tab>
          )}
          <Tab value="people" data-testid="tab-people">
            People
          </Tab>
          {can(caps, "view_events") && (
            <Tab value="events" data-testid="tab-events">
              Sign-ins
            </Tab>
          )}
          {can(caps, "view_alerts") && (
            <Tab value="lists" data-testid="tab-lists">
              Allow and block
            </Tab>
          )}
          {can(caps, "view_alerts") && (
            <Tab value="settings" data-testid="tab-settings">
              Detection and policy
            </Tab>
          )}
          <Tab value="roles" data-testid="tab-roles">
            Delegation
          </Tab>
          {(can(caps, "approve") || can(caps, "view_action_log")) && (
            <Tab value="approvals" data-testid="tab-approvals">
              Approvals
            </Tab>
          )}
          {can(caps, "reports") && (
            <Tab value="reports" data-testid="tab-reports">
              Reports
            </Tab>
          )}
          {can(caps, "view_action_log") && (
            <Tab value="log" data-testid="tab-log">
              Action log
            </Tab>
          )}
        </TabList>
        <TabPanel value="alerts" sx={{ px: 0 }}>
          <AlertsTab
            me={me}
            openId={params.get("alert")}
            onOpenUser={(id) => {
              setTab("people");
              openUser(id);
            }}
            onSummary={setSummary}
          />
        </TabPanel>
        <TabPanel value="people" sx={{ px: 0 }}>
          <PeopleTab onOpen={(id) => openUser(id)} refreshKey={key} />
        </TabPanel>
        <TabPanel value="events" sx={{ px: 0 }}>
          <EventsTab onOpenUser={(id) => openUser(id)} />
        </TabPanel>
        <TabPanel value="lists" sx={{ px: 0 }}>
          <ListsTab me={me} />
        </TabPanel>
        <TabPanel value="settings" sx={{ px: 0 }}>
          <SettingsTab me={me} />
        </TabPanel>
        <TabPanel value="roles" sx={{ px: 0 }}>
          <RolesTab me={me} />
        </TabPanel>
        <TabPanel value="approvals" sx={{ px: 0 }}>
          <ApprovalsTab me={me} onChanged={() => setKey((k) => k + 1)} />
        </TabPanel>
        <TabPanel value="reports" sx={{ px: 0 }}>
          <ReportsTab />
        </TabPanel>
        <TabPanel value="log" sx={{ px: 0 }}>
          <ActionLogTab />
        </TabPanel>
      </Tabs>
      <UserDrawer userId={user} onClose={() => openUser(null)} onChanged={() => setKey((k) => k + 1)} />
    </Box>
  );
}
