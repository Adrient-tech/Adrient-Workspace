// Business starter decks in Adrient green: pitch deck, quarterly review,
// sales proposal, project status and training. Built from a few slide kinds
// (cover, bullets, stats, table, closing) so every deck looks consistent.
import { newTable, uid, type DeckDoc, type Slide, type SlideElement } from "./model";
import type { DeckTemplate } from "./templates";

const GREEN = "#126C5A";
const INK = "#00261F";
const MINT = "#d7efe8";

function text(p: Partial<SlideElement>): SlideElement {
  return {
    id: uid(),
    type: "text",
    x: 70,
    y: 80,
    w: 820,
    h: 60,
    fontSize: 20,
    color: INK,
    align: "left",
    valign: "top",
    fontFamily: "Arial",
    text: "",
    ...p,
  };
}
function box(p: Partial<SlideElement>): SlideElement {
  return { id: uid(), type: "rect", x: 0, y: 0, w: 100, h: 100, fill: GREEN, stroke: "none", strokeWidth: 0, ...p };
}

const cover = (title: string, sub: string, notes: string): Slide => ({
  id: uid(),
  background: GREEN,
  notes,
  elements: [
    box({ x: 70, y: 196, w: 90, h: 6, fill: MINT }),
    text({ y: 214, h: 100, text: title, fontSize: 52, bold: true, color: "#ffffff" }),
    text({ y: 330, h: 50, text: sub, fontSize: 24, color: MINT }),
    text({ y: 470, h: 30, text: "Adrient Pvt Ltd", fontSize: 14, color: MINT }),
  ],
});

const bullets = (title: string, points: string[], notes = "Talking points for this slide."): Slide => ({
  id: uid(),
  background: "#ffffff",
  notes,
  elements: [
    box({ x: 0, y: 0, w: 960, h: 8 }),
    text({ y: 50, h: 70, text: title, fontSize: 34, bold: true, color: GREEN }),
    text({ y: 150, h: 330, text: points.map((p) => `• ${p}`).join("\n"), fontSize: 22, color: "#2f3b38" }),
  ],
});

const stats = (title: string, items: [string, string][], notes = "Explain what drives each number."): Slide => {
  const w = 260;
  const gap = 30;
  const x0 = Math.round((960 - (items.length * w + (items.length - 1) * gap)) / 2);
  return {
    id: uid(),
    background: "#ffffff",
    notes,
    elements: [
      box({ x: 0, y: 0, w: 960, h: 8 }),
      text({ y: 50, h: 70, text: title, fontSize: 34, bold: true, color: GREEN }),
      ...items.flatMap(([value, label], i): SlideElement[] => [
        box({ x: x0 + i * (w + gap), y: 170, w, h: 220, fill: MINT }),
        text({ x: x0 + i * (w + gap), y: 210, w, h: 80, text: value, fontSize: 48, bold: true, color: GREEN, align: "center" }),
        text({ x: x0 + i * (w + gap), y: 310, w, h: 60, text: label, fontSize: 20, color: INK, align: "center" }),
      ]),
    ],
  };
};

const tableSlide = (title: string, rows: string[][], notes = "Walk through the rows; highlight what changed."): Slide => {
  const t = newTable(rows.length, rows[0].length);
  t.cells = rows;
  return {
    id: uid(),
    background: "#ffffff",
    notes,
    elements: [
      box({ x: 0, y: 0, w: 960, h: 8 }),
      text({ y: 50, h: 70, text: title, fontSize: 34, bold: true, color: GREEN }),
      {
        id: uid(),
        type: "table",
        x: 70,
        y: 150,
        w: 820,
        h: rows.length * 44,
        table: t,
        fill: "none",
        stroke: "none",
        strokeWidth: 0,
        fontSize: 18,
        fontFamily: "Arial",
        color: INK,
      },
    ],
  };
};

const closing = (title: string, sub: string): Slide => ({
  id: uid(),
  notes: "Close with the single action you want from the audience.",
  background: INK,
  elements: [
    text({ y: 200, h: 90, text: title, fontSize: 48, bold: true, color: "#ffffff", align: "center" }),
    text({ y: 310, h: 50, text: sub, fontSize: 22, color: MINT, align: "center" }),
  ],
});

const deck = (...slides: Slide[]): DeckDoc => ({ slides });

