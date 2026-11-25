// DOCX round-trip fidelity run (Docs wave 6).
//
//   ADRIENT_FIDELITY_CORPUS=/path/to/docx/dir ADRIENT_FIDELITY_OUT=/path/out.json \
//     npx vitest run docx-fidelity
//
// Runs docx -> Adrient -> docx over the generated corpus (always) and over the
// directory in ADRIENT_FIDELITY_CORPUS (optional), scores structure kept per
// feature (see docx/fidelity.ts) and writes the numbers to
// ADRIENT_FIDELITY_OUT as JSON. Without the corpus variable only the generated
// corpus runs and the test asserts a floor so regressions fail CI.
import { describe, expect, it } from "vitest";
import * as fs from "node:fs";
import * as nodePath from "node:path";
import { Editor } from "@tiptap/core";
import { buildExtensions } from "../extensions";
import { readDocx } from "../docx/read";
import { writeDocx } from "../docx/write";
import { applyDocxImport, collectDocxInput } from "../docx/apply";
import { aggregate, compareInventories, documentScore, inventory, readParts, type FeatureScore } from "../docx/fidelity";
import { generatedCorpus } from "./docx-fidelity-corpus";

function editorFor() {
  return new Editor({ element: document.createElement("div"), extensions: buildExtensions({ collab: false }) });
}

async function roundTrip(bytes: Uint8Array, title: string): Promise<FeatureScore[]> {
  const editor = editorFor();
  try {
    const imp = await readDocx(bytes);
    applyDocxImport(editor, imp);
    const input = collectDocxInput(editor, { title });
    input.comments = imp.comments;
    input.now = new Date(0);
    const out = await writeDocx(input);
    const dump = process.env.ADRIENT_FIDELITY_DUMP;
    if (dump) {
      fs.mkdirSync(dump, { recursive: true });
      const base = title.replace(/[^\w.-]+/g, "_").slice(-60);
      fs.writeFileSync(nodePath.join(dump, base + ".in.docx"), bytes);
      fs.writeFileSync(nodePath.join(dump, base + ".out.docx"), out);
    }
    const before = inventory(await readParts(bytes));
    const after = inventory(await readParts(out));
    return compareInventories(before, after);
  } finally {
    editor.destroy();
  }
}

function findDocx(dir: string, out: string[] = []): string[] {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = nodePath.join(dir, e.name);
    if (e.isDirectory()) findDocx(p, out);
    else if (/\.docx$/i.test(e.name) && fs.statSync(p).size < 16 << 20) out.push(p);
  }
  return out;
}

const fmt = (n: number) => (n * 100).toFixed(1) + "%";

describe("docx round-trip fidelity", () => {
  it("generated corpus keeps its structure", async () => {
    const docs = await generatedCorpus();
    const results: { name: string; scores: FeatureScore[] }[] = [];
    for (const d of docs) results.push({ name: d.name, scores: await roundTrip(d.bytes, d.name) });
    const agg = aggregate(results.map((r) => r.scores));
    const lost = agg.perFeature.filter((f) => f.ratio < 1);
    console.log(`generated corpus: ${agg.documents} docs, document mean ${fmt(agg.documentMean)}, weighted ${fmt(agg.weighted)}`);
    for (const f of lost) console.log(`  ${f.feature}: ${f.kept}/${f.before} (${fmt(f.ratio)})`);
    for (const r of results)
      console.log(`  doc ${r.name}: ${fmt(documentScore(r.scores))} lost: ${r.scores.filter((x) => x.kept < x.before).map((x) => `${x.feature} ${x.kept}/${x.before}`).join(", ")}`);
    const out = process.env.ADRIENT_FIDELITY_OUT;
    if (out) fs.writeFileSync(out.replace(/\.json$/, "") + ".generated.json", JSON.stringify({ agg, docs: results.map((r) => ({ name: r.name, score: documentScore(r.scores) })) }, null, 1));
    expect(agg.weighted).toBeGreaterThan(0.9);
  }, 300_000);

  const root = process.env.ADRIENT_FIDELITY_CORPUS;
  it.skipIf(!root)("external corpus", async () => {
    // Every STRIDE-th file (default all): the full set takes a long time.
    const stride = Number(process.env.ADRIENT_FIDELITY_STRIDE || 1);
    const files = findDocx(root!).sort().filter((_, i) => i % stride === 0);
    const progress = process.env.ADRIENT_FIDELITY_OUT ? process.env.ADRIENT_FIDELITY_OUT + ".progress" : null;
    const results: { name: string; scores: FeatureScore[] }[] = [];
    const failed: string[] = [];
    for (const f of files) {
      if (progress) fs.appendFileSync(progress, `${results.length + failed.length}/${files.length} ${nodePath.basename(f)}\n`);
      try {
        results.push({ name: nodePath.relative(root!, f), scores: await roundTrip(new Uint8Array(fs.readFileSync(f)), f) });
      } catch (e) {
        failed.push(`${nodePath.relative(root!, f)}: ${String((e as Error).message).split("\n")[0].slice(0, 160)}`);
      }
    }
    const agg = aggregate(results.map((r) => r.scores));
    console.log(`external corpus: ${files.length} files, ${results.length} round-tripped, ${failed.length} failed; document mean ${fmt(agg.documentMean)}, weighted ${fmt(agg.weighted)}`);
    for (const f of agg.perFeature.filter((x) => x.ratio < 1)) console.log(`  ${f.feature}: ${f.kept}/${f.before} (${fmt(f.ratio)})`);
    const out = process.env.ADRIENT_FIDELITY_OUT;
    if (out) fs.writeFileSync(out, JSON.stringify({ files: files.length, failed, agg, docs: results.map((r) => ({ name: r.name, score: documentScore(r.scores) })) }, null, 1));
  }, 1_800_000);
});
