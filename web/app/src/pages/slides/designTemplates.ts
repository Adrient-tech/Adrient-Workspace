// Designed starter decks: report, roadmap, portfolio, product launch,
// marketing plan, company profile, case study and all-hands. Each deck has its
// own palette and slide kinds (cover, agenda, bullets, stats, timeline,
// two-column, quote, closing), so they look different, not recoloured.
import { uid, type DeckDoc, type Slide, type SlideElement } from "./model";
import type { DeckTemplate } from "./templates";

interface Pal {
  bg: string; // content slide background
  ink: string; // body text
  head: string; // headings
  accent: string;
  soft: string; // card background
  dark: string; // cover/closing background
  onDark: string;
  font: string;
}

const t = (p: Partial<SlideElement>, pal: Pal): SlideElement => ({
  id: uid(), type: "text", x: 64, y: 60, w: 832, h: 60, fontSize: 20, color: pal.ink, align: "left", valign: "top", fontFamily: pal.font, text: "", ...p,
});
const r = (p: Partial<SlideElement>): SlideElement => ({
  id: uid(), type: "rect", x: 0, y: 0, w: 100, h: 100, fill: "#000", stroke: "none", strokeWidth: 0, ...p,
});
const e = (p: Partial<SlideElement>): SlideElement => ({
  id: uid(), type: "ellipse", x: 0, y: 0, w: 100, h: 100, fill: "#000", stroke: "none", strokeWidth: 0, ...p,
});
const sl = (background: string, elements: SlideElement[], notes?: string): Slide => ({ id: uid(), background, elements, ...(notes ? { notes } : {}) });

type Cover = "band" | "split" | "circle" | "full";

function cover(pal: Pal, style: Cover, title: string, sub: string, notes: string): Slide {
  const els: SlideElement[] = [];
  if (style === "band") els.push(r({ x: 0, y: 380, w: 960, h: 160, fill: pal.accent }));
  if (style === "split") els.push(r({ x: 600, y: 0, w: 360, h: 540, fill: pal.accent }), e({ x: 660, y: 330, w: 240, h: 240, fill: pal.soft, }));
  if (style === "circle") els.push(e({ x: 640, y: -120, w: 420, h: 420, fill: pal.accent }), e({ x: 760, y: 300, w: 300, h: 300, fill: pal.soft }));
  if (style === "full") els.push(r({ x: 64, y: 190, w: 80, h: 6, fill: pal.accent }));
  const w = style === "split" ? 500 : 760;
  els.push(
    t({ x: 64, y: style === "band" ? 130 : 200, w, h: 150, text: title, fontSize: 48, bold: true, color: pal.onDark, valign: "bottom" }, pal),
    t({ x: 64, y: style === "band" ? 290 : 300, w, h: 60, text: sub, fontSize: 22, color: style === "band" ? pal.onDark : pal.soft }, pal),
  );
  if (style === "band") els.push(t({ x: 64, y: 430, w: 600, h: 40, text: "Presenter name · Date", fontSize: 18, color: pal.dark }, pal));
  return sl(pal.dark, els, notes);
}

const heading = (pal: Pal, text: string): SlideElement[] => [
  r({ x: 0, y: 0, w: 14, h: 540, fill: pal.accent }),
  t({ x: 56, y: 40, w: 840, h: 64, text, fontSize: 32, bold: true, color: pal.head }, pal),
];

function bullets(pal: Pal, title: string, points: string[], notes = "Speak to each point in turn."): Slide {
  return sl(pal.bg, [...heading(pal, title), t({ x: 56, y: 130, w: 840, h: 340, text: points.map((x) => `• ${x}`).join("\n"), fontSize: 22 }, pal)], notes);
}

