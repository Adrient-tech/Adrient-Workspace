import { useCallback, useEffect, useMemo, useState } from "react";
import { Avatar } from "@mui/joy";
import GroupsOutlinedIcon from "@mui/icons-material/GroupsOutlined";
import AccountTreeOutlinedIcon from "@mui/icons-material/AccountTreeOutlined";
import { whoami } from "../api/client";
import { lookupDirectory, searchDirectoryPage, type DirectoryEntry, type DirectoryType } from "../api/directorySearch";
import { apps } from "../catalog/apps";
import { AsyncPicker, type PickerItem, type PickerPage } from "./AsyncPicker";

/** A picked person, group, unit, role or app. `id` is what gets stored. */
export interface DirectoryItem extends PickerItem {
  type: DirectoryType | "role" | "app";
  email?: string;
  department?: string;
}

/** Converts a search hit to a picker row (exported for tests and callers that hold raw entries). */
export function toItem(e: DirectoryEntry): DirectoryItem {
  if (e.type === "group") {
    return { id: e.id, type: "group", label: e.name, secondary: e.email, tertiary: e.count ? `${e.count} members` : undefined };
  }
  if (e.type === "unit") {
    return { id: e.id, type: "unit", label: e.name, secondary: e.department ? `Inside ${e.department}` : "Organisation unit" };
  }
  const detail = [e.department, e.job_title, e.employee_id].filter(Boolean).join(" · ");
  return {
    id: e.id,
    type: "user",
    label: e.name,
    email: e.email,
    department: e.department,
    secondary: e.email,
    tertiary: detail || undefined,
    // No avatar image per row: a scrolled list would fire one request per
    // person (most answer 404), which the scan-burst tripwire reads as probing.
  };
}

let meCache: Promise<DirectoryItem | null> | null = null;
/** The signed-in person, fetched once, used for the "Me" shortcut. */
export function useMe(): DirectoryItem | null {
  const [me, setMe] = useState<DirectoryItem | null>(null);
  useEffect(() => {
    let live = true;
    meCache ??= whoami().then((r) =>
      r.status === "ok"
        ? ({
            id: r.data.user.id,
            type: "user",
            label: r.data.user.display_name || r.data.user.email,
            secondary: r.data.user.email,
            email: r.data.user.email,
          } as DirectoryItem)
        : null,
    );
    void meCache.then((m) => live && setMe(m));
    return () => {
      live = false;
    };
  }, []);
  return me;
}

interface CommonProps<T> {
  value: T[];
  onChange: (next: T[]) => void;
  multiple?: boolean;
  placeholder?: string;
  ariaLabel?: string;
  disabled?: boolean;
  testId?: string;
  maxChips?: number;
  size?: "sm" | "md";
  minWidth?: number | string;
}

function useDirectoryFetch(type: DirectoryType, includeBlocked?: boolean) {
  return useCallback(
    async (q: string, cursor: string | undefined, signal: AbortSignal): Promise<PickerPage<DirectoryItem>> => {
      const p = await searchDirectoryPage(type, q, cursor, signal, { includeBlocked });
      return { items: p.items.map(toItem), next_cursor: p.next_cursor };
    },
    [type, includeBlocked],
  );
}

/** Async, server-searched picker for people (single or multi select). */
export function PeoplePicker(props: CommonProps<DirectoryItem> & { includeBlocked?: boolean; hideMe?: boolean }) {
  const fetchPage = useDirectoryFetch("user", props.includeBlocked);
  const me = useMe();
  return (
    <AsyncPicker<DirectoryItem>
      fetchPage={fetchPage}
      value={props.value}
      onChange={props.onChange}
      multiple={props.multiple}
      placeholder={props.placeholder ?? (props.multiple ? "Add people by name, email or ID" : "Search people")}
      ariaLabel={props.ariaLabel ?? "People"}
      disabled={props.disabled}
      testId={props.testId}
      maxChips={props.maxChips}
      size={props.size}
      minWidth={props.minWidth}
      recentKey="people"
      me={props.hideMe ? null : me}
      emptyText="No people match. Try a name, email, employee ID or department."
    />
  );
}

