import { useCallback, useMemo } from "react";
import { AsyncPicker, type PickerItem, type PickerPage } from "../../components/AsyncPicker";
import { searchDirectoryPage, type DirectoryType } from "../../api/directorySearch";
import { toItem } from "../../components/PeoplePicker";
import { accessApi, type Subject, type SubjectType } from "./accessApi";

const DIRECTORY_TYPES: Partial<Record<SubjectType, DirectoryType>> = {
  user: "user",
  group: "group",
  department: "unit",
};

/**
 * Search-as-you-type picker for users, groups, departments and roles.
 * Users, groups and departments are searched on the server a page at a time
 * (so it stays fast with 20,000 people); roles and user types are short fixed
 * lists and are filtered in the browser.
 */
export function SubjectPicker({
  type,
  value,
  onChange,
  placeholder,
  testId,
}: {
  type: Exclude<SubjectType, "everyone">;
  value: Subject | null;
  onChange: (s: Subject | null) => void;
  placeholder?: string;
  testId?: string;
}) {
  const dirType = DIRECTORY_TYPES[type];
  const fetchPage = useCallback(
    async (q: string, cursor: string | undefined, signal: AbortSignal): Promise<PickerPage<PickerItem>> => {
      if (dirType) {
        const p = await searchDirectoryPage(dirType, q, cursor, signal, { includeBlocked: true });
        return { items: p.items.map(toItem), next_cursor: p.next_cursor };
      }
      const all = await accessApi.subjects(type, q);
      const needle = q.toLowerCase();
      return {
        items: all
          .filter((s) => !needle || s.name.toLowerCase().includes(needle))
          .map((s) => ({ id: s.id, label: s.name, secondary: s.detail })),
      };
    },
    [type, dirType],
  );
  const selected = useMemo<PickerItem[]>(
    () => (value ? [{ id: value.id, label: value.name, secondary: value.detail }] : []),
    [value],
  );
  return (
    <AsyncPicker
      fetchPage={fetchPage}
      value={selected}
      onChange={(next) => {
        const n = next[0];
        onChange(n ? { id: n.id, name: n.label, detail: n.secondary } : null);
      }}
      placeholder={placeholder ?? `Search ${type}s`}
      ariaLabel={`Select ${type}`}
      testId={testId}
      recentKey={`subject-${type}`}
      minWidth={220}
    />
  );
}
