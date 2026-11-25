// Generated DOCX corpus for the round-trip fidelity metric (Docs wave 6).
// Every document is built in code from ECMA-376 structures; each one focuses
// on one family of constructs so the per-feature numbers point at a cause.
import { buildDocx, PNG_1PX, richParts, STYLES, NUMBERING, THEME, type FixtureParts } from "./docx-fixture";

const NS =
  'xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture" xmlns:w14="http://schemas.microsoft.com/office/word/2010/wordml" xmlns:w15="http://schemas.microsoft.com/office/word/2012/wordml" xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math" xmlns:wps="http://schemas.microsoft.com/office/word/2010/wordprocessingShape"';

const p = (body: string, pPr = "") => `<w:p>${pPr ? `<w:pPr>${pPr}</w:pPr>` : ""}${body}</w:p>`;
const r = (text: string, rPr = "") => `<w:r>${rPr ? `<w:rPr>${rPr}</w:rPr>` : ""}<w:t xml:space="preserve">${text}</w:t></w:r>`;
const fld = (instr: string, result: string) =>
  `<w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText xml:space="preserve"> ${instr} </w:instrText></w:r><w:r><w:fldChar w:fldCharType="separate"/></w:r>${r(result)}<w:r><w:fldChar w:fldCharType="end"/></w:r>`;
const num = (id: number, lvl: number) => `<w:numPr><w:ilvl w:val="${lvl}"/><w:numId w:val="${id}"/></w:numPr>`;

const inlinePic = (id: number, descr: string, cx = 952500, cy = 476250) =>
  `<w:r><w:drawing><wp:inline><wp:extent cx="${cx}" cy="${cy}"/><wp:docPr id="${id}" name="Picture ${id}" descr="${descr}"/><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic><pic:nvPicPr><pic:cNvPr id="0" name="p.png"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="rIdImg"/></pic:blipFill><pic:spPr/></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`;

const anchoredPic = (id: number, wrap: string, descr: string) =>
  `<w:r><w:drawing><wp:anchor distT="0" distB="0" distL="114300" distR="114300" simplePos="0" relativeHeight="251659264" behindDoc="0" locked="0" layoutInCell="1" allowOverlap="1"><wp:simplePos x="0" y="0"/><wp:positionH relativeFrom="column"><wp:posOffset>2000000</wp:posOffset></wp:positionH><wp:positionV relativeFrom="paragraph"><wp:posOffset>100000</wp:posOffset></wp:positionV><wp:extent cx="1143000" cy="762000"/><wp:effectExtent l="0" t="0" r="0" b="0"/><wp:${wrap}${wrap === "wrapSquare" ? ' wrapText="bothSides"' : ""}/><wp:docPr id="${id}" name="Picture ${id}" descr="${descr}"/><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic><pic:nvPicPr><pic:cNvPr id="0" name="p.png"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="rIdImg"/></pic:blipFill><pic:spPr/></pic:pic></a:graphicData></a:graphic></wp:anchor></w:drawing></w:r>`;

const PG = '<w:pgSz w:w="12240" w:h="15840"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440" w:header="720" w:footer="720" w:gutter="0"/>';

export interface CorpusDoc {
  name: string;
  bytes: Uint8Array;
}

// Each document carries only the parts it uses, so the style / numbering /
// media counts measure what the document needs rather than boilerplate.
const base = (body: string, more: Partial<FixtureParts> = {}): FixtureParts => {
  const uses = (re: RegExp) => re.test(body);
  return {
    body,
    styles: uses(/w:pStyle|w:rStyle|w:tblStyle/) ? STYLES : undefined,
    numbering: uses(/w:numPr/) ? NUMBERING : undefined,
    theme: THEME,
    rels: [
      ["rIdLink", "hyperlink", "https://example.com/", true],
      ["rIdImg", "image", "media/image1.png"],
    ],
    media: uses(/r:embed/) ? { "image1.png": PNG_1PX } : undefined,
    ...more,
  };
};

