import { useCallback, useState, type ReactNode } from "react";
import { useNavigate } from "react-router-dom";
import { Avatar, Box, Button, CircularProgress, Link, Menu, Typography } from "@mui/joy";
import ChatBubbleOutlineIcon from "@mui/icons-material/ChatBubbleOutline";
import VideocamOutlinedIcon from "@mui/icons-material/VideocamOutlined";
import MailOutlineIcon from "@mui/icons-material/MailOutline";
import EventOutlinedIcon from "@mui/icons-material/EventOutlined";
import { findPersonByEmail, peopleApi, type PersonProfile } from "../api/people";
import { startCall } from "../lib/calls";
import {
  availableActions,
  runAction,
  type PersonAction,
  type PersonTarget,
} from "./personActions";

export interface PersonCardProps {
  /** Adrient user id (preferred) ... */
  userId?: string;
  /** ... or an email address, resolved against the org directory. */
  email?: string;
  /** Display name shown until the profile loads (and for unknown emails). */
  name?: string;
  children: ReactNode;
}

type Loaded =
  | { state: "loading" }
  | { state: "ready"; profile: PersonProfile }
  | { state: "external" }; // email that is not an org user

const LABELS: Record<PersonAction, string> = {
  chat: "Chat",
  call: "Call (Meet)",
  email: "Email",
  schedule: "Schedule",
};
const ICONS: Record<PersonAction, ReactNode> = {
  chat: <ChatBubbleOutlineIcon fontSize="small" />,
  call: <VideocamOutlinedIcon fontSize="small" />,
  email: <MailOutlineIcon fontSize="small" />,
  schedule: <EventOutlinedIcon fontSize="small" />,
};

/**
 * Wraps a person's name or avatar. Clicking it opens a card with the person's
 * title, department, contact details and manager plus Chat / Call / Email /
 * Schedule buttons that open those apps.
 */
export function PersonCard({ userId, email, name, children }: PersonCardProps) {
  const navigate = useNavigate();
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  const [loaded, setLoaded] = useState<Loaded>({ state: "loading" });
  const [busy, setBusy] = useState<PersonAction | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoaded({ state: "loading" });
    try {
      let id = userId;
      if (!id && email) id = (await findPersonByEmail(email))?.id;
      if (!id) {
        setLoaded({ state: "external" });
        return;
      }
      setLoaded({ state: "ready", profile: await peopleApi.profile(id) });
    } catch {
      setLoaded({ state: "external" });
    }
  }, [userId, email]);

  const open = (e: React.SyntheticEvent<HTMLElement>) => {
    e.stopPropagation();
    setAnchor(e.currentTarget);
    setErr(null);
    void load();
  };

  const p = loaded.state === "ready" ? loaded.profile.person : null;
  const target: PersonTarget = {
    id: p?.id ?? userId,
    name: p?.name ?? name,
    email: p?.email ?? email,
  };
  const actions = loaded.state === "loading" ? [] : availableActions(target);

  const act = async (a: PersonAction) => {
    setBusy(a);
    setErr(null);
    try {
      await runAction(a, target, {
        navigate,
        pathname: window.location.pathname,
        createCall: async (title, userId) =>
          (await startCall({ userIds: userId ? [userId] : [], title, video: true, returnTo: window.location.pathname + window.location.search })).path,
        openWindow: (url) => window.open(url, "_blank", "noopener"),
        dispatch: (n, detail) => window.dispatchEvent(new CustomEvent(n, { detail })),
      });
      setAnchor(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(null);
    }
  };

  const shownName = p?.name ?? name ?? email ?? "Unknown";
  const profile = loaded.state === "ready" ? loaded.profile : null;

  return (
    <>
      <Box
        component="span"
        role="button"
        tabIndex={0}
        data-testid="person-card-trigger"
        onClick={open}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            open(e);
          }
        }}
        sx={{ cursor: "pointer", display: "inline-flex", "&:hover": { opacity: 0.85 } }}
      >
        {children}
      </Box>
      <Menu
        open={!!anchor}
        anchorEl={anchor}
        onClose={() => setAnchor(null)}
        placement="bottom-start"
        sx={{ p: 0, width: 300, zIndex: 2000 }}
      >
        <Box data-testid="person-card" sx={{ p: 2, display: "flex", flexDirection: "column", gap: 1.25 }}>
          <Box sx={{ display: "flex", gap: 1.5, alignItems: "center" }}>
            <Avatar size="lg" src={target.id ? `/api/v1/users/${target.id}/avatar` : undefined}>
              {shownName.charAt(0).toUpperCase()}
            </Avatar>
            <Box sx={{ minWidth: 0 }}>
              <Typography level="title-md" noWrap data-testid="person-card-name">
                {shownName}
              </Typography>
              {p?.job_title && (
                <Typography level="body-sm" noWrap data-testid="person-card-title">
                  {p.job_title}
                </Typography>
              )}
              {profile?.department && (
                <Typography level="body-xs" noWrap data-testid="person-card-department">
                  {profile.department.name}
                </Typography>
              )}
            </Box>
          </Box>
          {loaded.state === "loading" && <CircularProgress size="sm" />}
          {target.email && (
            <Link level="body-sm" href={`mailto:${target.email}`} sx={{ wordBreak: "break-all" }}>
              {target.email}
            </Link>
          )}
          {p?.phone && (
            <Link level="body-sm" href={`tel:${p.phone}`} data-testid="person-card-phone">
              {p.phone}
            </Link>
          )}
          {p?.location && <Typography level="body-sm">{p.location}</Typography>}
          {profile?.manager && (
            <Typography level="body-sm" data-testid="person-card-manager">
              Reports to{" "}
              <PersonCard userId={profile.manager.id} name={profile.manager.name}>
                <Link component="span" level="body-sm">
                  {profile.manager.name}
                </Link>
              </PersonCard>
            </Typography>
          )}
          {err && (
            <Typography level="body-xs" color="danger">
              {err}
            </Typography>
          )}
          {actions.length > 0 && (
            <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.75 }}>
              {actions.map((a) => (
                <Button
                  key={a}
                  size="sm"
                  variant="soft"
                  startDecorator={ICONS[a]}
                  loading={busy === a}
                  onClick={() => void act(a)}
                  data-testid={`person-card-${a}`}
                >
                  {LABELS[a]}
                </Button>
              ))}
            </Box>
          )}
        </Box>
      </Menu>
    </>
  );
}
