import { useEffect, useMemo, useRef, useState } from "react";
import { Box, Container, Input, Button, Typography } from "@mui/joy";
import SearchIcon from "@mui/icons-material/Search";
import PersonAddIcon from "@mui/icons-material/PersonAddAlt1";
import { Header } from "../components/Header";
import { ProductBadge } from "../brand/ProductBadge";
import { TileGrid } from "../components/TileGrid";
import { PageHeader } from "../components/ui/PageHeader";
import { AppIcon } from "../components/AppIcon";
import { EmptyState } from "../components/ui/EmptyState";
import { GROUP_LABEL, groupApps } from "../catalog/groups";
import { useVisibleApps } from "../catalog/useVisibleApps";
import { usePinnedApps } from "../catalog/pins";
import type { User } from "../api/types";
import { AddUserDialog } from "./admin/AddUserDialog";

interface DashboardProps {
  user: User;
}

/** Dashboard is the full app grid (route /apps): every app the user can open,
 *  grouped Office / Communicate / Business / Admin / More, with search and
 *  pinning. The Today home lives at "/" (pages/home/Home.tsx). */
export function Dashboard({ user }: DashboardProps) {
  // Org service switches, per-user app policies and RBAC are resolved inside
  // useVisibleApps (fails open for built-ins; the server enforces access).
  const { apps: visible, admin } = useVisibleApps();
  const { pins, toggle } = usePinnedApps();
  const [query, setQuery] = useState("");
  const [addUserOpen, setAddUserOpen] = useState(false);
  // Focus the search on load so you can start typing apps immediately: desktop
  // only (auto-focusing on touch devices pops the soft keyboard).
  const searchRef = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (window.matchMedia && window.matchMedia("(pointer: fine)").matches) {
      searchRef.current?.focus();
    }
  }, []);

  const sections = useMemo(() => {
    const q = query.trim().toLowerCase();
    const list = q ? visible.filter((a) => a.name.toLowerCase().includes(q) || a.blurb.toLowerCase().includes(q)) : visible;
    return groupApps(list);
  }, [visible, query]);

  return (
    <>
      <Header user={user} />
      <Container maxWidth={false} sx={{ maxWidth: 1180, py: { xs: 2.5, md: 4 } }}>
        <PageHeader
          title="All apps"
          subtitle="Everything in your workspace. Tap the star to pin an app to your dock."
          icon={<AppIcon iconName="workspace" size={44} variant="solid" />}
          actions={
            admin.canAddUser ? (
              <Button size="sm" variant="outlined" startDecorator={<PersonAddIcon />} onClick={() => setAddUserOpen(true)} data-testid="dashboard-add-user">
                Add user
              </Button>
            ) : undefined
          }
        />
        <Box sx={{ mb: 3 }}>
          <Input
            startDecorator={<SearchIcon />}
            placeholder="Search apps…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            slotProps={{ input: { ref: searchRef, "aria-label": "Search apps" } }}
            sx={{ width: "100%", maxWidth: 420, "--Input-radius": "999px" }}
          />
        </Box>
        {sections.length === 0 && <EmptyState illustration="search" title="No app matches" hint="Try a different name." />}
        {sections.map(({ group, items }) => (
          <Box key={group} component="section" data-testid={`apps-group-${group}`} sx={{ mb: 3.5 }}>
            <Typography level="body-xs" sx={{ mb: 1, fontWeight: 700, letterSpacing: "0.08em", textTransform: "uppercase", color: "text.tertiary" }}>
              {GROUP_LABEL[group]}
            </Typography>
            <TileGrid apps={items} pinned={pins} onTogglePin={toggle} />
          </Box>
        ))}
      </Container>
      <ProductBadge />
      {addUserOpen && <AddUserDialog onClose={() => setAddUserOpen(false)} />}
    </>
  );
}
