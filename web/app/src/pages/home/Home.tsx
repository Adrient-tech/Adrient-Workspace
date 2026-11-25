import { useMemo, useState } from "react";
import { Box, Chip, Container, Typography } from "@mui/joy";
import SearchRoundedIcon from "@mui/icons-material/SearchRounded";
import AutoAwesomeRoundedIcon from "@mui/icons-material/AutoAwesomeRounded";
import { useNavigate } from "react-router-dom";
import { Header } from "../../components/Header";
import { AppIcon } from "../../components/AppIcon";
import type { User } from "../../api/types";
import { useVisibleApps } from "../../catalog/useVisibleApps";
import { usePinnedApps } from "../../catalog/pins";
import { greeting } from "./paletteLogic";
import { QUICK_ACTIONS, runQuickAction } from "./quickActions";
import { openPalette } from "./paletteStore";
import { AppsPanel } from "./AppsPanel";
import { ProductBadge } from "../../brand/ProductBadge";
import { ApprovalsCard, MailCard, MeetingsCard, RecentFilesCard, TasksCard } from "./HomeCards";

/** Today: the signed-in home. Greeting, command bar, quick actions, cards that
 *  read live data from the apps the user has, and the launcher. Cards for apps
 *  that are switched off for the user are not rendered at all. */