export const BUSINESS_TEMPLATES: DeckTemplate[] = [
  {
    id: "biz-pitch",
    name: "Pitch deck",
    subtitle: "Investors",
    accent: GREEN,
    build: () =>
      deck(
        cover("Company name", "The one-line pitch", "Open with the problem in one sentence."),
        bullets("The problem", ["Who suffers, and how often", "What it costs them today", "Why existing options fall short"]),
        bullets("Our solution", ["What we built", "How it works in three steps", "Why it is better"]),
        stats("Market and traction", [["₹200 Cr", "Market size"], ["120", "Paying customers"], ["35%", "Month-on-month growth"]]),
        tableSlide("Business model", [["Plan", "Price", "Target customer"], ["Starter", "₹999 / mo", "Small teams"], ["Business", "₹2,999 / mo", "Growing companies"]]),
        bullets("The team", ["Founder, role and past experience", "Co-founder, role and past experience", "Key advisers"]),
        bullets("The ask", ["Raising: ₹—", "Use of funds: product, sales, operations", "Milestones for the next 18 months"]),
        closing("Thank you", "name@company.com"),
      ),
  },
  {
    id: "biz-quarterly",
    name: "Quarterly review",
    subtitle: "Results",
    accent: GREEN,
    build: () =>
      deck(
        cover("Q1 business review", "Results, learnings and next quarter", "Share the headline number first."),
        stats("Quarter at a glance", [["₹0.0 L", "Revenue"], ["0%", "Growth vs last quarter"], ["0", "New customers"]]),
        tableSlide("Targets vs actuals", [["Metric", "Target", "Actual", "Status"], ["Revenue", "—", "—", "On track"], ["Gross margin", "—", "—", "On track"], ["New customers", "—", "—", "At risk"]]),
        bullets("Wins", ["Biggest deal closed", "Product launched", "Process improved"]),
        bullets("Challenges", ["What slipped and why", "What we are doing about it"]),
        bullets("Next quarter priorities", ["Priority one with owner", "Priority two with owner", "Priority three with owner"]),
        closing("Questions?", "Discussion"),
      ),
  },
  {
    id: "biz-proposal",
    name: "Sales proposal",
    subtitle: "Clients",
    accent: GREEN,
    build: () =>
      deck(
        cover("Proposal for Client Name", "Prepared by your name", "Confirm the client's goals before presenting."),
        bullets("Your goals", ["What the client wants to achieve", "What success looks like", "The deadline that matters"]),
        bullets("Our approach", ["Discovery and planning", "Delivery in phases", "Support and review"]),
        tableSlide("Scope and pricing", [["Item", "Description", "Price"], ["Phase 1", "Discovery and design", "₹—"], ["Phase 2", "Build and test", "₹—"], ["Total", "", "₹—"]]),
        bullets("Timeline", ["Week 1-2: kick-off and discovery", "Week 3-6: delivery", "Week 7: handover"]),
        bullets("Why us", ["Relevant experience and references", "Clear process and reporting", "Dedicated support"]),
        closing("Next steps", "Approve the proposal to start"),
      ),
  },
  {
    id: "biz-status",
    name: "Project status",
    subtitle: "Weekly update",
    accent: GREEN,
    build: () =>
      deck(
        cover("Project status", "Week of — · Owner —", "State the overall status (green, amber or red) first."),
        stats("Health", [["Green", "Overall status"], ["62%", "Complete"], ["3", "Open risks"]]),
        tableSlide("Milestones", [["Milestone", "Owner", "Due", "Status"], ["Kick-off", "—", "—", "Done"], ["Build", "—", "—", "In progress"], ["Launch", "—", "—", "Not started"]]),
        bullets("Risks and issues", ["Risk, impact and mitigation", "Issue, owner and fix date"]),
        bullets("Decisions needed", ["Decision one", "Decision two"]),
        bullets("Next two weeks", ["Planned work item", "Planned work item", "Planned work item"]),
      ),
  },
  {
    id: "biz-training",
    name: "Training session",
    subtitle: "Teams",
    accent: GREEN,
    build: () =>
      deck(
        cover("Training title", "Facilitator · Date", "Welcome everyone and state the learning goals."),
        bullets("Agenda", ["Introduction and goals", "Core concepts", "Hands-on practice", "Q&A and next steps"]),
        bullets("Learning objectives", ["After this session you can…", "After this session you can…", "After this session you can…"]),
        bullets("Key concept", ["Definition in plain words", "Why it matters", "A short example"]),
        bullets("Exercise", ["Task description", "Time: 15 minutes", "Report back in pairs"]),
        bullets("Summary", ["Three things to remember", "Where to find resources"]),
        closing("Questions?", "Feedback form and resources"),
      ),
  },
];
