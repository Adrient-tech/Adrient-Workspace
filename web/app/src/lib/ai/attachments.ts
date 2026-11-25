/** Attach text files from Drive to an assistant message. */
export interface Attachment {
  name: string;
  text: string;
}

export const MAX_ATTACH_BYTES = 200 * 1024;

const TEXT_TYPES = /^(text\/|application\/(json|xml|csv|x-yaml|yaml|markdown|x-ndjson))/;
const TEXT_EXT = /\.(txt|md|markdown|csv|tsv|json|xml|ya?ml|log|ini|html?)$/i;

/** Whether a Drive file can be attached (text-like and small enough). */
export function attachable(f: { name: string; mime_type?: string; size_bytes?: string | number }): string | null {
  const size = Number(f.size_bytes ?? 0);
  if (!(TEXT_TYPES.test(f.mime_type ?? "") || TEXT_EXT.test(f.name))) {
    return "Only text files (txt, md, csv, json, xml) can be attached for now.";
  }
  if (size > MAX_ATTACH_BYTES) return "The file is larger than 200 KB.";
  return null;
}

/** Downloads a Drive file as text with the user's own session (Drive enforces access). */
export async function loadDriveText(f: { id: string; name: string; mime_type?: string; size_bytes?: string | number }): Promise<Attachment> {
  const why = attachable(f);
  if (why) throw new Error(why);
  const r = await fetch(`/api/v1/drive/files/${encodeURIComponent(f.id)}/content`, { credentials: "include" });
  if (!r.ok) throw new Error(`Could not open ${f.name} (HTTP ${r.status})`);
  const text = await r.text();
  return { name: f.name, text: text.slice(0, MAX_ATTACH_BYTES) };
}