function stats(pal: Pal, title: string, items: [string, string][], notes = "Lead with the largest number."): Slide {
  const w = Math.min(260, (840 - (items.length - 1) * 24) / items.length);
  const x0 = 56 + (840 - (items.length * w + (items.length - 1) * 24)) / 2;
  return sl(pal.bg, [
    ...heading(pal, title),
    ...items.flatMap(([v, l], i) => [
      r({ x: x0 + i * (w + 24), y: 160, w, h: 230, fill: pal.soft }),
      r({ x: x0 + i * (w + 24), y: 160, w, h: 8, fill: pal.accent }),
      t({ x: x0 + i * (w + 24), y: 210, w, h: 80, text: v, fontSize: 46, bold: true, color: pal.accent, align: "center" }, pal),
      t({ x: x0 + i * (w + 24) + 12, y: 305, w: w - 24, h: 70, text: l, fontSize: 18, align: "center" }, pal),
    ]),
  ], notes);
}

function timeline(pal: Pal, title: string, cols: [string, string[]][], notes = "Walk left to right; call out dependencies."): Slide {
  const w = (840 - (cols.length - 1) * 16) / cols.length;
  return sl(pal.bg, [
    ...heading(pal, title),
    r({ x: 56, y: 150, w: 840, h: 4, fill: pal.accent }),
    ...cols.flatMap(([h, items], i) => {
      const x = 56 + i * (w + 16);
      return [
        e({ x: x + w / 2 - 9, y: 143, w: 18, h: 18, fill: pal.accent }),
        t({ x, y: 172, w, h: 36, text: h, fontSize: 20, bold: true, color: pal.head, align: "center" }, pal),
        r({ x, y: 218, w, h: 250, fill: pal.soft }),
        t({ x: x + 8, y: 228, w: w - 16, h: 230, text: items.map((x) => `• ${x}`).join("\n"), fontSize: 16 }, pal),
      ];
    }),
  ], notes);
}

function twoCol(pal: Pal, title: string, a: [string, string[]], b: [string, string[]], notes = "Compare the two sides."): Slide {
  const col = (x: number, [h, items]: [string, string[]]): SlideElement[] => [
    r({ x, y: 130, w: 404, h: 44, fill: pal.accent }),
    t({ x: x + 12, y: 136, w: 380, h: 34, text: h, fontSize: 20, bold: true, color: pal.onDark, valign: "middle" }, pal),
    t({ x, y: 190, w: 404, h: 280, text: items.map((i) => `• ${i}`).join("\n"), fontSize: 20 }, pal),
  ];
  return sl(pal.bg, [...heading(pal, title), ...col(56, a), ...col(492, b)], notes);
}

function quote(pal: Pal, q: string, who: string): Slide {
  return sl(pal.soft, [
    t({ x: 80, y: 60, w: 120, h: 140, text: "“", fontSize: 140, bold: true, color: pal.accent }, pal),
    t({ x: 120, y: 190, w: 720, h: 170, text: q, fontSize: 32, italic: true, color: pal.head }, pal),
    t({ x: 120, y: 380, w: 720, h: 40, text: `— ${who}`, fontSize: 20, color: pal.ink }, pal),
  ], "Let the quote land before moving on.");
}

function closing(pal: Pal, title: string, sub: string): Slide {
  return sl(pal.dark, [
    r({ x: 380, y: 250, w: 200, h: 5, fill: pal.accent }),
    t({ x: 80, y: 130, w: 800, h: 110, text: title, fontSize: 50, bold: true, color: pal.onDark, align: "center", valign: "bottom" }, pal),
    t({ x: 80, y: 280, w: 800, h: 60, text: sub, fontSize: 22, color: pal.soft, align: "center" }, pal),
  ], "End with the single next step.");
}

const mk = (id: string, name: string, subtitle: string, accent: string, build: () => DeckDoc): DeckTemplate => ({ id, name, subtitle, accent, build });

