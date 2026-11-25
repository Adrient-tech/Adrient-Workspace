export interface DocTemplate {
  id: string;
  name: string;
  subtitle: string;
  /** Seed HTML inserted into the new document (empty for a blank doc). */
  html: string;
}

// Built-in templates shown in the "Start a new document" gallery. Selecting one
// creates a new document seeded with its HTML (via the editor's seed handoff).
export const TEMPLATES: DocTemplate[] = [
  { id: "blank", name: "Blank document", subtitle: "", html: "" },
  {
    id: "project-proposal",
    name: "Project proposal",
    subtitle: "Tropic",
    html: `<h1>Project Name</h1><p><em>Prepared by — Your Name · ${"{date}"}</em></p>
<h2>Overview</h2><p>Briefly describe the project, its goals, and why it matters.</p>
<h2>Objectives</h2><ul><li>Objective one</li><li>Objective two</li><li>Objective three</li></ul>
<h2>Scope</h2><p>What is in scope, and what is explicitly out of scope.</p>
<h2>Timeline</h2><p>Key milestones and target dates.</p>
<h2>Budget</h2><p>Estimated cost and resourcing.</p>`,
  },
  {
    id: "meeting-notes",
    name: "Meeting notes",
    subtitle: "Modern Writer",
    html: `<h1>Meeting notes</h1><p><strong>Date:</strong> ${"{date}"} &nbsp; <strong>Attendees:</strong> </p>
<h2>Agenda</h2><ul><li>Item one</li><li>Item two</li></ul>
<h2>Notes</h2><p>Discussion summary…</p>
<h2>Action items</h2><ul data-type="taskList"><li data-checked="false">Owner — task</li></ul>`,
  },
  {
    id: "brochure",
    name: "Brochure",
    subtitle: "Geometric",
    html: `<h1>Your Company</h1><h2>Product Brochure</h2>
<p>A short, punchy intro to your product and the problem it solves.</p>
<h2>Features</h2><ul><li>Feature one</li><li>Feature two</li><li>Feature three</li></ul>
<h2>Get in touch</h2><p>hello@yourcompany.com · yourcompany.com</p>`,
  },
  {
    id: "newsletter",
    name: "Newsletter",
    subtitle: "Lively",
    html: `<h1>We have a surprise!</h1><p><em>Your monthly update</em></p>
<h2>What's new</h2><p>Lead story…</p>
<h2>Highlights</h2><ul><li>Highlight one</li><li>Highlight two</li></ul>
<p>Thanks for reading — see you next month.</p>`,
  },
  {
    id: "letter",
    name: "Business letter",
    subtitle: "Geometric",
    html: `<p>Your Name<br>Your Company<br>${"{date}"}</p>
<p>Dear [Recipient],</p>
<p>Opening paragraph stating the purpose of your letter.</p>
<p>Body paragraph with supporting details.</p>
<p>Closing paragraph with a call to action.</p>
<p>Sincerely,<br>Your Name</p>`,
  },
  {
    id: "quotation-cover",
    name: "Quotation cover letter",
    subtitle: "Adrient",
    html: `<h1><span style="color: #126C5A">Quotation</span></h1><p>${"{date}"}</p>
<p>[Client name]<br>[Client company]<br>[Address]</p>
<p><strong>Ref:</strong> Quotation [QT-0000]</p>
<p>Dear [Client name],</p>
<p>Thank you for your enquiry. Please find our quotation attached for the items and services discussed.</p>
<table><tbody><tr><th><p>Summary</p></th><th><p>Details</p></th></tr><tr><td><p>Quotation no.</p></td><td><p>[QT-0000]</p></td></tr><tr><td><p>Valid until</p></td><td><p>[Date]</p></td></tr><tr><td><p>Total</p></td><td><p>[Amount]</p></td></tr><tr><td><p>Payment terms</p></td><td><p>[Terms]</p></td></tr></tbody></table><p></p>
<p>We would be glad to answer any questions. To accept, please reply to this letter or sign and return the quotation.</p>
<p>Yours sincerely,<br>[Your name]<br>[Title], Adrient Pvt Ltd</p>`,
  },
  {
    id: "meeting-minutes",
    name: "Meeting minutes",
    subtitle: "Adrient",
    html: `<h1><span style="color: #126C5A">Minutes of meeting</span></h1><p><strong>Date:</strong> ${"{date}"} &nbsp; <strong>Time:</strong> [Start – End] &nbsp; <strong>Location / link:</strong> [Room or Meet link]</p>
<p><strong>Chair:</strong> [Name] &nbsp; <strong>Minutes by:</strong> [Name]</p>
<h2>Attendees</h2><ul><li>[Name, role]</li><li>[Name, role]</li></ul>
<h2>Apologies</h2><p>[Names]</p>
<h2>Agenda and discussion</h2><ol><li><p>[Topic] — summary of discussion and decisions.</p></li><li><p>[Topic] — summary of discussion and decisions.</p></li></ol>
<h2>Decisions</h2><ul><li>[Decision]</li></ul>
<h2>Action items</h2><table><tbody><tr><th><p>Action</p></th><th><p>Owner</p></th><th><p>Due</p></th></tr><tr><td><p></p></td><td><p></p></td><td><p></p></td></tr></tbody></table><p></p>
<p><strong>Next meeting:</strong> [Date and time]</p>`,
  },
  {
    id: "business-proposal",
    name: "Business proposal",
    subtitle: "Adrient",
    html: `<h1><span style="color: #126C5A">Business proposal</span></h1><p><em>Prepared for [Client] by Adrient Pvt Ltd · ${"{date}"}</em></p>
<h2>1. Executive summary</h2><p>One paragraph: the client's need, our solution and the outcome.</p>
<h2>2. Understanding of your requirements</h2><p>What we heard from you.</p>
<h2>3. Proposed solution</h2><p>Scope, deliverables and approach.</p>
<h2>4. Timeline</h2><table><tbody><tr><th><p>Phase</p></th><th><p>Deliverable</p></th><th><p>Date</p></th></tr><tr><td><p></p></td><td><p></p></td><td><p></p></td></tr></tbody></table><p></p>
<h2>5. Investment</h2><p>Pricing, payment schedule and validity.</p>
<h2>6. Terms</h2><p>Key terms and assumptions.</p>
<h2>7. Next steps</h2><p>How to accept and start.</p>`,
  },
  {
    id: "sop",
    name: "Standard operating procedure",
    subtitle: "Adrient",
    html: `<h1><span style="color: #126C5A">Standard operating procedure</span></h1><table><tbody><tr><th><p>Document no.</p></th><th><p>Version</p></th><th><p>Owner</p></th><th><p>Effective</p></th></tr><tr><td><p>SOP-000</p></td><td><p>1.0</p></td><td><p>[Name]</p></td><td><p>${"{date}"}</p></td></tr></tbody></table><p></p>
<h2>1. Purpose</h2><p>Why this procedure exists.</p>
<h2>2. Scope</h2><p>Who and what it applies to.</p>
<h2>3. Responsibilities</h2><ul><li>[Role] — [responsibility]</li></ul>
<h2>4. Procedure</h2><ol><li><p>Step one.</p></li><li><p>Step two.</p></li><li><p>Step three.</p></li></ol>
<h2>5. Records and forms</h2><p>Where records are kept.</p>
<h2>6. Revision history</h2><table><tbody><tr><th><p>Version</p></th><th><p>Date</p></th><th><p>Change</p></th></tr><tr><td><p>1.0</p></td><td><p>${"{date}"}</p></td><td><p>First issue</p></td></tr></tbody></table><p></p>`,
  },
  {
    id: "offer-letter",
    name: "Offer letter",
    subtitle: "Adrient",
    html: `<h1><span style="color: #126C5A">Offer of employment</span></h1><p>${"{date}"}</p>
<p>[Candidate name]<br>[Address]</p>
<p>Dear [Candidate name],</p>
<p>We are pleased to offer you the position of <strong>[Job title]</strong> at Adrient Pvt Ltd, reporting to [Manager]. Your start date will be [Date].</p>
<table><tbody><tr><th><p>Term</p></th><th><p>Details</p></th></tr><tr><td><p>Compensation</p></td><td><p>[Amount] per [year/month]</p></td></tr><tr><td><p>Location</p></td><td><p>[Location]</p></td></tr><tr><td><p>Probation</p></td><td><p>[Months]</p></td></tr><tr><td><p>Benefits</p></td><td><p>[Summary]</p></td></tr></tbody></table><p></p>
<p>This offer is subject to [conditions]. Please sign and return a copy by [Date] to accept.</p>
<p>Sincerely,<br>[Name]<br>[Title], Adrient Pvt Ltd</p>
<p>Accepted: ____________________ &nbsp; Date: ______________</p>`,
  },
  {
    id: "invoice-cover",
    name: "Invoice cover note",
    subtitle: "Adrient",
    html: `<h1><span style="color: #126C5A">Invoice cover note</span></h1><p>${"{date}"}</p>
<p>[Client name]<br>[Client company]</p>
<p>Dear [Client name],</p>
<p>Please find attached invoice <strong>[INV-0000]</strong> for [description of work or goods].</p>
<table><tbody><tr><th><p>Invoice</p></th><th><p>Amount due</p></th><th><p>Due date</p></th></tr><tr><td><p>[INV-0000]</p></td><td><p>[Amount]</p></td><td><p>[Date]</p></td></tr></tbody></table><p></p>
<p><strong>Payment details:</strong> [Bank name, account number, IFSC / UPI]. Please quote the invoice number as the payment reference.</p>
<p>Thank you for your business.</p>
<p>Regards,<br>[Your name]<br>Accounts, Adrient Pvt Ltd</p>`,
  },
  // Report, Resume and Essay starters.
  {
    id: "report",
    name: "Report",
    subtitle: "Summary and numbered sections",
    html: `<h1>[Report title]</h1><p><em>[Subtitle or author] · ${"{date}"}</em></p>
<h1>Summary</h1><p>[One paragraph that states the question, the finding, and the recommendation.]</p>
<h1>Background</h1><p>[Why this matters and what was already known.]</p>
<h1>Findings</h1><h2>[First finding]</h2><p>[Evidence and discussion.]</p><h2>[Second finding]</h2><p>[Evidence and discussion.]</p>
<h1>Recommendations</h1><ol><li>[First recommendation]</li><li>[Second recommendation]</li></ol>
<h1>Appendix</h1><p>[Sources, data, and methods.]</p>`,
  },
  {
    id: "resume",
    name: "Resume",
    subtitle: "Experience, education, skills",
    html: `<h1>[Your name]</h1><p>[City] · [Phone] · [Email] · [Website]</p>
<h2>Summary</h2><p>[Two or three sentences on what you do and what you are looking for.]</p>
<h2>Experience</h2><h3>[Job title], [Company] — [Start] to [End]</h3><ul><li>[Achievement with a number]</li><li>[Responsibility that shows scope]</li><li>[Skill or tool used]</li></ul>
<h3>[Job title], [Company] — [Start] to [End]</h3><ul><li>[Achievement with a number]</li><li>[Responsibility that shows scope]</li></ul>
<h2>Education</h2><p>[Degree], [School] — [Year]</p>
<h2>Skills</h2><p>[Skill] · [Skill] · [Skill] · [Skill]</p>`,
  },
  {
    id: "essay",
    name: "Essay",
    subtitle: "Thesis, body, conclusion",
    html: `<h1>[Essay title]</h1><p><em>[Your name] · [Course] · ${"{date}"}</em></p>
<p>[Introduction: set the scene and end with the thesis in one sentence.]</p>
<p>[First body paragraph: one claim, the evidence for it, and why it matters.]</p>
<p>[Second body paragraph.]</p><p>[Third body paragraph.]</p>
<p>[Conclusion: restate the thesis in new words and say what follows from it.]</p>
<h2>References</h2><p>[Author, Title, Year.]</p>`,
  },
];

/** templateHtml returns a template's HTML with placeholders filled in. */
export function templateHtml(t: DocTemplate): string {
  return t.html.replaceAll("{date}", new Date().toLocaleDateString());
}