export function Home({ user }: { user: User }) {
  const navigate = useNavigate();
  const { apps } = useVisibleApps();
  const { pins, toggle } = usePinnedApps();
  const [busy, setBusy] = useState<string | null>(null);
  const [failed, setFailed] = useState<string | null>(null);
  const has = useMemo(() => new Set(apps.map((a) => a.id)), [apps]);
  const now = new Date();
  const name = user.display_name || user.email.split("@")[0];
  const actions = QUICK_ACTIONS.filter((a) => has.has(a.appId));
  const showApprovals = has.has("hr") || has.has("accounting");

  async function run(id: string, appId: string) {
    setBusy(id);
    setFailed(null);
    try {
      await runQuickAction(id, navigate);
    } catch {
      setFailed(id);
      navigate(`/${appId}`);
    } finally {
      setBusy(null);
    }
  }

  return (
    <Box data-testid="home" sx={{ minHeight: "100vh", bgcolor: "background.body" }}>
      <Header user={user} />
      <Box
        sx={{
          position: "relative",
          overflow: "hidden",
          background: {
            xs: "none",
          },
        }}
      >
        {/* Soft green + saffron wash behind the greeting */}
        <Box
          aria-hidden
          sx={{
            position: "absolute",
            inset: 0,
            pointerEvents: "none",
            background:
              "radial-gradient(900px 340px at 12% -10%, rgba(18,108,90,0.16), transparent 60%), radial-gradient(700px 300px at 92% 0%, rgba(242,162,27,0.14), transparent 62%)",
            "[data-joy-color-scheme='dark'] &": {
              background:
                "radial-gradient(900px 340px at 12% -10%, rgba(47,170,140,0.16), transparent 60%), radial-gradient(700px 300px at 92% 0%, rgba(242,162,27,0.09), transparent 62%)",
            },
          }}
        />
        <Container maxWidth={false} sx={{ position: "relative", maxWidth: 1180, pt: { xs: 3, md: 5 }, pb: 3 }}>
          <Typography level="body-sm" sx={{ color: "text.tertiary", fontWeight: 600, letterSpacing: "0.04em", textTransform: "uppercase" }}>
            {now.toLocaleDateString([], { weekday: "long", month: "long", day: "numeric" })}
          </Typography>
          <Typography level="h1" component="h1" data-testid="home-greeting" sx={{ fontSize: { xs: "1.9rem", md: "2.6rem" }, mt: 0.5, mb: 2.5 }}>
            {greeting(now, name)}
          </Typography>

          <Box
            component="button"
            type="button"
            onClick={() => openPalette()}
            data-testid="home-command-bar"
            aria-label="Open command bar"
            sx={{
              all: "unset",
              boxSizing: "border-box",
              cursor: "text",
              display: "flex",
              alignItems: "center",
              gap: 1.5,
              width: "100%",
              maxWidth: 720,
              px: 2,
              py: 1.5,
              borderRadius: "999px",
              bgcolor: "background.surface",
              border: "1px solid",
              borderColor: "divider",
              boxShadow: "var(--joy-shadow-md)",
              color: "text.tertiary",
              transition: "box-shadow 160ms, border-color 160ms",
              "&:hover": { boxShadow: "var(--joy-shadow-lg)", borderColor: "primary.300" },
              "&:focus-visible": { outline: "2px solid var(--joy-palette-primary-400)", outlineOffset: 2 },
            }}
          >
            <SearchRoundedIcon />
            <Typography level="body-md" sx={{ flex: 1, color: "text.tertiary" }} noWrap>
              Search apps, files, people, or ask Adrient…
            </Typography>
            <Chip size="sm" variant="soft" color="primary" startDecorator={<AutoAwesomeRoundedIcon sx={{ fontSize: 14 }} />} sx={{ display: { xs: "none", sm: "inline-flex" } }}>
              Ask
            </Chip>
            <Typography level="body-xs" sx={{ px: 0.9, py: 0.3, border: "1px solid", borderColor: "divider", borderRadius: "sm", display: { xs: "none", sm: "block" } }}>
              Ctrl K
            </Typography>
          </Box>

          {actions.length > 0 && (
            <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", mt: 2 }} data-testid="quick-actions">
              {actions.map((a) => (
                <Box
                  key={a.id}
                  component="button"
                  type="button"
                  disabled={busy === a.id}
                  onClick={() => void run(a.id, a.appId)}
                  data-testid={`quick-${a.id}`}
                  sx={{
                    all: "unset",
                    boxSizing: "border-box",
                    display: "inline-flex",
                    alignItems: "center",
                    gap: 1,
                    pl: 0.75,
                    pr: 1.5,
                    py: 0.6,
                    borderRadius: "999px",
                    cursor: "pointer",
                    bgcolor: "background.surface",
                    border: "1px solid",
                    borderColor: "divider",
                    fontSize: "0.85rem",
                    fontWeight: 600,
                    transition: "background-color 140ms, transform 80ms",
                    "&:hover": { bgcolor: "background.level1" },
                    "&:active": { transform: "translateY(0.5px)" },
                    "&:focus-visible": { outline: "2px solid var(--joy-palette-primary-400)", outlineOffset: 2 },
                    opacity: busy === a.id ? 0.6 : 1,
                  }}
                >
                  <AppIcon iconName={a.glyph} size={24} />
                  {busy === a.id ? "Opening…" : a.label}
                </Box>
              ))}
            </Box>
          )}
          {failed && (
            <Typography level="body-xs" color="danger" sx={{ mt: 1 }}>
              Couldn't create that. Opened the app instead.
            </Typography>
          )}
        </Container>
      </Box>

      <Container maxWidth={false} sx={{ maxWidth: 1180, pb: 6 }}>
        <Box
          sx={{
            display: "grid",
            gap: 2,
            gridTemplateColumns: { xs: "1fr", md: "repeat(2, minmax(0, 1fr))", lg: "repeat(3, minmax(0, 1fr))" },
            mb: 2,
          }}
        >
          {has.has("calendar") && <MeetingsCard />}
          {has.has("mail") && <MailCard />}
          {has.has("tasks") && <TasksCard />}
          {showApprovals && <ApprovalsCard hasHR={has.has("hr")} hasAccounting={has.has("accounting")} />}
          {has.has("drive") && <RecentFilesCard />}
        </Box>
        <AppsPanel apps={apps} pins={pins} onTogglePin={toggle} />
      </Container>
      <ProductBadge />
    </Box>
  );
}
