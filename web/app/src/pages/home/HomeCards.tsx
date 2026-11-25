import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { Box, Chip, Sheet, Skeleton, Typography } from "@mui/joy";
import ArrowOutwardRoundedIcon from "@mui/icons-material/ArrowOutwardRounded";
import { Link as RouterLink } from "react-router-dom";
import { AppIcon } from "../../components/AppIcon";
import { EmptyState } from "../../components/ui/EmptyState";
import { listEvents } from "../calendar/api";
import { listThreads } from "../mail/api";
import { listTaskLists, listTasks } from "../tasks/api";
import { listRecent } from "../drive/api";
import { openFileRoute } from "../drive/editorRoutes";
import { hr } from "../hr/api";
import { listReports } from "../accounting/api";
import {
  fileUpdatedAt,
  glyphForFile,
  localDayRange,
  myTasks,
  relTime,
  todayEvents,
  topThreads,
} from "./homeLogic";
import type { Task } from "../tasks/types";

type Status<T> = { status: "loading" } | { status: "ready"; data: T } | { status: "error" };

/** useCard loads a card's data once, refreshes every two minutes and on
 *  window focus, and reports loading / ready / error (never throws). */
function useCard<T>(load: () => Promise<T>): { state: Status<T>; reload: () => void } {
  const [state, setState] = useState<Status<T>>({ status: "loading" });
  const loadRef = useRef(load);
  loadRef.current = load;
  const run = useCallback(() => {
    loadRef
      .current()
      .then((data) => setState({ status: "ready", data }))
      .catch(() => setState((s) => (s.status === "ready" ? s : { status: "error" })));
  }, []);
  useEffect(() => {
    run();
    const t = setInterval(run, 120000);
    const onFocus = () => run();
    window.addEventListener("focus", onFocus);
    return () => {
      clearInterval(t);
      window.removeEventListener("focus", onFocus);
    };
  }, [run]);
  return { state, reload: run };
}

interface CardProps {
  title: string;
  glyph: string;
  to: string;
  testId: string;
  badge?: ReactNode;
  children: ReactNode;
  /** Grid span hint on wide screens. */
  wide?: boolean;
}

export function HomeCard({ title, glyph, to, testId, badge, children, wide }: CardProps) {
  return (
    <Sheet
      variant="outlined"
      data-testid={testId}
      sx={{
        borderRadius: "lg",
        p: 2,
        display: "flex",
        flexDirection: "column",
        gap: 1.25,
        minHeight: 220,
        minWidth: 0,
        overflow: "hidden",
        bgcolor: "background.surface",
        gridColumn: { xs: "auto", lg: wide ? "span 2" : "auto" },
        animation: "ad-rise 280ms ease both",
        transition: "box-shadow 160ms, border-color 160ms",
        "&:hover": { boxShadow: "md" },
      }}
    >
      <Box sx={{ display: "flex", alignItems: "center", gap: 1.25 }}>
        <AppIcon iconName={glyph} size={32} />
        <Typography level="title-md" sx={{ flex: 1 }}>
          {title}
        </Typography>
        {badge}
        <Box
          component={RouterLink}
          to={to}
          aria-label={`Open ${title}`}
          sx={{ display: "flex", color: "text.tertiary", "&:hover": { color: "primary.plainColor" } }}
        >
          <ArrowOutwardRoundedIcon fontSize="small" />
        </Box>
      </Box>
      {children}
    </Sheet>
  );
}

function Loading() {
  return (
    <Box sx={{ display: "grid", gap: 1.25, pt: 0.5 }} aria-busy>
      {[0, 1, 2].map((i) => (
        <Box key={i} sx={{ display: "flex", gap: 1.25, alignItems: "center" }}>
          <Skeleton variant="rectangular" width={36} height={36} sx={{ borderRadius: "10px", position: "relative" }} />
          <Box sx={{ flex: 1 }}>
            <Skeleton variant="text" width="70%" sx={{ position: "relative" }} />
            <Skeleton variant="text" width="40%" sx={{ position: "relative" }} />
          </Box>
        </Box>
      ))}
    </Box>
  );
}

function Failed({ onRetry, what }: { onRetry: () => void; what: string }) {
  return (
    <EmptyState
      compact
      illustration="generic"
      title={`Couldn't load ${what}`}
      hint="Check your connection and try again."
      action={
        <Chip size="sm" variant="soft" color="primary" onClick={onRetry} slotProps={{ action: { "aria-label": "Retry" } }}>
          Retry
        </Chip>
      }
    />
  );
}

