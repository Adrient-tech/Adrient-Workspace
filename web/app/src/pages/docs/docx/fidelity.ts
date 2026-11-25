// DOCX round-trip fidelity metric (Docs wave 6).
//
// `inventory()` counts how many of each OOXML construct a package carries
// (paragraphs, tables, field codes, revisions, section properties, ...), by
// scanning the XML text. `compareInventories()` scores a round trip
// (docx -> Adrient -> docx) as, per feature, min(before, after) / before, so a
// construct that is dropped scores 0 and one that is preserved scores 1.
// Extra constructs created by the writer are not rewarded.
//
// The metric measures structure kept, not pixels; it is what the corpus run
// (docx-fidelity.test.ts, tools/docx-fidelity.mjs) reports before and after
// each fidelity fix.
import JSZip from "jszip";

export type Inventory = Record<string, number>;

/** One countable construct: a name, the parts it lives in, and a regex. */
interface Probe {
  name: string;
  /** "body" = document.xml; "story" = header/footer/footnotes/endnotes; "all" = every word part. */
  scope: "body" | "story" | "all" | "styles" | "numbering" | "settings";
  re: RegExp;
}

const c = (name: string, re: RegExp, scope: Probe["scope"] = "all"): Probe => ({ name, re, scope });

export const PROBES: Probe[] = [
  // Text structure
  c("paragraph", /<w:p[ >\/]/g),
  c("run", /<w:r[ >]/g),
  c("tab", /<w:tab\/>/g),
  c("line-break", /<w:br\b(?![^>]*w:type="(?:page|column)")[^>]*\/?>/g),
  c("page-break", /<w:br\b[^>]*w:type="page"/g),
  c("column-break", /<w:br\b[^>]*w:type="column"/g),
  c("soft-hyphen", /<w:softHyphen\/>/g),
  c("symbol", /<w:sym\b/g),
  // Character formatting
  c("fmt-bold", /<w:b(?: w:val="(?:1|true|on)")?\/>/g),
  c("fmt-italic", /<w:i(?: w:val="(?:1|true|on)")?\/>/g),
  c("fmt-underline", /<w:u w:val="(?!none)[^"]*"/g),
  c("fmt-strike", /<w:(?:strike|dstrike)\b/g),
  c("fmt-color", /<w:color w:val="(?!auto)[^"]*"/g),
  c("fmt-size", /<w:sz w:val=/g),
  // Highlight and run shading are the same thing in Adrient (a highlight mark).
  c("fmt-highlight", /<w:highlight w:val=|<w:rPr>(?:(?!<\/w:rPr>)[^])*?<w:shd\b/g),
  c("fmt-font", /<w:rFonts [^>]*w:(?:ascii|hAnsi|cs|eastAsia)="/g),
  c("fmt-vertalign", /<w:vertAlign w:val="(?:superscript|subscript)"/g),
  c("fmt-caps", /<w:(?:caps|smallCaps)\b/g),
  c("fmt-spacing-char", /<w:spacing w:val=/g),
  c("fmt-lang", /<w:lang\b/g),
  c("fmt-hidden", /<w:vanish\b/g),
  c("fmt-rstyle", /<w:rStyle w:val=/g),
  // Paragraph formatting
  c("para-style", /<w:pStyle w:val=/g),
  c("para-align", /<w:jc w:val="(?:center|right|both|distribute|end)"/g),
  c("para-indent", /<w:ind\b/g),
  c("para-spacing", /<w:spacing w:(?:before|after|line|beforeLines|afterLines|beforeAutospacing|afterAutospacing)/g),
  c("para-keep", /<w:(?:keepNext|keepLines|pageBreakBefore)\b|<w:widowControl w:val="(?:0|false)"/g),
  c("para-border", /<w:pBdr>/g),
  c("para-shading", /<w:pPr>(?:(?!<\/w:pPr>).)*?<w:shd\b/gs),
  c("para-tabstop", /<w:tab w:val=/g),
  c("para-outline", /<w:outlineLvl\b/g),
  c("para-framepr", /<w:framePr\b/g),
  c("para-bidi", /<w:bidi\b/g),
  c("para-textdirection", /<w:textDirection\b/g),
  // Lists
  c("list-item", /<w:numPr>/g),
  // Tables
  c("table", /<w:tbl>/g),
  c("table-row", /<w:tr[ >]/g),
  c("table-cell", /<w:tc>/g),
  c("table-gridspan", /<w:gridSpan\b/g),
  c("table-vmerge", /<w:vMerge\b/g),
  c("table-header-row", /<w:tblHeader\b/g),
  c("table-cell-shading", /<w:tcPr>(?:(?!<\/w:tcPr>).)*?<w:shd\b/gs),
  c("table-cell-border", /<w:tcBorders>/g),
  c("table-borders", /<w:tblBorders>/g),
  c("table-style", /<w:tblStyle\b/g),
  c("table-width", /<w:tblW\b/g),
  c("table-cell-valign", /<w:vAlign\b/g),
  c("table-row-height", /<w:trHeight\b/g),
  c("table-cell-margin", /<w:(?:tcMar|tblCellMar)>/g),
  c("table-floating", /<w:tblpPr\b/g),
  // Objects
  c("image", /<a:blip\b[^>]*r:(?:embed|link)=/g),
  c("drawing-inline", /<wp:inline\b/g),
  c("drawing-anchor", /<wp:anchor\b/g),
  c("drawing-wrap", /<wp:wrap(?:Square|Tight|Through|TopAndBottom|None)\b/g),
  c("drawing-alt-text", /<wp:docPr\b[^>]*\bdescr="[^"]+"/g),
  c("shape", /<wps:wsp\b/g),
  c("textbox", /<w:txbxContent>/g),
  c("chart", /<c:chart\b/g),
  c("diagram", /<dgm:relIds\b/g),
  c("math", /<m:oMath\b/g),
  c("vml-shape", /<v:(?:shape|rect|roundrect|oval|line|group)\b/g),
  c("ole-object", /<w:object\b/g),
  c("pict", /<w:pict\b/g),
  // Links and references
  c("hyperlink", /<w:hyperlink\b/g),
  c("bookmark", /<w:bookmarkStart\b/g),
  c("footnote-ref", /<w:footnoteReference\b/g),
  c("endnote-ref", /<w:endnoteReference\b/g),
  c("comment-ref", /<w:commentReference\b/g),
  c("comment-range", /<w:commentRangeStart\b/g),
  // Fields
  c("field", /<w:fldSimple\b|<w:fldChar w:fldCharType="begin"/g),
  c("field-page", /(?:w:instr="|<w:instrText[^>]*>)\s*(?:PAGE|NUMPAGES|SECTIONPAGES)\b/g),
  c("field-toc", /(?:w:instr="|<w:instrText[^>]*>)\s*(?:TOC|TOA|INDEX|TC|XE)\b/g),
  c("field-ref", /(?:w:instr="|<w:instrText[^>]*>)\s*(?:REF|PAGEREF|NOTEREF|SEQ|STYLEREF)\b/g),
  c("field-date", /(?:w:instr="|<w:instrText[^>]*>)\s*(?:DATE|TIME|SAVEDATE|CREATEDATE|PRINTDATE|EDITTIME)\b/g),
  c("field-merge", /(?:w:instr="|<w:instrText[^>]*>)\s*(?:MERGEFIELD|IF|NEXT|NEXTIF|SKIPIF|ASK|FILLIN|SET|COMPARE)\b/g),
  c("field-docinfo", /(?:w:instr="|<w:instrText[^>]*>)\s*(?:TITLE|AUTHOR|SUBJECT|KEYWORDS|COMMENTS|FILENAME|DOCPROPERTY|NUMWORDS|NUMCHARS)\b/g),
  c("field-other", /(?:w:instr="|<w:instrText[^>]*>)\s*(?:HYPERLINK|INCLUDETEXT|INCLUDEPICTURE|EQ|SYMBOL|FORMTEXT|FORMCHECKBOX|CITATION|BIBLIOGRAPHY|MACROBUTTON|=)/g),
  // Review
  // A move is an insertion at the target and a deletion at the source.
  c("rev-insert", /<w:(?:ins|moveTo)\b[^>]*w:author=/g),
  c("rev-delete", /<w:(?:del|moveFrom)\b[^>]*w:author=/g),
  c("rev-format", /<w:(?:rPrChange|pPrChange|sectPrChange|tblPrChange|tcPrChange|trPrChange)\b/g),
  // Content controls
  c("sdt", /<w:sdt>/g),
  c("sdt-checkbox", /<w14:checkbox\b/g),
  c("sdt-dropdown", /<w:(?:dropDownList|comboBox)\b/g),
  c("sdt-date", /<w:date\b/g),
  // Sections and page setup
  c("section", /<w:sectPr[ >]/g),
  c("section-header-ref", /<w:headerReference\b/g),
  c("section-footer-ref", /<w:footerReference\b/g),
  c("section-title-page", /<w:titlePg\b/g),
  c("section-columns", /<w:cols\b[^>]*w:num="(?:[2-9]|\d\d)"/g),
  c("section-pgsize", /<w:pgSz\b/g),
  c("section-landscape", /<w:pgSz\b[^>]*w:orient="landscape"/g),
  c("section-pgnumtype", /<w:pgNumType\b/g),
  c("section-linenumbers", /<w:lnNumType\b/g),
  c("section-borders", /<w:pgBorders\b/g),
  c("section-vertical-align", /<w:vAlign w:val="(?:center|both|bottom)"/g),
  c("section-type", /<w:type w:val="(?:continuous|evenPage|oddPage|nextColumn)"/g),
  c("section-footnote-pr", /<w:(?:footnotePr|endnotePr)\b/g),
  c("page-background", /<w:background\b/g),
  // Settings
  c("setting-even-odd", /<w:evenAndOddHeaders\b/g, "settings"),
  c("setting-track-revisions", /<w:trackRevisions\b/g, "settings"),
  c("setting-default-tab", /<w:defaultTabStop\b/g, "settings"),
  c("setting-footnote-pr", /<w:(?:footnotePr|endnotePr)\b/g, "settings"),
  c("setting-protection", /<w:documentProtection\b/g, "settings"),
  c("setting-compat", /<w:compatSetting\b/g, "settings"),
  c("setting-mirror-margins", /<w:mirrorMargins\b/g, "settings"),
  c("setting-theme-font-lang", /<w:themeFontLang\b/g, "settings"),
  // Styles and numbering definitions
  c("style-def", /<w:style\b/g, "styles"),
  c("style-paragraph", /<w:style\b[^>]*w:type="paragraph"/g, "styles"),
  c("style-character", /<w:style\b[^>]*w:type="character"/g, "styles"),
  c("style-table", /<w:style\b[^>]*w:type="table"/g, "styles"),
  c("style-based-on", /<w:basedOn\b/g, "styles"),
  c("style-next", /<w:next\b/g, "styles"),
  c("style-linked", /<w:link\b/g, "styles"),
  c("style-latent", /<w:lsdException\b/g, "styles"),
  c("style-doc-defaults", /<w:docDefaults>/g, "styles"),
  c("style-table-conditional", /<w:tblStylePr\b/g, "styles"),
  c("numbering-abstract", /<w:abstractNum\b/g, "numbering"),
  c("numbering-num", /<w:num\b/g, "numbering"),
  c("numbering-level", /<w:lvl\b/g, "numbering"),
  c("numbering-override", /<w:lvlOverride\b/g, "numbering"),
  c("numbering-multilevel", /<w:multiLevelType\b/g, "numbering"),
  c("numbering-picture-bullet", /<w:numPicBullet\b/g, "numbering"),
];

function stripFor(scope: Probe["scope"], parts: Record<string, string>): string {
  const names = Object.keys(parts);
  const pick = (f: (n: string) => boolean) => names.filter(f).map((n) => parts[n]).join("\n");
  switch (scope) {
    case "styles":
      return parts["word/styles.xml"] ?? "";
    case "numbering":
      return parts["word/numbering.xml"] ?? "";
    case "settings":
      return parts["word/settings.xml"] ?? "";
    case "body":
      return parts["word/document.xml"] ?? "";
    case "story":
      return pick((n) => /^word\/(header|footer|footnotes|endnotes)\d*\.xml$/.test(n));
    default:
      return pick((n) => /^word\/(document|header\d*|footer\d*|footnotes|endnotes)\.xml$|^word\/(header|footer)\d+\.xml$/.test(n));
  }
}

/** Reads the WordprocessingML parts of a package as text. */
export async function readParts(data: ArrayBuffer | Uint8Array): Promise<Record<string, string>> {
  const zip = await JSZip.loadAsync(data);
  const out: Record<string, string> = {};
  for (const name of Object.keys(zip.files)) {
    if (zip.files[name].dir) continue;
    if (/^word\/[^/]+\.xml$/.test(name) || name === "docProps/core.xml" || name === "docProps/app.xml")
      out[name] = await zip.file(name)!.async("string");
  }
  // Part-level counts that live outside the XML text.
  out["__media"] = String(Object.keys(zip.files).filter((n) => /^word\/media\//.test(n)).length);
  return out;
}

/** Counts every probe in the package; `parts` come from readParts. */
export function inventory(parts: Record<string, string>): Inventory {
  const inv: Inventory = {};
  for (const p of PROBES) {
    const text = stripFor(p.scope, parts);
    inv[p.name] = (text.match(p.re) ?? []).length;
  }
  // Story-level (part) counts.
  inv["part-header"] = Object.keys(parts).filter((n) => /^word\/header\d*\.xml$/.test(n)).length;
  inv["part-footer"] = Object.keys(parts).filter((n) => /^word\/footer\d*\.xml$/.test(n)).length;
  inv["part-footnote"] = ((parts["word/footnotes.xml"] ?? "").match(/<w:footnote\b(?![^>]*w:type=")/g) ?? []).length;
  inv["part-endnote"] = ((parts["word/endnotes.xml"] ?? "").match(/<w:endnote\b(?![^>]*w:type=")/g) ?? []).length;
  inv["part-comment"] = ((parts["word/comments.xml"] ?? "").match(/<w:comment\b/g) ?? []).length;
  inv["part-media"] = Number(parts["__media"] ?? 0);
  inv["part-theme"] = Object.keys(parts).some((n) => /^word\/theme\//.test(n)) ? 1 : 0;
  inv["part-font-table"] = parts["word/fontTable.xml"] ? 1 : 0;
  inv["prop-title"] = /<dc:title>[^<]+/.test(parts["docProps/core.xml"] ?? "") ? 1 : 0;
  inv["prop-creator"] = /<dc:creator>[^<]+/.test(parts["docProps/core.xml"] ?? "") ? 1 : 0;
  // Text volume, as words in w:t / w:delText.
  const text = stripFor("all", parts);
  inv["text-words"] = [...text.matchAll(/<w:(?:t|delText)(?:\s[^>]*)?>([^<]*)</g)]
    .map((m) => m[1])
    .join(" ")
    .split(/\s+/)
    .filter(Boolean).length;
  return inv;
}

export interface FeatureScore {
  feature: string;
  before: number;
  kept: number;
}

/** min(before, after) per feature that the source carries. */
export function compareInventories(before: Inventory, after: Inventory): FeatureScore[] {
  const out: FeatureScore[] = [];
  for (const [feature, b] of Object.entries(before)) {
    if (!b) continue;
    out.push({ feature, before: b, kept: Math.min(b, after[feature] ?? 0) });
  }
  return out;
}

/** Per-document score: the mean over the features the source carries, 0..1. */
export function documentScore(scores: FeatureScore[]): number {
  if (!scores.length) return 1;
  return scores.reduce((s, f) => s + f.kept / f.before, 0) / scores.length;
}

export interface Aggregate {
  /** Mean of the per-document scores. */
  documentMean: number;
  /** Sum of kept / sum of before over all features and documents. */
  weighted: number;
  perFeature: { feature: string; before: number; kept: number; ratio: number }[];
  documents: number;
}

export function aggregate(docs: FeatureScore[][]): Aggregate {
  const per = new Map<string, { before: number; kept: number }>();
  let sumBefore = 0;
  let sumKept = 0;
  for (const d of docs)
    for (const f of d) {
      const e = per.get(f.feature) ?? { before: 0, kept: 0 };
      e.before += f.before;
      e.kept += f.kept;
      per.set(f.feature, e);
      sumBefore += f.before;
      sumKept += f.kept;
    }
  return {
    documentMean: docs.length ? docs.reduce((s, d) => s + documentScore(d), 0) / docs.length : 1,
    weighted: sumBefore ? sumKept / sumBefore : 1,
    perFeature: [...per.entries()]
      .map(([feature, v]) => ({ feature, ...v, ratio: v.kept / v.before }))
      .sort((a, b) => a.ratio - b.ratio || b.before - a.before),
    documents: docs.length,
  };
}