const P = {
  report: { bg: "#ffffff", ink: "#2b2f36", head: "#12355b", accent: "#1f6feb", soft: "#e8f0fe", dark: "#12355b", onDark: "#ffffff", font: "Arial" },
  roadmap: { bg: "#fafafa", ink: "#2d2a32", head: "#4a1d96", accent: "#7c3aed", soft: "#ede9fe", dark: "#2e1065", onDark: "#ffffff", font: "Verdana" },
  portfolio: { bg: "#ffffff", ink: "#222222", head: "#111111", accent: "#e11d48", soft: "#f4f4f5", dark: "#111111", onDark: "#ffffff", font: "Georgia" },
  launch: { bg: "#0f172a", ink: "#cbd5e1", head: "#ffffff", accent: "#22d3ee", soft: "#1e293b", dark: "#020617", onDark: "#ffffff", font: "Arial" },
  marketing: { bg: "#fffaf0", ink: "#3b2f2f", head: "#9a3412", accent: "#f97316", soft: "#ffedd5", dark: "#7c2d12", onDark: "#ffffff", font: "Trebuchet MS" },
  profile: { bg: "#ffffff", ink: "#1f2937", head: "#064e3b", accent: "#059669", soft: "#d1fae5", dark: "#064e3b", onDark: "#ffffff", font: "Georgia" },
  casestudy: { bg: "#ffffff", ink: "#27272a", head: "#0f766e", accent: "#14b8a6", soft: "#ccfbf1", dark: "#134e4a", onDark: "#ffffff", font: "Verdana" },
  allhands: { bg: "#ffffff", ink: "#292524", head: "#1c1917", accent: "#d97706", soft: "#fef3c7", dark: "#1c1917", onDark: "#ffffff", font: "Arial" },
} satisfies Record<string, Pal>;