function Row({ to, glyph, title, meta, right, strong, testId }: { to: string; glyph?: string; title: string; meta?: string; right?: ReactNode; strong?: boolean; testId?: string }) {
  return (
    <Box
      component={RouterLink}
      to={to}
      data-testid={testId}
      sx={{
        display: "flex",
        alignItems: "center",
        gap: 1.25,
        px: 0.75,
        py: 0.75,
        mx: -0.75,
        minWidth: 0,
        borderRadius: "md",
        textDecoration: "none",
        color: "inherit",
        "&:hover": { bgcolor: "background.level1" },
      }}
    >
      {glyph && <AppIcon iconName={glyph} size={34} />}
      <Box sx={{ minWidth: 0, flex: 1 }}>
        <Typography level="title-sm" noWrap sx={{ fontWeight: strong ? 700 : 600 }}>
          {title}
        </Typography>
        {meta && (
          <Typography level="body-xs" noWrap sx={{ color: "text.tertiary" }}>
            {meta}
          </Typography>
        )}
      </Box>
      {right}
    </Box>
  );
}

/** Today's meetings from Calendar. */
export function MeetingsCard() {
  const { state, reload } = useCard(async () => {
    const r = localDayRange(new Date());
    return todayEvents(await listEvents(r.from, r.to), new Date());
  });
  return (
    <HomeCard title="Today's meetings" glyph="calendar" to="/calendar" testId="home-card-meetings">
      {state.status === "loading" ? (
        <Loading />
      ) : state.status === "error" ? (
        <Failed onRetry={reload} what="your calendar" />
      ) : state.data.length === 0 ? (
        <EmptyState compact illustration="calendar" title="Nothing on the calendar" hint="A clear day. Block time for focused work." />
      ) : (
        <Box sx={{ display: "grid", gap: 0.25, minWidth: 0, gridTemplateColumns: "minmax(0, 1fr)" }}>
          {state.data.map(({ event, live, next, label }) => (
            <Row
              key={event.id + event.start_at}
              to="/calendar"
              testId="home-meeting-row"
              title={event.title || "Untitled event"}
              meta={[label, event.location].filter(Boolean).join(" · ")}
              right={
                live ? (
                  <Chip size="sm" color="success" variant="soft">
                    Now
                  </Chip>
                ) : next ? (
                  <Chip size="sm" color="warning" variant="soft">
                    Next
                  </Chip>
                ) : undefined
              }
            />
          ))}
        </Box>
      )}
    </HomeCard>
  );
}

/** Unread count and top threads from Mail. */
export function MailCard() {
  const { state, reload } = useCard(async () => {
    const r = await listThreads("inbox");
    const unread = r.unread?.inbox ?? r.unread?.INBOX ?? (r.threads ?? []).filter((t) => t.any_unread).length;
    return { unread, threads: topThreads(r.threads ?? []) };
  });
  const unread = state.status === "ready" ? state.data.unread : 0;
  return (
    <HomeCard
      title="Mail"
      glyph="mail"
      to="/mail"
      testId="home-card-mail"
      badge={
        unread > 0 ? (
          <Chip size="sm" color="primary" variant="solid" data-testid="home-unread-count">
            {unread > 99 ? "99+" : unread} unread
          </Chip>
        ) : undefined
      }
    >
      {state.status === "loading" ? (
        <Loading />
      ) : state.status === "error" ? (
        <Failed onRetry={reload} what="your inbox" />
      ) : state.data.threads.length === 0 ? (
        <EmptyState compact illustration="inbox" title="Inbox zero" hint="Nothing waiting for you." />
      ) : (
        <Box sx={{ display: "grid", gap: 0.25, minWidth: 0, gridTemplateColumns: "minmax(0, 1fr)" }}>
          {state.data.threads.map((t) => (
            <Row
              key={t.thread_id}
              to={`/mail?thread=${encodeURIComponent(t.thread_id)}`}
              testId="home-mail-row"
              strong={t.any_unread}
              title={t.latest.subject || "(no subject)"}
              meta={`${t.latest.from_name || t.latest.from_addr} · ${t.latest.snippet || ""}`}
              right={
                <Typography level="body-xs" sx={{ color: t.any_unread ? "primary.plainColor" : "text.tertiary", fontWeight: t.any_unread ? 700 : 500 }}>
                  {relTime(t.latest.sent_at, new Date())}
                </Typography>
              }
            />
          ))}
        </Box>
      )}
    </HomeCard>
  );
}