export function GroupPicker(props: CommonProps<DirectoryItem>) {
  const fetchPage = useDirectoryFetch("group");
  return (
    <AsyncPicker<DirectoryItem>
      fetchPage={fetchPage}
      value={props.value}
      onChange={props.onChange}
      multiple={props.multiple}
      placeholder={props.placeholder ?? "Search groups"}
      ariaLabel={props.ariaLabel ?? "Groups"}
      disabled={props.disabled}
      testId={props.testId}
      maxChips={props.maxChips}
      size={props.size}
      minWidth={props.minWidth}
      recentKey="groups"
      renderIcon={() => <GroupsOutlinedIcon fontSize="small" />}
      emptyText="No groups match."
    />
  );
}

export function UnitPicker(props: CommonProps<DirectoryItem>) {
  const fetchPage = useDirectoryFetch("unit");
  return (
    <AsyncPicker<DirectoryItem>
      fetchPage={fetchPage}
      value={props.value}
      onChange={props.onChange}
      multiple={props.multiple}
      placeholder={props.placeholder ?? "Search organisation units"}
      ariaLabel={props.ariaLabel ?? "Organisation units"}
      disabled={props.disabled}
      testId={props.testId}
      maxChips={props.maxChips}
      size={props.size}
      minWidth={props.minWidth}
      recentKey="units"
      renderIcon={() => <AccountTreeOutlinedIcon fontSize="small" />}
      emptyText="No organisation units match."
    />
  );
}

/** Builds a fetcher over a small fixed list (roles, apps): filtered in memory, paged like a server. */
export function staticFetcher(all: DirectoryItem[], pageSize = 30) {
  return async (q: string, cursor: string | undefined): Promise<PickerPage<DirectoryItem>> => {
    const needle = q.toLowerCase();
    const hits = all.filter((i) => !needle || `${i.label} ${i.secondary ?? ""}`.toLowerCase().includes(needle));
    const from = cursor ? Number(cursor) : 0;
    const items = hits.slice(from, from + pageSize);
    return { items, next_cursor: from + pageSize < hits.length ? String(from + pageSize) : undefined };
  };
}

/** Role picker; roles are a short list so they are filtered on the client. */
export function RolePicker(props: CommonProps<DirectoryItem> & { roles: { key: string; name: string }[] }) {
  const { roles } = props;
  const fetchPage = useMemo(
    () =>
      staticFetcher(roles.map((r) => ({ id: r.key, type: "role" as const, label: r.name, secondary: r.key }))),
    [roles],
  );
  return (
    <AsyncPicker<DirectoryItem>
      fetchPage={fetchPage}
      value={props.value}
      onChange={props.onChange}
      multiple={props.multiple}
      placeholder={props.placeholder ?? "Search roles"}
      ariaLabel={props.ariaLabel ?? "Roles"}
      disabled={props.disabled}
      testId={props.testId}
      size={props.size}
      minWidth={props.minWidth}
      emptyText="No roles match."
    />
  );
}

/** App picker over the catalogue of apps. */
export function AppPicker(props: CommonProps<DirectoryItem>) {
  const fetchPage = useMemo(
    () => staticFetcher(apps.map((a) => ({ id: a.id, type: "app" as const, label: a.name, secondary: a.blurb }))),
    [],
  );
  return (
    <AsyncPicker<DirectoryItem>
      fetchPage={fetchPage}
      value={props.value}
      onChange={props.onChange}
      multiple={props.multiple}
      placeholder={props.placeholder ?? "Search apps"}
      ariaLabel={props.ariaLabel ?? "Apps"}
      disabled={props.disabled}
      testId={props.testId}
      maxChips={props.maxChips}
      size={props.size}
      minWidth={props.minWidth}
      renderIcon={(i) => <Avatar size="sm">{i.label.slice(0, 1)}</Avatar>}
      emptyText="No apps match."
    />
  );
}