export const DESIGN_TEMPLATES: DeckTemplate[] = [
  mk("d-report", "Annual report", "Results", P.report.accent, () => ({ slides: [
    cover(P.report, "band", "Annual report", "Performance, priorities and outlook", "Open with the year in one sentence."),
    stats(P.report, "The year in numbers", [["₹0.0 Cr", "Revenue"], ["0%", "Growth"], ["0", "Customers"], ["0%", "Retention"]]),
    bullets(P.report, "Highlights", ["Biggest win of the year", "Product and market milestones", "Team and culture"]),
    twoCol(P.report, "Targets vs actuals", ["Targets", ["Revenue goal", "Margin goal", "Hiring plan"]], ["Actuals", ["Revenue result", "Margin result", "Hires made"]]),
    timeline(P.report, "Next year", [["Q1", ["Plan"]], ["Q2", ["Launch"]], ["Q3", ["Scale"]], ["Q4", ["Review"]]]),
    closing(P.report, "Thank you", "Questions and discussion"),
  ] })),
  mk("d-roadmap", "Product roadmap", "Planning", P.roadmap.accent, () => ({ slides: [
    cover(P.roadmap, "circle", "Product roadmap", "What we will build and when", "Explain how priorities were chosen."),
    bullets(P.roadmap, "Goals and principles", ["Customer problems first", "Ship small, learn fast", "Measure every release"]),
    timeline(P.roadmap, "Now · Next · Later", [["Now", ["In progress", "Bug fixes"]], ["Next", ["Planned", "Designing"]], ["Later", ["Ideas", "Research"]]]),
    timeline(P.roadmap, "Quarterly plan", [["Q1", ["Feature A", "Feature B"]], ["Q2", ["Feature C"]], ["Q3", ["Feature D", "Feature E"]], ["Q4", ["Feature F"]]]),
    twoCol(P.roadmap, "Risks and dependencies", ["Risks", ["Capacity", "Scope creep"]], ["Mitigation", ["Weekly review", "Clear owners"]]),
    closing(P.roadmap, "Feedback welcome", "Tell us what matters most"),
  ] })),
  mk("d-portfolio", "Portfolio", "Creative", P.portfolio.accent, () => ({ slides: [
    cover(P.portfolio, "full", "Name Surname", "Designer · Portfolio 2026", "Introduce yourself in two sentences."),
    bullets(P.portfolio, "About me", ["Background and skills", "What I care about", "How I work"]),
    twoCol(P.portfolio, "Selected project", ["The brief", ["Client and goal", "Constraints"]], ["The outcome", ["What I delivered", "Result in numbers"]]),
    quote(P.portfolio, "A short testimonial from a client goes here.", "Client name, Company"),
    stats(P.portfolio, "By the numbers", [["50+", "Projects"], ["12", "Years"], ["30", "Happy clients"]]),
    closing(P.portfolio, "Let’s work together", "hello@example.com"),
  ] })),
  mk("d-launch", "Product launch", "Dark · Tech", P.launch.accent, () => ({ slides: [
    cover(P.launch, "split", "Introducing Product", "Built for teams that move fast", "Start with the one-line promise."),
    bullets(P.launch, "The problem", ["What slows teams down today", "Why current tools fall short"]),
    bullets(P.launch, "The solution", ["Three things it does well", "How it feels to use"]),
    stats(P.launch, "Early results", [["3×", "Faster"], ["98%", "Satisfaction"], ["10k", "Waitlist"]]),
    timeline(P.launch, "Launch plan", [["Week 1", ["Beta"]], ["Week 4", ["Public launch"]], ["Week 8", ["Integrations"]]]),
    closing(P.launch, "Available today", "product.example.com"),
  ] })),
  mk("d-marketing", "Marketing plan", "Campaign", P.marketing.accent, () => ({ slides: [
    cover(P.marketing, "band", "Marketing plan", "Campaign strategy for the next quarter", "State the single goal of the campaign."),
    bullets(P.marketing, "Audience and positioning", ["Who we talk to", "What we promise", "Why they believe us"]),
    twoCol(P.marketing, "Channels", ["Owned", ["Website", "Email", "Social"]], ["Paid", ["Search", "Social ads", "Partners"]]),
    stats(P.marketing, "Targets", [["50k", "Reach"], ["2.5%", "Conversion"], ["₹0", "Budget"]]),
    timeline(P.marketing, "Campaign calendar", [["Week 1-2", ["Teasers"]], ["Week 3-4", ["Launch"]], ["Week 5-8", ["Sustain"]], ["Week 9", ["Measure"]]]),
    closing(P.marketing, "Let’s launch", "Approvals needed this week"),
  ] })),
  mk("d-profile", "Company profile", "Corporate", P.profile.accent, () => ({ slides: [
    cover(P.profile, "split", "Company name", "Who we are and what we do", "Keep the intro under a minute."),
    bullets(P.profile, "Our story", ["Founded in year, by whom", "Where we are today", "Where we are going"]),
    stats(P.profile, "At a glance", [["2015", "Founded"], ["120", "People"], ["8", "Offices"]]),
    bullets(P.profile, "What we offer", ["Product or service one", "Product or service two", "Support and training"]),
    quote(P.profile, "Customers describe us in their own words here.", "Customer name"),
    closing(P.profile, "Get in touch", "contact@example.com · +91 00000 00000"),
  ] })),
  mk("d-case", "Case study", "Clients", P.casestudy.accent, () => ({ slides: [
    cover(P.casestudy, "circle", "Case study", "How Client achieved a result with us", "Name the headline result first."),
    twoCol(P.casestudy, "Challenge and approach", ["Challenge", ["The situation", "What was at stake"]], ["Approach", ["What we did", "Why it worked"]]),
    stats(P.casestudy, "Results", [["+45%", "Revenue"], ["-30%", "Cost"], ["6 wks", "Time to value"]]),
    quote(P.casestudy, "The client’s words about working with us.", "Name, Title"),
    closing(P.casestudy, "Want similar results?", "Book a call"),
  ] })),
  mk("d-allhands", "All-hands update", "Team", P.allhands.accent, () => ({ slides: [
    cover(P.allhands, "full", "All-hands", "Company update · Month Year", "Welcome and set the agenda."),
    bullets(P.allhands, "Agenda", ["Where we are", "Wins and thanks", "What is next", "Q&A"]),
    stats(P.allhands, "Where we are", [["0%", "Of target"], ["0", "Launches"], ["0", "New joiners"]]),
    bullets(P.allhands, "Wins and thanks", ["Team or person and what they did", "Customer win", "Milestone"]),
    timeline(P.allhands, "What is next", [["This month", ["Priority"]], ["Next month", ["Priority"]], ["This quarter", ["Priority"]]]),
    closing(P.allhands, "Questions?", "Ask anything"),
  ] })),
];
