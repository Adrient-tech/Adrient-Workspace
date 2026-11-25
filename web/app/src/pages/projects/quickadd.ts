// "Ship the beta !high #design @maya friday" becomes an issue draft with live chips.
// Dates reuse the Tasks parser. A chip can be dismissed ("keep as text"): its text returns to the title.
import { parseQuickAdd } from "../tasks/quickparse";

export interface QuickIssueToken {
  key: string;
  type: "due" | "priority" | "label" | "assignee";
  label: string;
  text: string;
}
export interface QuickIssueDraft {
  title: string;
  /** 0 none, 1 urgent, 2 high, 3 medium, 4 low (Projects scale) */
  priority: number;
  assigneeId: string;
  labelIds: string[];
  /** YYYY-MM-DD or "" */
  due: string;
  tokens: QuickIssueToken[];
}

const PRIORITY_LABEL = ["", "Urgent", "High", "Medium", "Low"];
const B = String.raw`(?=[\s,.!?;]|$)`;
/** Tasks scale (3 high, 2 medium, 1 low) to Projects scale. */
const FROM_TASKS = [0, 4, 3, 2];

export function parseIssueQuick(
  text: string,
  o: { members?: { id: string; name: string }[]; labels?: { id: string; name: string }[]; today?: string; ignore?: Set<string> } = {},
): QuickIssueDraft {
  const members = o.members ?? [];
  const labels = o.labels ?? [];
  const ignore = o.ignore ?? new Set<string>();
  const tokens: QuickIssueToken[] = [];
  const out: QuickIssueDraft = { title: "", priority: 0, assigneeId: "", labelIds: [], due: "", tokens };
  let work = text;
  const blank = (i: number, len: number) => {
    work = work.slice(0, i) + " ".repeat(len) + work.slice(i + len);
  };

  // Urgent is not in the Tasks parser: handle it first.
  if (!ignore.has("priority")) {
    const m = new RegExp(String.raw`(^|\s)!(urgent|p0)${B}`, "i").exec(work);
    if (m) {
      out.priority = 1;
      tokens.push({ key: "priority", type: "priority", label: "Urgent", text: m[0].trim() });
      blank(m.index, m[0].length);
    }
  }

  // Assignees: @name matches a member by first word or full name, case-insensitively.
  if (!ignore.has("assignee")) {
    const re = new RegExp(String.raw`(^|\s)@([\p{L}\p{N}._-]+)${B}`, "giu");
    let m: RegExpExecArray | null;
    while ((m = re.exec(work))) {
      const w = m[2].toLowerCase();
      const who = members.find((x) => x.name.toLowerCase() === w) ?? members.find((x) => x.name.toLowerCase().split(/\s+/)[0] === w) ?? members.find((x) => x.name.toLowerCase().startsWith(w));
      if (who && !out.assigneeId) {
        out.assigneeId = who.id;
        tokens.push({ key: "assignee", type: "assignee", label: `@${who.name}`, text: m[0].trim() });
        blank(m.index, m[0].length);
        break;
      }
    }
  }

  // Everything else goes through the shared parser. #words that name an existing label become label chips.
  const p = parseQuickAdd(work, { today: o.today, ignore: new Set([...ignore].filter((k) => k !== "assignee" && k !== "label" && !k.startsWith("label:")).concat(["repeat", "time", "list"])) });
  work = p.title;
  if (p.priority && !out.priority && !ignore.has("priority")) {
    out.priority = FROM_TASKS[p.priority];
    tokens.push({ key: "priority", type: "priority", label: PRIORITY_LABEL[out.priority], text: p.tokens.find((t) => t.type === "priority")?.text ?? "" });
  }
  if (p.due) {
    out.due = p.due;
    tokens.push({ key: "due", type: "due", label: p.due, text: p.tokens.find((t) => t.type === "due")?.text ?? "" });
  }
  // Tags from the shared parser: keep those matching a label as chips, give the rest back to the title.
  const unmatched: string[] = [];
  for (const tag of p.tags) {
    const lab = labels.find((l) => l.name.toLowerCase() === tag) ?? labels.find((l) => l.name.toLowerCase().replace(/\s+/g, "-") === tag);
    if (lab && !ignore.has(`label:${tag}`) && !ignore.has("label")) {
      if (!out.labelIds.includes(lab.id)) out.labelIds.push(lab.id);
      tokens.push({ key: `label:${tag}`, type: "label", label: `#${lab.name}`, text: `#${tag}` });
    } else unmatched.push(`#${tag}`);
  }
  out.title = [work, ...unmatched].join(" ").replace(/\s+/g, " ").trim();
  return out;
}
