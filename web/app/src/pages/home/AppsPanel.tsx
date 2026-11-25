import { useMemo, useState } from "react";
import { Box, Button, Input, Sheet, Tab, TabList, Tabs, Typography } from "@mui/joy";
import SearchRoundedIcon from "@mui/icons-material/SearchRounded";
import ArrowForwardRoundedIcon from "@mui/icons-material/ArrowForwardRounded";
import { Link as RouterLink } from "react-router-dom";
import { TileGrid } from "../../components/TileGrid";
import { EmptyState } from "../../components/ui/EmptyState";
import type { AppTile } from "../../catalog/apps";
import { GROUP_LABEL, GROUP_ORDER, groupApps, groupOf, type AppGroup } from "../../catalog/groups";
import { visiblePins } from "../../catalog/pins";

type TabId = "pinned" | AppGroup;

interface Props {
  apps: AppTile[];
  pins: string[];
  onTogglePin: (id: string) => void;
}

/** AppsPanel is the launcher on the home screen: a filter box, tabs for
 *  Pinned and each group (Office, Communicate, Business, Admin, More) and a
 *  link to the full grid. Pin a tile with its star. */
export function AppsPanel({ apps, pins, onTogglePin }: Props) {
  const pinned = useMemo(() => visiblePins(pins, apps), [pins, apps]);
  const [tab, setTab] = useState<TabId>("pinned");
  const [query, setQuery] = useState("");
  const groups = useMemo(() => groupApps(apps), [apps]);

  const q = query.trim().toLowerCase();
  const shown = useMemo(() => {
    if (q) return apps.filter((a) => a.name.toLowerCase().includes(q) || a.blurb.toLowerCase().includes(q));
    if (tab === "pinned") return pinned;
    return groups.find((g) => g.group === tab)?.items ?? [];
  }, [q, tab, apps, pinned, groups]);

  return (
    <Sheet variant="outlined" data-testid="apps-panel" sx={{ borderRadius: "lg", p: 2, bgcolor: "background.surface" }}>
      <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, flexWrap: "wrap", mb: 1.5 }}>
        <Typography level="title-md" sx={{ mr: "auto" }}>
          Your apps
        </Typography>
        <Input
          size="sm"
          placeholder="Filter apps…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          startDecorator={<SearchRoundedIcon fontSize="small" />}
          slotProps={{ input: { "aria-label": "Filter apps" } }}
          sx={{ width: { xs: "100%", sm: 220 }, "--Input-radius": "999px" }}
        />
        <Button component={RouterLink} to="/apps" size="sm" variant="soft" endDecorator={<ArrowForwardRoundedIcon fontSize="small" />} data-testid="all-apps-link">
          All apps
        </Button>
      </Box>
      {!q && (
        <Tabs value={tab} onChange={(_, v) => setTab(v as TabId)} sx={{ bgcolor: "transparent", mb: 1 }}>
          <TabList sx={{ overflowX: "auto", flexWrap: "nowrap", width: "fit-content", maxWidth: "100%" }}>
            <Tab value="pinned" data-testid="apps-tab-pinned" sx={{ flex: "none" }}>
              Pinned
            </Tab>
            {GROUP_ORDER.filter((g) => groups.some((x) => x.group === g)).map((g) => (
              <Tab key={g} value={g} data-testid={`apps-tab-${g}`} sx={{ flex: "none" }}>
                {GROUP_LABEL[g]}
              </Tab>
            ))}
          </TabList>
        </Tabs>
      )}
      {shown.length === 0 ? (
        <EmptyState
          compact
          illustration={q ? "search" : "generic"}
          title={q ? "No app matches" : "Nothing pinned yet"}
          hint={q ? "Try another name, or open All apps." : "Open a group and tap the star on an app to pin it to your dock."}
        />
      ) : (
        <TileGrid apps={shown} pinned={pins} onTogglePin={onTogglePin} />
      )}
      {q && shown.length > 0 && (
        <Typography level="body-xs" sx={{ mt: 1, color: "text.tertiary" }}>
          {shown.map((a) => GROUP_LABEL[groupOf(a.id)]).filter((v, i, arr) => arr.indexOf(v) === i).join(" · ")}
        </Typography>
      )}
    </Sheet>
  );
}