const hdr = (text: string, instr = "") =>
  `<w:hdr ${NS}>${p(r(text) + (instr ? fld(instr, "1") : ""))}</w:hdr>`;
const ftr = (text: string, instr = "") =>
  `<w:ftr ${NS}>${p(r(text) + (instr ? fld(instr, "1") : ""))}</w:ftr>`;

export async function generatedCorpus(): Promise<CorpusDoc[]> {
  const docs: [string, FixtureParts][] = [];

  docs.push(["rich-everything", richParts()]);

  docs.push([
    "fields",
    base(
      [
        p(fld("TOC \\o &quot;1-3&quot; \\h \\z \\u", "Contents")),
        p(r("Page ") + fld("PAGE", "1") + r(" of ") + fld("NUMPAGES", "3") + r(" sec ") + fld("SECTIONPAGES", "1")),
        p(r("Title: ") + fld("TITLE", "Doc") + r(" author ") + fld("AUTHOR", "Me") + r(" file ") + fld("FILENAME", "a.docx")),
        p(r("Ref ") + fld("REF intro \\h", "intro") + r(" pageref ") + fld("PAGEREF intro \\h", "1") + r(" seq ") + fld("SEQ Figure \\* ARABIC", "1")),
        p(r("Date ") + fld('DATE \\@ "d MMMM yyyy"', "1 May 2026") + r(" time ") + fld('TIME \\@ "HH:mm"', "10:00") + r(" save ") + fld("SAVEDATE", "x")),
        p(r("Merge ") + fld("MERGEFIELD Name", "«Name»") + fld('IF 1 = 1 "yes" "no"', "yes") + fld("NEXT", "")),
        p(r("Style ") + fld("STYLEREF &quot;Heading 1&quot;", "H") + r(" eq ") + fld("EQ \\f(1,2)", "1/2") + r(" calc ") + fld("= 2 + 3", "5")),
        p(r("Doc ") + fld("DOCPROPERTY Company", "Adrient") + fld("NUMWORDS", "9") + fld("KEYWORDS", "k")),
        p('<w:fldSimple w:instr=" PAGE \\* MERGEFORMAT ">' + r("1") + "</w:fldSimple>" + '<w:fldSimple w:instr=" HYPERLINK &quot;https://adrient.example/&quot; ">' + r("link") + "</w:fldSimple>"),
        p('<w:bookmarkStart w:id="1" w:name="intro"/>' + r("Anchor") + '<w:bookmarkEnd w:id="1"/>'),
        p(fld('MACROBUTTON NoMacro "Click"', "Click") + fld("SYMBOL 169 \\f Symbol", "©") + fld("CITATION Smith2020", "(Smith 2020)")),
      ].join(""),
    ),
  ]);

  docs.push([
    "styles-inheritance",
    base(
      [
        p(r("Heading one"), '<w:pStyle w:val="Heading1"/>'),
        p(r("Callout paragraph"), '<w:pStyle w:val="Callout"/>'),
        p(r("Big"), '<w:pStyle w:val="BigHeading"/>'),
        p(r("Marked ") + r("run", '<w:rStyle w:val="Marker"/>')),
        p(r("Title"), '<w:pStyle w:val="Titel"/>'),
      ].join(""),
      {
        styles: STYLES.replace(
          "</w:styles>",
          `<w:style w:type="paragraph" w:styleId="Quote"><w:name w:val="Quote"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:link w:val="QuoteChar"/><w:uiPriority w:val="29"/><w:qFormat/><w:pPr><w:ind w:left="864" w:right="864"/><w:jc w:val="center"/></w:pPr><w:rPr><w:i/><w:color w:val="404040"/></w:rPr></w:style>
<w:style w:type="character" w:customStyle="1" w:styleId="QuoteChar"><w:name w:val="Quote Char"/><w:basedOn w:val="DefaultParagraphFont"/><w:link w:val="Quote"/><w:rPr><w:i/></w:rPr></w:style>
<w:style w:type="table" w:styleId="Banded"><w:name w:val="Banded"/><w:basedOn w:val="TableGrid"/><w:tblPr><w:tblStyleRowBandSize w:val="1"/></w:tblPr><w:tblStylePr w:type="firstRow"><w:rPr><w:b/></w:rPr><w:tcPr><w:shd w:val="clear" w:color="auto" w:fill="D9E2F3"/></w:tcPr></w:tblStylePr><w:tblStylePr w:type="band1Horz"><w:tcPr><w:shd w:val="clear" w:color="auto" w:fill="F2F2F2"/></w:tcPr></w:tblStylePr></w:style>
</w:styles>`,
        ),
      },
    ),
  ]);

  const multiNum = base(
    [
      p(r("Chapter"), num(5, 0)),
      p(r("Section"), num(5, 1)),
      p(r("Clause"), num(5, 2)),
      p(r("Section b"), num(5, 1)),
      p(r("Roman"), num(6, 0)),
      p(r("Roman two"), num(6, 0)),
      p(r("Letter"), num(7, 0)),
      p(r("Picture bullet"), num(8, 0)),
      p(r("Restart"), num(9, 0)),
    ].join(""),
    {
      numbering: `<w:numbering ${NS}>
<w:abstractNum w:abstractNumId="10"><w:multiLevelType w:val="multilevel"/><w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1."/><w:lvlJc w:val="left"/><w:pPr><w:ind w:left="432" w:hanging="432"/></w:pPr></w:lvl><w:lvl w:ilvl="1"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1.%2"/><w:lvlJc w:val="left"/><w:pPr><w:ind w:left="576" w:hanging="576"/></w:pPr></w:lvl><w:lvl w:ilvl="2"><w:start w:val="1"/><w:numFmt w:val="lowerLetter"/><w:lvlText w:val="(%3)"/><w:lvlJc w:val="left"/><w:pPr><w:ind w:left="720" w:hanging="720"/></w:pPr><w:rPr><w:b/></w:rPr></w:lvl></w:abstractNum>
<w:abstractNum w:abstractNumId="11"><w:multiLevelType w:val="singleLevel"/><w:lvl w:ilvl="0"><w:start w:val="3"/><w:numFmt w:val="upperRoman"/><w:lvlText w:val="%1)"/><w:lvlJc w:val="right"/></w:lvl></w:abstractNum>
<w:abstractNum w:abstractNumId="12"><w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="lowerLetter"/><w:suff w:val="nothing"/><w:lvlText w:val="%1"/><w:lvlJc w:val="left"/></w:lvl></w:abstractNum>
<w:abstractNum w:abstractNumId="13"><w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="bullet"/><w:lvlText w:val="o"/><w:lvlJc w:val="left"/><w:rPr><w:rFonts w:ascii="Courier New" w:hAnsi="Courier New"/></w:rPr></w:lvl></w:abstractNum>
<w:num w:numId="5"><w:abstractNumId w:val="10"/></w:num><w:num w:numId="6"><w:abstractNumId w:val="11"/></w:num><w:num w:numId="7"><w:abstractNumId w:val="12"/></w:num><w:num w:numId="8"><w:abstractNumId w:val="13"/></w:num><w:num w:numId="9"><w:abstractNumId w:val="10"/><w:lvlOverride w:ilvl="0"><w:startOverride w:val="7"/></w:lvlOverride></w:num>
</w:numbering>`,
    },
  );
  docs.push(["numbering-multilevel", multiNum]);

  // Sections: continuous, landscape, columns, per-section headers and footers.
  const secP = (inner: string) => `<w:p><w:pPr><w:sectPr>${inner}</w:sectPr></w:pPr></w:p>`;
  docs.push([
    "sections",
    base(
      [
        p(r("Section one")),
        secP('<w:headerReference w:type="default" r:id="rIdH2"/><w:footerReference w:type="default" r:id="rIdF2"/><w:type w:val="nextPage"/>' + PG),
        p(r("Section two, landscape")),
        secP('<w:headerReference w:type="default" r:id="rIdH3"/><w:headerReference w:type="first" r:id="rIdH2"/><w:titlePg/><w:type w:val="nextPage"/><w:pgSz w:w="15840" w:h="12240" w:orient="landscape"/><w:pgMar w:top="1080" w:right="1080" w:bottom="1080" w:left="1080" w:header="500" w:footer="500" w:gutter="0"/><w:pgNumType w:fmt="lowerRoman" w:start="3"/>'),
        p(r("Section three, two columns")),
        secP('<w:type w:val="continuous"/>' + PG + '<w:cols w:num="2" w:space="720" w:sep="1"/><w:lnNumType w:countBy="1" w:restart="newPage"/>'),
        p(r("Section four with borders")),
      ].join(""),
      {
        header: hdr("Default header", "PAGE"),
        footer: ftr("Default footer"),
        extraParts: { "header2.xml": hdr("Second header"), "header3.xml": hdr("Landscape header"), "footer2.xml": ftr("Second footer", "PAGE") },
        rels: [
          ["rIdImg", "image", "media/image1.png"],
          ["rIdH2", "header", "header2.xml"],
          ["rIdH3", "header", "header3.xml"],
          ["rIdF2", "footer", "footer2.xml"],
        ],
        sectPr: PG + '<w:pgBorders w:offsetFrom="page"><w:top w:val="single" w:sz="8" w:space="24" w:color="126C5A"/></w:pgBorders><w:vAlign w:val="center"/>',
        settings: `<w:settings ${NS}><w:evenAndOddHeaders/><w:defaultTabStop w:val="720"/><w:footnotePr><w:numFmt w:val="lowerRoman"/></w:footnotePr></w:settings>`,
      },
    ),
  ]);

  docs.push([
    "notes-and-comments",
    base(
      [
        p(r("Alpha") + '<w:r><w:footnoteReference w:id="1"/></w:r>' + r(" beta") + '<w:r><w:footnoteReference w:id="3"/></w:r>' + r(" gamma") + '<w:r><w:endnoteReference w:id="2"/></w:r>'),
        p('<w:commentRangeStart w:id="0"/>' + r("one") + '<w:commentRangeEnd w:id="0"/><w:r><w:commentReference w:id="0"/></w:r>' + '<w:commentRangeStart w:id="1"/>' + r(" two") + '<w:commentRangeEnd w:id="1"/><w:r><w:commentReference w:id="1"/></w:r>'),
        p('<w:commentRangeStart w:id="2"/>' + r("third") + '<w:commentRangeEnd w:id="2"/><w:r><w:commentReference w:id="2"/></w:r>'),
      ].join(""),
      {
        footnotes: `<w:footnotes ${NS}><w:footnote w:type="separator" w:id="-1"><w:p><w:r><w:separator/></w:r></w:p></w:footnote><w:footnote w:type="continuationSeparator" w:id="0"><w:p><w:r><w:continuationSeparator/></w:r></w:p></w:footnote><w:footnote w:id="1"><w:p><w:r><w:footnoteRef/></w:r>${r(" First ")}${r("bold note", "<w:b/>")}</w:p><w:p>${r("Second paragraph of the note.")}</w:p></w:footnote><w:footnote w:id="3"><w:p><w:r><w:footnoteRef/></w:r>${r(" Another.")}</w:p></w:footnote></w:footnotes>`,
        endnotes: `<w:endnotes ${NS}><w:endnote w:id="2"><w:p><w:r><w:endnoteRef/></w:r>${r(" End.")}</w:p></w:endnote></w:endnotes>`,
        comments: `<w:comments ${NS}><w:comment w:id="0" w:author="Ann" w:initials="A" w:date="2026-02-03T04:05:06Z"><w:p w14:paraId="00000001"><w:r><w:annotationRef/></w:r>${r("Top comment.")}</w:p></w:comment><w:comment w:id="1" w:author="Bo" w:date="2026-02-04T04:05:06Z"><w:p w14:paraId="00000002">${r("A reply.")}</w:p></w:comment><w:comment w:id="2" w:author="Cy" w:date="2026-02-05T04:05:06Z"><w:p w14:paraId="00000003">${r("Resolved one.")}</w:p><w:p>${r("Two paragraphs.")}</w:p></w:comment></w:comments>`,
        commentsExtended: `<w15:commentsEx ${NS}><w15:commentEx w15:paraId="00000001" w15:done="0"/><w15:commentEx w15:paraId="00000002" w15:paraIdParent="00000001" w15:done="0"/><w15:commentEx w15:paraId="00000003" w15:done="1"/></w15:commentsEx>`,
      },
    ),
  ]);

  const rev = (kind: string, id: number, who: string, inner: string) => `<w:${kind} w:id="${id}" w:author="${who}" w:date="2026-03-04T05:06:07Z">${inner}</w:${kind}>`;
  docs.push([
    "tracked-changes",
    base(
      [
        p(r("Kept ") + rev("ins", 1, "Ann", r("inserted")) + r(" ") + rev("del", 2, "Bob", '<w:r><w:delText xml:space="preserve">deleted</w:delText></w:r>')),
        p(rev("ins", 3, "Ann", r("bold insert", "<w:b/>")) + rev("del", 4, "Ann", '<w:r><w:rPr><w:i/></w:rPr><w:delText>gone italics</w:delText></w:r>')),
        p(r("Formatted", "<w:b/>" + '<w:rPrChange w:id="5" w:author="Cy" w:date="2026-03-04T05:06:07Z"><w:rPr/></w:rPrChange>')),
        p(r("Para fmt"), '<w:jc w:val="center"/><w:pPrChange w:id="6" w:author="Cy" w:date="2026-03-04T05:06:07Z"><w:pPr/></w:pPrChange>'),
        `<w:p><w:pPr><w:rPr><w:ins w:id="7" w:author="Ann" w:date="2026-03-04T05:06:07Z"/></w:rPr></w:pPr>${rev("ins", 8, "Ann", r("Inserted paragraph"))}</w:p>`,
        `<w:p>${rev("moveFrom", 9, "Dee", r("moved text"))}</w:p><w:p>${rev("moveTo", 10, "Dee", r("moved text"))}</w:p>`,
        `<w:tbl><w:tblPr/><w:tblGrid><w:gridCol w:w="4000"/></w:tblGrid><w:tr><w:trPr><w:ins w:id="11" w:author="Ann" w:date="2026-03-04T05:06:07Z"/></w:trPr><w:tc>${p(rev("ins", 12, "Ann", r("row inserted")))}</w:tc></w:tr></w:tbl>`,
        p(""),
      ].join(""),
      { settings: `<w:settings ${NS}><w:trackRevisions/></w:settings>` },
    ),
  ]);

  docs.push([
    "images-and-wrap",
    base(
      [
        p(r("Inline ") + inlinePic(1, "Logo one") + r(" and again ") + inlinePic(2, "Logo two", 1905000, 952500)),
        p(anchoredPic(3, "wrapSquare", "Square wrapped") + r("Text that wraps the picture. ".repeat(6))),
        p(anchoredPic(4, "wrapTopAndBottom", "Top and bottom") + r("Above and below.")),
        p(anchoredPic(5, "wrapNone", "Behind text") + r("Text over the image.")),
        p(anchoredPic(6, "wrapTight", "Tight") + r("Tight wrap text.")),
      ].join(""),
    ),
  ]);

  const cell = (t: string, tcPr = "") => `<w:tc>${tcPr ? `<w:tcPr>${tcPr}</w:tcPr>` : ""}${p(r(t))}</w:tc>`;
  docs.push([
    "tables-complex",
    base(
      [
        `<w:tbl><w:tblPr><w:tblStyle w:val="TableGrid"/><w:tblW w:w="9000" w:type="dxa"/><w:jc w:val="center"/><w:tblBorders><w:top w:val="single" w:sz="12" w:space="0" w:color="126C5A"/><w:insideH w:val="dotted" w:sz="4" w:space="0" w:color="999999"/></w:tblBorders><w:tblCellMar><w:left w:w="200" w:type="dxa"/></w:tblCellMar><w:tblLook w:val="04A0"/></w:tblPr><w:tblGrid><w:gridCol w:w="3000"/><w:gridCol w:w="3000"/><w:gridCol w:w="3000"/></w:tblGrid>` +
          `<w:tr><w:trPr><w:tblHeader/><w:trHeight w:val="500" w:hRule="atLeast"/></w:trPr>${cell("A", '<w:shd w:val="clear" w:color="auto" w:fill="D9E2F3"/>')}${cell("B")}${cell("C")}</w:tr>` +
          `<w:tr>${cell("span2", '<w:tcW w:w="6000" w:type="dxa"/><w:gridSpan w:val="2"/><w:tcBorders><w:bottom w:val="double" w:sz="6" w:space="0" w:color="FF0000"/></w:tcBorders>')}${cell("v", '<w:vMerge w:val="restart"/><w:vAlign w:val="center"/>')}</w:tr>` +
          `<w:tr>${cell("x")}${cell("y", '<w:tcMar><w:top w:w="100" w:type="dxa"/></w:tcMar><w:textDirection w:val="btLr"/>')}${cell("", "<w:vMerge/>")}</w:tr></w:tbl>`,
        p(r("between")),
        // Nested table and a floating table.
        `<w:tbl><w:tblPr><w:tblpPr w:leftFromText="180" w:rightFromText="180" w:vertAnchor="text" w:horzAnchor="margin" w:tblpY="100"/><w:tblW w:w="3000" w:type="dxa"/></w:tblPr><w:tblGrid><w:gridCol w:w="3000"/></w:tblGrid><w:tr><w:tc><w:tcPr><w:tcW w:w="3000" w:type="dxa"/></w:tcPr>${p(r("outer"))}<w:tbl><w:tblPr><w:tblW w:w="2000" w:type="dxa"/></w:tblPr><w:tblGrid><w:gridCol w:w="2000"/></w:tblGrid><w:tr>${cell("inner")}</w:tr></w:tbl>${p("")}</w:tc></w:tr></w:tbl>`,
        p(r("end")),
      ].join(""),
    ),
  ]);

  docs.push([
    "content-controls",
    base(
      [
        `<w:sdt><w:sdtPr><w:alias w:val="Client"/><w:tag w:val="client"/><w:id w:val="11"/><w:text/></w:sdtPr><w:sdtContent>${p(r("Acme"))}</w:sdtContent></w:sdt>`,
        p(`<w:sdt><w:sdtPr><w:alias w:val="Pick"/><w:dropDownList><w:listItem w:displayText="One" w:value="1"/><w:listItem w:displayText="Two" w:value="2"/></w:dropDownList></w:sdtPr><w:sdtContent>${r("One")}</w:sdtContent></w:sdt>`),
        p(`<w:sdt><w:sdtPr><w14:checkbox><w14:checked w14:val="1"/></w14:checkbox></w:sdtPr><w:sdtContent>${r("☒")}</w:sdtContent></w:sdt>`),
        p(`<w:sdt><w:sdtPr><w:date><w:dateFormat w:val="d/M/yyyy"/></w:date></w:sdtPr><w:sdtContent>${r("1/5/2026")}</w:sdtContent></w:sdt>`),
      ].join(""),
    ),
  ]);

  docs.push([
    "math-and-shapes",
    base(
      [
        p(`<m:oMath><m:f><m:num><m:r><m:t>a</m:t></m:r></m:num><m:den><m:r><m:t>b</m:t></m:r></m:den></m:f></m:oMath>`),
        p(r("Inline ") + `<m:oMath><m:sSup><m:e><m:r><m:t>x</m:t></m:r></m:e><m:sup><m:r><m:t>2</m:t></m:r></m:sup></m:sSup></m:oMath>`),
        p(`<w:r><w:drawing><wp:inline><wp:extent cx="1800000" cy="900000"/><wp:docPr id="20" name="Text Box"/><a:graphic><a:graphicData uri="http://schemas.microsoft.com/office/word/2010/wordprocessingShape"><wps:wsp><wps:cNvSpPr txBox="1"/><wps:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="1800000" cy="900000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:solidFill><a:srgbClr val="FFF2CC"/></a:solidFill></wps:spPr><wps:txbx><w:txbxContent>${p(r("In a box"))}</w:txbxContent></wps:txbx><wps:bodyPr/></wps:wsp></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`),
      ].join(""),
    ),
  ]);

  docs.push([
    "formatting-matrix",
    base(
      [
        p(r("a", "<w:b/><w:i/>") + r("b", '<w:u w:val="double"/><w:strike/>') + r("c", '<w:color w:val="00AA00"/><w:sz w:val="40"/><w:szCs w:val="40"/>') + r("d", '<w:highlight w:val="cyan"/><w:caps/>') + r("e", '<w:smallCaps/><w:spacing w:val="40"/>') + r("f", '<w:rFonts w:ascii="Courier New" w:hAnsi="Courier New" w:cs="Courier New"/><w:vertAlign w:val="subscript"/>') + r("g", '<w:shd w:val="clear" w:color="auto" w:fill="FFFF00"/><w:lang w:val="fr-FR"/>') + r("h", "<w:vanish/>") + '<w:r><w:br/></w:r>' + '<w:r><w:tab/></w:r>' + '<w:r><w:sym w:font="Wingdings" w:char="F04A"/></w:r>'),
        p(r("right"), '<w:jc w:val="right"/><w:spacing w:before="200" w:after="100" w:line="480" w:lineRule="auto"/><w:ind w:left="720" w:firstLine="360"/>'),
        p(r("justified"), '<w:jc w:val="both"/><w:keepLines/><w:pageBreakBefore/><w:widowControl/>'),
        p(r("borders"), '<w:pBdr><w:top w:val="single" w:sz="6" w:space="1" w:color="000000"/><w:left w:val="single" w:sz="6" w:space="4" w:color="000000"/><w:bottom w:val="single" w:sz="6" w:space="1" w:color="000000"/><w:right w:val="single" w:sz="6" w:space="4" w:color="000000"/></w:pBdr><w:shd w:val="clear" w:color="auto" w:fill="F0F0F0"/>'),
        p(r("tabs"), '<w:tabs><w:tab w:val="left" w:pos="1440"/><w:tab w:val="center" w:leader="hyphen" w:pos="4680"/><w:tab w:val="decimal" w:pos="7000"/></w:tabs>'),
        p(r("outline"), '<w:outlineLvl w:val="2"/><w:bidi/>'),
        p(r("hello") + '<w:hyperlink r:id="rIdLink" w:tooltip="tip"><w:r><w:t>site</w:t></w:r></w:hyperlink>' + '<w:hyperlink w:anchor="top"><w:r><w:t>local</w:t></w:r></w:hyperlink>' + '<w:bookmarkStart w:id="9" w:name="top"/><w:bookmarkEnd w:id="9"/>'),
        p(r("")), p(""),
      ].join(""),
      { settings: `<w:settings ${NS}><w:defaultTabStop w:val="360"/><w:compat><w:compatSetting w:name="compatibilityMode" w:uri="http://schemas.microsoft.com/office/word" w:val="15"/></w:compat></w:settings>` },
    ),
  ]);

  const out: CorpusDoc[] = [];
  for (const [name, parts] of docs) out.push({ name, bytes: await buildDocx(parts) });
  return out;
}
