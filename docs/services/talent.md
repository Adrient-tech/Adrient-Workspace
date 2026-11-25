# Talent

Route `/talent`, module id `talent`, accent `#126C5A`, migrations `0160` to `0163`, Go package `internal/talent`, web `web/app/src/pages/talent`.
Talent is the people half of the HRMS: recruitment (ATS), onboarding and offboarding, performance, learning and policies, announcements and recognition.
It does not depend on the HR core (`internal/hr`). People are referenced by org user id only, and Talent keeps its own tables.

## Access model (enforced on the server)
- **HR** = org admins/owners plus users listed under Settings > HR users. HR sees everything below.
- **Hiring team of an opening** = recruiter, hiring manager, creator and the interviewers picked on the opening. Only they (and HR) can see that opening's applicants, resumes and scorecards. Everyone else gets "not found", also for the resume download.
- Recruiter, hiring manager, creator and HR **lead** an opening (move, reject, hire, schedule, offer). Interviewers can read candidates and write a scorecard, and see other interviewers' scorecards only after submitting their own.
- A non-HR user needs HR approval before an opening can be published. HR can publish directly.
- Talent pool, hiring reports, new-hire record, candidate erasure, settings, review cycles, calibration, checklist templates: HR only.
- Performance: a manager (direct or above) sees reviews of their reports with peers anonymised; the subject sees their own self review at any time and the manager review, peer feedback and final rating only after HR **publishes** the cycle; peers see nothing of the result. Calibration is HR only. 1:1 notes are private to the manager and the report (HR cannot read them). Feedback answers go to the person who asked and to HR.
- Goals have visibility private (owner, manager chain, HR), team (plus people with the same manager) or company.
- Exit interviews are HR only (the manager never receives them).

## Links to other apps
- **Calendar + Meet**: scheduling an interview creates a Calendar event (attendees: candidate and interviewers) with a Meet room (`calendar.Repository`, `meet.Repository`, `eventmeet`) and mails the candidate. If no mail transport is configured the interview is still created and the UI says the candidate was not emailed.
- **Drive**: resumes are stored in the blob store and registered as Drive files in a "Recruitment resumes" folder owned by the opening's recruiter. Downloads go through Talent so the hiring-team check applies.
- **Tasks**: checklist tasks are created as Tasks items (list "HR tasks") for each assignee, and ticking the checklist task completes the Tasks item.
- **Mail**: `email.Sender` (Resend or SMTP). Without a configured transport, offers are marked sent and the recruiter gets the e-accept link to share by hand.
- **Admin**: the offboarding access-removal task links to `/admin/users?q=<email>` (there is no dedicated offboarding page in Admin; the Users page is where access is blocked).
- **Policies** live in Docs or Drive: Talent stores the link and the acknowledgements.
- **Directory**: departments and managers come from `adrient.users` (`manager_id`, `department_id`, `start_date` for anniversaries).

## Public surface (no sign-in)
- `/careers/<org slug>`: career page with open jobs and the application form. `/careers/offer/<token>`: offer review and e-accept.
- API: `GET /api/v1/public/talent/{org}/jobs[/{id}]`, `POST .../jobs/{id}/apply` (multipart), `GET/POST /api/v1/public/talent/offers/{token}[/pdf|/respond]`.
- Abuse limits: 5 applications per IP per hour, 3 per email per day, hidden honeypot field, minimum fill time, consent required, resume limited to 5 MB and checked by extension and file signature (PDF, DOCX/ODT, DOC, RTF, TXT). A repeat application with the same email looks like success and stores nothing twice (no email enumeration).

## Recruitment
Openings (draft/open/closed, approval none/pending/approved/rejected), editable pipeline stages (2 to 10, Hired and Rejected are fixed), drag or "Move to" on the board, candidate de-duplication by email (one candidate record across openings), source tracking, scorecards (1 to 5 plus recommendation), offer letter PDF from a template (`{{name}} {{job_title}} {{department}} {{salary}} {{start_date}} {{company}} {{sender}}`), e-accept with a typed name, talent pool, GDPR-style erasure, reports (time to hire, funnel, source quality).
Hiring the last open position closes the opening.

### New-hire hand-off to HR core
`GET /api/v1/talent/applications/{id}/new-hire` (HR only; the application must be hired or have an accepted offer) returns `{name, email, phone, job_title, department, start_date, salary_offer, currency, manager_id, application_id, candidate_id}`. Go: `(*talent.Repository).ConvertToNewHire(ctx, actor, applicationID) (talent.NewHire, error)`. `POST /api/v1/bridge/talent/hire {application_id, create_project?}` (HR admins) does the hand-off, see "Hire into HR" below.

## Onboarding / offboarding
Templates with tasks (role hr/it/manager/employee, due offset in days from the start or last day), checklists per person, tasks created in Tasks, "my open tasks", overdue reminders by email (HR button, once a day per task), asset handover list, exit interview.

## Performance
Goals and OKRs (objective with key results, progress), review cycles (draft, active, closed, published; self, manager and peer 360 reviews; rating scale and questions), calibration table, 1:1 notes, feedback requests.

## Learning, policies, feed
Course catalogue with self-enrolment, HR assignment (including everyone), completion tracking; mandatory policies with versioned acknowledgement and tracking of who has not acknowledged; announcements (HR), kudos (everyone), birthdays (entered by each person or HR, there is no other source) and work anniversaries (from the directory start date).

## API (`/api/v1/talent`, auth-wrapped, org-scoped)
`me`, `directory`, `settings`, `jobs`, `applications`, `interviews`, `offers`, `candidates`, `pool`, `reports`, `templates`, `checklists`, `checklist-tasks`, `assets`, `goals`, `cycles`, `reviews`, `one-on-ones`, `feedback`, `courses`, `policies`, `announcements`, `feed`, `kudos`, `celebrations`. Lists return `{<name>: [...]}`.

## Hire into HR
The pipeline dialog has **Hire into HR** for hired candidates (or an accepted offer). `POST /api/v1/bridge/talent/hire`:
- creates the HR employee (`hr.CreateFromNewHire`): name, work email, phone, join date from the offer start date, department and designation looked up or created by name, manager from the job's hiring manager when they are an employee; links the account when a member has that email; creates a Contacts entry (label Employee);
- starts the onboarding checklist (the first onboarding template; a 7-task default template is created when none exists), with Tasks items for each assignee;
- optionally creates an "Onboarding: name" Projects project with one issue per default task;
- is idempotent: the application id (`hr_employees.source_ref = talent:<id>`) or the work email finds the existing employee, nothing is duplicated, and the response says `created: false`.
The response lists "open in" links (HR, Contacts, directory, checklist, project). The offered salary is **not** copied: set the salary structure in HR (the response says so).
Links are also stored in `adrient.record_links` (`GET /api/v1/bridge/links?app=&type=&id=`).
Deep links: `/talent?view=onboarding|jobs|pool|reports|...`.
