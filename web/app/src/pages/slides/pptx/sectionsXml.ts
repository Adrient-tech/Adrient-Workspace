// PowerPoint sections: p14:sectionLst in presentation.xml's extLst.
import type JSZip from "jszip";
import { sectionRanges } from "../sections";
import { uid, type DeckDoc } from "../model";

const DEFAULT_NAME = "Default Section";
const URI = "{521415D9-36F7-43E2-AB2F-B90AF26B5E84}";
const P14 = "http://schemas.microsoft.com/office/powerpoint/2010/main";
const esc = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
const guid = () => {
  const h = (uid() + uid() + uid() + uid()).replace(/[^0-9a-f]/gi, "0").padEnd(32, "0").slice(0, 32).toUpperCase();
  return `{${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}}`;
};

/** Add the section list to ppt/presentation.xml (no-op for decks without sections). */
export async function addSectionParts(zip: JSZip, deck: DeckDoc): Promise<void> {
  if (!deck.sections?.length) return;
  const f = zip.file("ppt/presentation.xml");
  if (!f) return;
  let xml = await f.async("string");
  const ids = [...(/<p:sldIdLst>([\s\S]*?)<\/p:sldIdLst>/.exec(xml)?.[1] ?? "").matchAll(/<p:sldId\b[^>]*\bid="(\d+)"/g)].map((m) => m[1]);
  if (ids.length !== deck.slides.length) return;
  const ranges = sectionRanges(deck).map((r) => ({ name: r.section?.name ?? DEFAULT_NAME, ids: ids.slice(r.start, r.end) }));
  const list =
    `<p:ext uri="${URI}"><p14:sectionLst xmlns:p14="${P14}">` +
    ranges
      .map((r) => `<p14:section name="${esc(r.name)}" id="${guid()}"><p14:sldIdLst>${r.ids.map((i) => `<p14:sldId id="${i}"/>`).join("")}</p14:sldIdLst></p14:section>`)
      .join("") +
    `</p14:sectionLst></p:ext>`;
  if (/<p:extLst>/.test(xml)) xml = xml.replace("<p:extLst>", `<p:extLst>${list}`);
  else xml = xml.replace("</p:presentation>", `<p:extLst>${list}</p:extLst></p:presentation>`);
  zip.file("ppt/presentation.xml", xml);
}

/** Sections from a parsed presentation.xml; `slideIdOf` maps a sldId number to our slide id. */
export function readSections(pres: Document, slideIdOf: (sldId: string) => string | undefined): NonNullable<DeckDoc["sections"]> {
  const out: NonNullable<DeckDoc["sections"]> = [];
  for (const sec of Array.from(pres.getElementsByTagName("*")).filter((e) => e.localName === "section")) {
    const first = Array.from(sec.getElementsByTagName("*")).find((e) => e.localName === "sldId");
    const sid = first ? slideIdOf(first.getAttribute("id") || "") : undefined;
    const name = sec.getAttribute("name") || "Section";
    if (sid) out.push({ id: uid(), name, slideId: sid });
  }
  // Our writer names the implicit leading group "Default Section"; it is
  // not a section of its own on our side.
  if (out.length > 1 && out[0].name === DEFAULT_NAME) out.shift();
  return out;
}