/**
 * Keeps picker chips in sync with a plain id list: given stored ids it loads
 * the people behind them (one request), so forms that only keep ids can still
 * show names.
 */
export function usePeopleByIds(ids: string[]): DirectoryItem[] {
  const [items, setItems] = useState<DirectoryItem[]>([]);
  const key = ids.join(",");
  useEffect(() => {
    let live = true;
    void lookupDirectory(key ? key.split(",") : []).then((r) => live && setItems(r.map(toItem)));
    return () => {
      live = false;
    };
  }, [key]);
  return items;
}

/** Minimal person record callers may already hold, to skip a lookup for chips. */
export interface KnownPerson {
  id: string;
  name: string;
  email?: string;
}

function itemsFor(ids: string[], known: KnownPerson[] | undefined, loaded: DirectoryItem[]): DirectoryItem[] {
  return ids.map((id) => {
    const k = known?.find((x) => x.id === id);
    if (k) return { id, type: "user" as const, label: k.name || k.email || id, secondary: k.email, email: k.email };
    return loaded.find((l) => l.id === id) ?? { id, type: "user" as const, label: id.slice(0, 8) + "…" };
  });
}

/**
 * Drop-in replacement for a `<Select>` over every person: holds just the
 * selected id and searches the directory on the server as the user types.
 * `known` lets a screen pass names it already has so chips need no lookup.
 */
export function PersonSelect(props: {
  value: string;
  onChange: (id: string, item?: DirectoryItem) => void;
  known?: KnownPerson[];
  placeholder?: string;
  ariaLabel?: string;
  disabled?: boolean;
  testId?: string;
  size?: "sm" | "md";
  minWidth?: number | string;
}) {
  const { value, known } = props;
  const [picked, setPicked] = useState<DirectoryItem[] | null>(null);
  const needLookup = value && !known?.some((k) => k.id === value) ? [value] : [];
  const loaded = usePeopleByIds(needLookup);
  const fromPick = picked && (picked.length === 0 ? !value : picked[0]?.id === value) ? picked : null;
  const shown = fromPick ?? (value ? itemsFor([value], known, loaded) : []);
  return (
    <PeoplePicker
      value={shown}
      onChange={(n) => {
        setPicked(n);
        props.onChange(n[0]?.id ?? "", n[0]);
      }}
      placeholder={props.placeholder ?? "Search people"}
      ariaLabel={props.ariaLabel ?? "Person"}
      disabled={props.disabled}
      testId={props.testId}
      size={props.size}
      minWidth={props.minWidth}
      hideMe={false}
    />
  );
}

/** Multi-select by ids, same idea as PersonSelect. */
export function PeopleSelect(props: {
  value: string[];
  onChange: (ids: string[], items: DirectoryItem[]) => void;
  known?: KnownPerson[];
  placeholder?: string;
  ariaLabel?: string;
  disabled?: boolean;
  testId?: string;
  maxChips?: number;
  minWidth?: number | string;
}) {
  const { value, known } = props;
  const need = value.filter((id) => !known?.some((k) => k.id === id));
  const loaded = usePeopleByIds(need);
  const [extra, setExtra] = useState<DirectoryItem[]>([]);
  const shown = itemsFor(value, known, [...loaded, ...extra]);
  return (
    <PeoplePicker
      multiple
      value={shown}
      onChange={(n) => {
        setExtra((old) => [...old, ...n]);
        props.onChange(n.map((i) => i.id), n);
      }}
      placeholder={props.placeholder ?? "Add people by name, email or ID"}
      ariaLabel={props.ariaLabel ?? "People"}
      disabled={props.disabled}
      testId={props.testId}
      maxChips={props.maxChips}
      minWidth={props.minWidth}
      hideMe={false}
    />
  );
}