/** Open tasks across the user's lists. */
export function TasksCard() {
  const { state, reload } = useCard(async () => {
    const lists = await listTaskLists();
    const all: Task[] = [];
    for (const l of lists.slice(0, 4)) {
      try {
        all.push(...(await listTasks(l.id)));
      } catch {
        /* one list failing should not blank the card */
      }
    }
    return myTasks(all, new Date());
  });
  return (
    <HomeCard title="My tasks" glyph="tasks" to="/tasks" testId="home-card-tasks">
      {state.status === "loading" ? (
        <Loading />
      ) : state.status === "error" ? (
        <Failed onRetry={reload} what="your tasks" />
      ) : state.data.length === 0 ? (
        <EmptyState compact illustration="tasks" title="All caught up" hint="No open tasks. Add one from Tasks." />
      ) : (
        <Box sx={{ display: "grid", gap: 0.25, minWidth: 0, gridTemplateColumns: "minmax(0, 1fr)" }}>
          {state.data.map(({ task, due, label }) => (
            <Row
              key={task.id}
              to="/tasks"
              testId="home-task-row"
              title={task.title}
              meta={label || "No due date"}
              right={
                due === "overdue" ? (
                  <Chip size="sm" color="danger" variant="soft">
                    Overdue
                  </Chip>
                ) : due === "today" ? (
                  <Chip size="sm" color="warning" variant="soft">
                    Today
                  </Chip>
                ) : undefined
              }
            />
          ))}
        </Box>
      )}
    </HomeCard>
  );
}

interface Approval {
  key: string;
  glyph: string;
  title: string;
  meta: string;
  to: string;
}

/** Things waiting for the user's decision: HR leave requests and submitted
 *  expense reports. Each source is optional; a source that errors (app off, no
 *  permission) is skipped instead of failing the card. */
export function ApprovalsCard({ hasHR, hasAccounting }: { hasHR: boolean; hasAccounting: boolean }) {
  const { state, reload } = useCard(async () => {
    const out: Approval[] = [];
    const [leave, reports] = await Promise.allSettled([
      hasHR ? hr.leaveRequests("team", "pending") : Promise.resolve([]),
      hasAccounting ? listReports() : Promise.resolve([]),
    ]);
    if (leave.status === "fulfilled") {
      for (const r of leave.value.filter((x) => x.can_decide)) {
        out.push({
          key: `leave:${r.id}`,
          glyph: "hr",
          title: `${r.employee_name}: ${r.type_name}`,
          meta: `${r.days} day${r.days === 1 ? "" : "s"} from ${r.from_date}`,
          to: "/hr",
        });
      }
    }
    if (reports.status === "fulfilled") {
      for (const r of reports.value.filter((x) => x.status === "submitted")) {
        out.push({
          key: `expense:${r.id}`,
          glyph: "accounting",
          title: r.title || "Expense report",
          meta: `${r.submitter_name} · ${r.count} item${r.count === 1 ? "" : "s"}`,
          to: "/accounting",
        });
      }
    }
    if (leave.status === "rejected" && reports.status === "rejected") throw new Error("no sources");
    return out;
  });
  const count = state.status === "ready" ? state.data.length : 0;
  return (
    <HomeCard
      title="Approvals waiting"
      glyph="hr"
      to={hasHR ? "/hr" : "/accounting"}
      testId="home-card-approvals"
      badge={
        count > 0 ? (
          <Chip size="sm" color="warning" variant="solid">
            {count}
          </Chip>
        ) : undefined
      }
    >
      {state.status === "loading" ? (
        <Loading />
      ) : state.status === "error" ? (
        <Failed onRetry={reload} what="approvals" />
      ) : state.data.length === 0 ? (
        <EmptyState compact illustration="approvals" title="Nothing to approve" hint="Leave and expense requests that need you show up here." />
      ) : (
        <Box sx={{ display: "grid", gap: 0.25, minWidth: 0, gridTemplateColumns: "minmax(0, 1fr)" }}>
          {state.data.slice(0, 5).map((a) => (
            <Row key={a.key} to={a.to} glyph={a.glyph} testId="home-approval-row" title={a.title} meta={a.meta} />
          ))}
        </Box>
      )}
    </HomeCard>
  );
}

/** Recently changed files from Drive. */
export function RecentFilesCard() {
  const { state, reload } = useCard(async () => (await listRecent()).filter((f) => !f.trashed).slice(0, 6));
  return (
    <HomeCard title="Recent files" glyph="drive" to="/drive" testId="home-card-files" wide>
      {state.status === "loading" ? (
        <Loading />
      ) : state.status === "error" ? (
        <Failed onRetry={reload} what="your files" />
      ) : state.data.length === 0 ? (
        <EmptyState compact illustration="files" title="No recent files" hint="Files you open or edit will appear here." />
      ) : (
        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1fr 1fr" }, columnGap: 3, rowGap: 0.25 }}>
          {state.data.map((f) => (
            <Row
              key={f.id}
              to={openFileRoute(f)}
              testId="home-file-row"
              glyph={glyphForFile(f)}
              title={f.name}
              meta={relTime(fileUpdatedAt(f), new Date())}
            />
          ))}
        </Box>
      )}
    </HomeCard>
  );
}
