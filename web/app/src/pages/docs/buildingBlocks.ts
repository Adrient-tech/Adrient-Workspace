// Building blocks: reusable chunks of document content inserted at the
// caret (Insert ▸ Building blocks). Pure HTML so they work with the same
// insertContent path as templates.
export interface BuildingBlock {
  id: string;
  name: string;
  group: "Documents" | "Parts";
  description: string;
  html: string;
}

const date = () => new Date().toLocaleDateString(undefined, { year: "numeric", month: "long", day: "numeric" });

export function blockHtml(b: BuildingBlock): string {
  return b.html.replace(/\{date\}/g, date());
}

export const BUILDING_BLOCKS: BuildingBlock[] = [
  {
    id: "resume",
    name: "Résumé",
    group: "Documents",
    description: "Name, summary, experience, education, skills.",
    html: `<h1>Your Name</h1><p>City · email@example.com · +00 0000 000000</p>
<h2>Summary</h2><p>Two or three lines about your strengths and what you are looking for.</p>
<h2>Experience</h2><h3>Job title, Company</h3><p><em>Month Year – Present</em></p><ul><li>Achievement with a number</li><li>Responsibility you owned</li></ul>
<h2>Education</h2><p><strong>Degree</strong>, Institution, Year</p>
<h2>Skills</h2><ul><li>Skill one</li><li>Skill two</li><li>Skill three</li></ul>`,
  },
  {
    id: "letter",
    name: "Business letter",
    group: "Documents",
    description: "Sender, date, recipient, body, sign-off.",
    html: `<p>Your Name<br>Your Company<br>Street, City</p><p>{date}</p><p>Recipient Name<br>Recipient Company<br>Street, City</p>
<p>Dear [Recipient],</p><p>Opening paragraph stating the purpose of your letter.</p><p>Supporting details.</p><p>Closing paragraph with a clear next step.</p><p>Sincerely,<br><br>Your Name</p>`,
  },
  {
    id: "report",
    name: "Report",
    group: "Documents",
    description: "Title, summary, findings, recommendations.",
    html: `<h1>Report title</h1><p><em>Prepared by Your Name · {date}</em></p>
<h2>Executive summary</h2><p>The conclusion in three sentences.</p>
<h2>Background</h2><p>Why this report exists.</p>
<h2>Findings</h2><p>What you found, with evidence.</p>
<h2>Recommendations</h2><ol><li>First recommendation</li><li>Second recommendation</li></ol>
<h2>Appendix</h2><p>Supporting material.</p>`,
  },
  {
    id: "meeting-notes",
    name: "Meeting notes",
    group: "Documents",
    description: "Attendees, agenda, decisions, action items.",
    html: `<h1>Meeting notes</h1><p><strong>Date:</strong> {date}</p><p><strong>Attendees:</strong> </p>
<h2>Agenda</h2><ol><li>Topic one</li><li>Topic two</li></ol>
<h2>Decisions</h2><ul><li>Decision</li></ul>
<h2>Action items</h2><ul data-type="taskList"><li data-checked="false"><p>Owner: task, due date</p></li></ul>`,
  },
  {
    id: "cover",
    name: "Cover page",
    group: "Parts",
    description: "Title, subtitle, author and date, then a page break.",
    html: `<h1>Document title</h1><h2>Subtitle</h2><p>Author name</p><p>{date}</p><div data-page-break="true"></div>`,
  },
  {
    id: "signature",
    name: "Signature block",
    group: "Parts",
    description: "Lines for name, title and date.",
    html: `<p><br></p><p>______________________________<br>Name<br>Title<br>Date: ____________</p>`,
  },
  {
    id: "callout",
    name: "Callout / note",
    group: "Parts",
    description: "A highlighted quote-style note.",
    html: `<blockquote><p><strong>Note:</strong> Write the important point here.</p></blockquote>`,
  },
  {
    id: "pros-cons",
    name: "Pros and cons table",
    group: "Parts",
    description: "Two-column comparison table.",
    html: `<table><tbody><tr><th><p>Pros</p></th><th><p>Cons</p></th></tr><tr><td><p></p></td><td><p></p></td></tr><tr><td><p></p></td><td><p></p></td></tr></tbody></table><p></p>`,
  },
];
