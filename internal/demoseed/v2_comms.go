package demoseed

import (
	"fmt"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/chat"
	"github.com/Adrient-tech/Adrient-Workspace/internal/mail"
)

// ---- Mail ---------------------------------------------------------------
//
// The dev stack stores mail in adrient.mail_messages (the local backend). The
// seeder inserts rows into the admin's mailbox through the mail repository,
// exactly as the local backend does when a message is delivered. Nothing is
// ever sent, and every address uses the reserved .example TLD.

type mm struct {
	from     string // first name of a team member, "me", or "Name <addr>" for an outsider
	body     string
	hoursAgo float64
}

type mthread struct {
	subject string
	labels  []string
	starred bool
	unread  bool
	msgs    []mm
	draft   bool
}

func ext(name, domain string) string {
	return fmt.Sprintf("%s <%s@%s.example>", name, slug(name), domain)
}

var mailThreads = []mthread{
	{subject: "Q3 sales review: numbers attached", labels: []string{"Sales"}, starred: true, msgs: []mm{
		{"Rohan", "Hi Asha,\n\nThe Q3 numbers are in. We closed at 112% of target, driven by the Indore and Pune accounts. Two renewals slipped into October. Full table is in the Sales by region sheet.\n\nCan we put 20 minutes on Friday's review for the discounting discussion?\n\nRohan", 52},
		{"me", "Thanks Rohan. Great quarter. Yes, let's take discounting first on Friday. Please add the renewal pipeline to the sheet so we can see the slipped ones.\n\nAsha", 49},
		{"Rohan", "Done, the pipeline tab is updated. Sending the agenda tonight.", 26}}},
	{subject: "PO-1042: delivery delay from Kaveri Textiles", labels: []string{"Suppliers"}, unread: true, msgs: []mm{
		{ext("Meera Joshi", "kaveritextiles"), "Dear Asha,\n\nOur dyeing unit had a power outage last week, so PO-1042 (1,200 cotton shirts) will reach your Pune store on 28 November instead of the 22nd. We can send 400 pieces by road this weekend if that helps.\n\nSincere apologies,\nMeera Joshi\nKaveri Textiles", 3}}},
	{subject: "Payroll cut-off moved to Friday", labels: []string{"HR"}, unread: true, msgs: []mm{
		{"Neha", "Hi Asha, because of the festival holidays the payroll cut-off moves from Thursday to Friday 6 pm. Attendance regularisations and leave approvals must be done before then. I will remind the managers tomorrow.\n\nNeha", 5}}},
	{subject: "Warehouse safety drill, Thursday 3 pm", labels: []string{"Operations"}, msgs: []mm{
		{"Arjun", "Team, the fire and evacuation drill is on Thursday at 3 pm. Assembly point is the north gate. Shift B joins at 3:15. Please make sure the forklifts are parked before the siren.\n\nArjun", 30},
		{"me", "Thanks Arjun. I will join at the north gate. Please share the headcount sheet afterwards.", 28}}},
	{subject: "GST return: documents needed by Monday", labels: []string{"Finance"}, starred: true, unread: true, msgs: []mm{
		{"Priya", "Asha, for the October GSTR-3B I still need: the sales register for the Pune store, three purchase invoices from Narmada Packaging, and the e-way bill summary. The accountant files on Tuesday.\n\nPriya", 7}}},
	{subject: "Payment received: invoice INV-0087", labels: []string{"Finance", "Customers"}, msgs: []mm{
		{ext("Accounts Desk", "sahyadrihardware"), "Hello,\n\nWe have transferred INR 1,18,400 against invoice INV-0087 today via NEFT. UTR: HDFCN26110198223. Please confirm receipt.\n\nAccounts, Sahyadri Hardware Traders", 20},
		{"me", "Received with thanks. The invoice is marked paid. Regards, Asha", 18}}},
	{subject: "Welcome to the team, Nikhil", labels: []string{"HR"}, msgs: []mm{
		{"Neha", "Hi all, please welcome Nikhil Hegde who joins us as Senior Backend Engineer on Monday. Laptop and accounts are ready. Imran will run the onboarding and Faisal will set up access.\n\nNeha", 70},
		{"Imran", "Looking forward to it. I have the first week plan ready.", 69}}},
	{subject: "Re: Pune store opening checklist", labels: []string{"Operations"}, msgs: []mm{
		{"me", "Karan, can you confirm the shelving delivery and the POS hardware for the 1st? The checklist in Docs has the full list.", 100},
		{"Karan", "Shelving arrives Tuesday, POS terminals are already at the store. Only the signage is pending, the printer promised Wednesday.", 95},
		{"me", "Good. Please send me a photo of the signage once it is up.", 94},
		{"Karan", "Will do.", 20}}},
	{subject: "Quote request: 2,000 steel bottles with logo", labels: []string{"Customers"}, unread: true, starred: true, msgs: []mm{
		{ext("Anil Bhatt", "godavaripharma"), "Hello,\n\nWe are looking for 2,000 steel bottles (750 ml) with our logo for a customer event on 15 December. Could you share a quote with unit price, lead time and the logo printing charges?\n\nAnil Bhatt\nPurchase, Godavari Pharma Distributors", 9}}},
	{subject: "Security: new device sign-in noticed", labels: []string{"IT"}, msgs: []mm{
		{"Faisal", "Hi Asha, we noticed a sign-in to your account from a new laptop in Pune on Tuesday. If that was you there is nothing to do. If not, tell me right away and I will reset the session.\n\nFaisal, IT", 40}}},
	{subject: "Agenda: Quarterly business review", labels: []string{"Sales"}, msgs: []mm{
		{"Rohan", "Asha, agenda for Friday: 1) Q3 results 2) Pipeline and renewals 3) Discounting policy 4) Hiring plan 5) Q4 priorities. I attached the deck as a Slides link. Please add your points on operations.", 30},
		{"me", "Added the warehouse automation update under item 5.", 29}}},
	{subject: "Supplier onboarding policy v3 for review", labels: []string{"HR", "Operations"}, msgs: []mm{
		{"Neha", "Asha, v3 of the supplier onboarding policy is ready in Docs. Main changes: GST and MSME verification before the first PO, a 30-day review for new suppliers, and a conflict of interest declaration. Comments by Friday please.", 80},
		{"Karan", "Looks good. I added one comment about the bank detail verification step.", 60}}},
	{subject: "Diwali campaign results", labels: []string{"Marketing"}, msgs: []mm{
		{"Sneha", "Campaign wrap-up: 18,400 reach, 1,260 leads, 214 orders worth INR 9.4 lakh. Best channel was WhatsApp broadcasts (31% of orders). Full sheet and creative samples are in Drive under Marketing.\n\nSneha", 120}}},
	{subject: "Escalation: delayed delivery to Surat", labels: []string{"Customers"}, unread: true, msgs: []mm{
		{"Divya", "Asha, Vindhya Retail Mart in Surat has escalated their order 4417 which is 4 days late. They are a top-10 customer. I have promised a revised date of Friday and a 5% credit note. Can you confirm the stock is available at the main warehouse?\n\nDivya", 4}}},
	{subject: "Friday lunch order", msgs: []mm{
		{"Arjun", "Team lunch on Friday: veg thali or biryani? Reply by Wednesday noon so I can place the order.", 16},
		{"me", "Biryani for me, please.", 15}}},
	{subject: "Reminder: submit expense claims by the 25th", labels: []string{"Finance"}, msgs: []mm{
		{"Priya", "Reminder to everyone: expense claims for November must be submitted by the 25th with receipts attached. Late claims roll into the December payroll.\n\nPriya", 140}}},
	{subject: "Domain and hosting renewal due 30 November", labels: []string{"IT"}, msgs: []mm{
		{"Faisal", "The website domain and hosting renew on 30 November (INR 14,900). Shall I renew for two years to save about 12 percent?\n\nFaisal", 190},
		{"me", "Yes, renew for two years, and please put the invoice in the Finance folder in Drive.", 188}}},
	{subject: "Partnership proposal: Coromandel Logistics", labels: []string{"Partners"}, starred: true, msgs: []mm{
		{ext("Suresh Pillai", "coromandellogistics"), "Dear Asha,\n\nFollowing our call, we would like to propose a 12-month last-mile partnership covering Pune, Nashik and Indore with a guaranteed next-day slot for your top accounts. The one-page proposal is attached. We are happy to run a 4-week pilot.\n\nSuresh Pillai\nCoromandel Logistics", 220},
		{"me", "Thanks Suresh, this is interesting. Let's start with the 4-week pilot in Pune. Karan will contact you with the volumes.", 200}}},
	{subject: "Monthly MIS: October", labels: []string{"Finance"}, msgs: []mm{
		{"me", "Priya, please send the October MIS by Thursday: revenue by region, gross margin, receivables ageing and stock value.", 160},
		{"Priya", "On it. Receivables ageing will show three accounts over 60 days, I will flag them.", 150}}},
	{subject: "Updated vendor payment terms", labels: []string{"Suppliers"}, msgs: []mm{
		{"me", "Hi all, from December we move all new suppliers to 30-day payment terms. Existing contracts stay unchanged until renewal. Karan will send the revised terms to each vendor.\n\nAsha", 260}}},
	{subject: "Draft: Notes from the supplier call", draft: true, msgs: []mm{
		{"me", "Notes from today's call with Kaveri Textiles:\n- partial shipment of 400 pieces this weekend\n- remaining 800 by 28 Nov\n- ask for", 2}}},
}

func (s *seeder) v2Mail() {
	repo := mail.NewRepository(s.pool)
	var adminEmail, adminName string
	if err := s.pool.QueryRow(s.ctx, `SELECT email, display_name FROM adrient.users WHERE id=$1::uuid`, s.admin).Scan(&adminEmail, &adminName); err != nil {
		s.warn("mail admin", err)
		return
	}
	now := time.Now()
	threads, msgs := 0, 0
	for _, t := range mailThreads {
		threadID := ""
		for i, m := range t.msgs {
			msg := mail.Message{OrgID: s.org, OwnerID: s.admin, Subject: t.subject, Body: m.body, Snippet: snippet(m.body),
				SentAt: now.Add(-time.Duration(m.hoursAgo * float64(time.Hour))), ThreadID: threadID, Labels: t.labels, ToAddrs: []string{adminEmail}}
			if i > 0 && !strings.HasPrefix(t.subject, "Re:") {
				msg.Subject = t.subject
			}
			switch {
			case m.from == "me":
				msg.Folder, msg.FromAddr, msg.FromName, msg.IsRead = "sent", adminEmail, adminName, true
				msg.ToAddrs = []string{replyTarget(t)}
				if t.draft {
					msg.Folder = "drafts"
				}
			case teamByFirst(m.from) != nil:
				p := teamByFirst(m.from)
				msg.Folder, msg.FromAddr, msg.FromName = "inbox", p.email(), p.name()
				msg.IsRead = !(t.unread && i == len(t.msgs)-1)
			default:
				name, addr := splitAddr(m.from)
				msg.Folder, msg.FromAddr, msg.FromName = "inbox", addr, name
				msg.IsRead = !(t.unread && i == len(t.msgs)-1)
			}
			if m.from != "me" && t.starred {
				msg.Starred = true
			}
			out, err := repo.Insert(s.ctx, msg)
			if err != nil {
				s.warn("mail insert", err)
				break
			}
			threadID = out.ThreadID
			msgs++
		}
		threads++
	}
	s.count("mail_threads", threads)
	s.count("mail_messages", msgs)
}

func snippet(b string) string {
	b = strings.Join(strings.Fields(b), " ")
	if len(b) > 110 {
		b = b[:110]
	}
	return b
}

func splitAddr(s string) (name, addr string) {
	if i := strings.Index(s, " <"); i > 0 && strings.HasSuffix(s, ">") {
		return s[:i], s[i+2 : len(s)-1]
	}
	return s, s
}

// replyTarget is who the admin's reply in a thread goes to.
func replyTarget(t mthread) string {
	for _, m := range t.msgs {
		if m.from == "me" {
			continue
		}
		if p := teamByFirst(m.from); p != nil {
			return p.email()
		}
		_, a := splitAddr(m.from)
		return a
	}
	return "team@" + demoDomain
}

// ---- Chat ---------------------------------------------------------------

type cm struct {
	who     string // first name, or "me" for the admin
	text    string
	minsAgo int
	rx      string // "👍:Rohan,Priya;🎉:Neha"
	replies []cm
}

type cchan struct {
	name, topic, desc string
	dm                string // first name of the other person for a DM
	members           []string
	msgs              []cm
	readBefore        int // admin has read messages up to this index (-1 = all read)
}

var chatChannels = []cchan{
	{name: "general", topic: "Company news and everyday chat", desc: "Everyone is here. Announcements, celebrations and anything for the whole team.", members: []string{"Rohan", "Priya", "Imran", "Neha", "Arjun", "Divya", "Karan", "Sneha", "Faisal"}, readBefore: -1, msgs: []cm{
		{"Neha", "Good morning everyone! A reminder that the payroll cut-off is moved to Friday 6 pm.", 4300, "👍:Rohan,Priya,Arjun,Divya;🙏:Karan", nil},
		{"me", "Thanks Neha. Also, the Pune store opens on the 1st. Please drop by if you are in town, we are doing a small puja at 10 am.", 4100, "🎉:Rohan,Priya,Imran,Neha,Sneha;❤️:Divya,Karan", []cm{
			{"Divya", "Will there be sweets?", 4090, "😂:Arjun,Rohan", nil},
			{"me", "Of course. Boondi laddoos from the shop next door.", 4085, "😋:Divya,Sneha,Karan", nil}}},
		{"Sneha", "Diwali campaign wrap-up is in Drive > Marketing. 214 orders, thank you all for the push.", 3900, "🔥:Rohan,Divya,Neha,Imran;👏:Priya,Karan,Arjun", nil},
		{"Imran", "Heads up: we deploy the new release tonight at 11 pm. The app may be slow for ten minutes.", 3000, "👍:Faisal,Neha", nil},
		{"Arjun", "Fire drill is Thursday 3 pm at the north gate. Please do not skip it!", 2400, "👍:Priya,Neha,Karan,Rohan,Sneha", nil},
		{"Priya", "Expense claims for November are due by the 25th, with receipts.", 1900, "👍:Neha", nil},
		{"Faisal", "Wi-Fi at the Pune store is upgraded. If you see a slow connection, tell me.", 1500, "🙌:Karan,Arjun,Rohan", nil},
		{"Rohan", "Welcome Nikhil, our new backend engineer! 🎉 He starts Monday.", 900, "🎉:Asha,Neha,Imran,Priya,Faisal,Sneha,Divya;👋:Karan,Arjun", nil},
		{"Divya", "Customer of the month: Vindhya Retail Mart, five orders in November.", 300, "🏆:Rohan,Neha,Sneha;👏:Asha,Priya", nil},
		{"me", "Great work everyone. Q3 closed at 112% of target.", 120, "🚀:Rohan,Priya,Imran,Neha,Divya,Sneha,Karan;🎉:Arjun,Faisal", nil}}},
	{name: "sales", topic: "Pipeline, deals and customers", desc: "Daily sales chatter. Pipeline updates, quotes and escalations.", members: []string{"Rohan", "Divya", "Sneha", "Priya", "Karan"}, readBefore: 7, msgs: []cm{
		{"Rohan", "Pipeline check: 12 open deals worth INR 38 lakh, 4 in negotiation.", 3700, "👀:Divya,Sneha", nil},
		{"Divya", "Godavari Pharma wants a quote for 2,000 steel bottles with a logo. I'll build it with Karan's costs.", 3600, "👍:Rohan", []cm{
			{"Karan", "Cost per bottle is INR 118 landed, logo printing adds 11.", 3590, "", nil},
			{"Divya", "Thanks! I'll quote at INR 165 with a 5 percent volume discount.", 3580, "💯:Rohan", nil}}},
		{"Sneha", "Landing page for the bulk-order offer is live, link in the campaign sheet.", 3000, "🔥:Rohan,Divya", nil},
		{"Rohan", "We won Sahyadri Hardware, INR 4.2 lakh annual supply 🎉", 2500, "🎉:Divya,Sneha,Karan,Priya,Asha;🥳:Neha", nil},
		{"Priya", "Please remember the credit note for Vindhya needs Asha's approval.", 1800, "👍:Divya", nil},
		{"Divya", "Vindhya Retail Mart (Surat) escalated order 4417. Promised delivery by Friday.", 1500, "😬:Rohan,Karan", []cm{
			{"me", "Stock is available at the main warehouse. Arjun will dispatch tomorrow morning.", 1450, "🙏:Divya;👍:Rohan", nil}}},
		{"Rohan", "Reminder: quarterly review on Friday. Please update your deals before Thursday evening.", 600, "👍:Divya,Sneha", nil},
		{"Sneha", "New case study draft: how Narmada Packaging cut returns by 18%. Feedback welcome.", 240, "📝:Rohan", nil},
		{"Divya", "Anyone free to join a customer demo tomorrow 11 am?", 90, "", nil}}},
	{name: "operations", topic: "Warehouse, logistics and stores", desc: "Stock, dispatch, store operations and vendors.", members: []string{"Arjun", "Karan", "Faisal", "Divya", "Priya"}, readBefore: 5, msgs: []cm{
		{"Arjun", "Cycle count done for aisle A to D. Variance 0.4 percent, within limits.", 3800, "👏:Karan,Priya", nil},
		{"Karan", "PO-1042 from Kaveri Textiles is delayed to the 28th. 400 pieces come by road this weekend.", 3500, "😕:Arjun,Divya", []cm{
			{"Arjun", "OK, I'll keep dock 2 free for Saturday.", 3490, "👍:Karan", nil}}},
		{"me", "Please prioritise Vindhya's order 4417 tomorrow morning.", 3000, "👍:Arjun", nil},
		{"Faisal", "Barcode scanners firmware updated on all 6 devices.", 2600, "🙌:Arjun", nil},
		{"Arjun", "Forklift 2 is back after service.", 2000, "👍:Karan", nil},
		{"Karan", "Shelving for the Pune store arrives Tuesday, POS terminals are already there.", 1500, "🎉:Divya,Arjun", nil},
		{"Arjun", "Dispatched 38 orders today, 2 pending for courier pickup.", 700, "💪:Karan,Priya", nil},
		{"me", "Thanks. Safety drill on Thursday 3 pm, everyone please attend.", 400, "👍:Arjun,Karan,Faisal", nil},
		{"Karan", "Packing tape stock is low, raising a PO for 200 rolls.", 100, "", nil}}},
	{name: "engineering", topic: "Builds, releases and bugs", desc: "Engineering team room.", members: []string{"Imran", "Faisal", "Neha", "Priya"}, readBefore: -1, msgs: []cm{
		{"Imran", "Release 2.14 is out. Changes: faster invoice PDF, barcode scan on mobile, 9 bug fixes.", 3900, "🚀:Faisal,Priya;🎉:Neha", nil},
		{"Faisal", "Nightly backup restored cleanly into staging, test passed.", 3000, "✅:Imran", nil},
		{"Imran", "Reminder: code freeze starts Wednesday for the quarter end close.", 2300, "👍:Faisal", nil},
		{"Priya", "Bug: GST summary shows the wrong total when a credit note is in the same month. Steps in the ticket.", 1700, "🐛:Imran", []cm{
			{"Imran", "Reproduced. Fix is in review, will ship tonight.", 1650, "🙏:Priya", nil}}},
		{"Faisal", "Planned maintenance Sunday 2 am to 4 am for the database upgrade.", 800, "👍:Imran,Neha", nil}}},
	{name: "finance", topic: "Books, GST and payroll", desc: "Finance and compliance.", members: []string{"Priya", "Neha", "Rohan"}, readBefore: 4, msgs: []cm{
		{"Priya", "October books are closed. Net profit INR 11.8 lakh, up 9 percent on September.", 3500, "🎉:Neha,Rohan;📈:Asha", nil},
		{"Neha", "Payroll draft is ready for review. 34 employees, gross INR 24.6 lakh.", 2800, "👍:Priya", nil},
		{"Priya", "GST 3B filing on Tuesday. I need the Pune sales register.", 1800, "", []cm{
			{"Rohan", "Sending it this afternoon.", 1750, "🙏:Priya", nil}}},
		{"Priya", "Three customers are over 60 days on receivables. I'll share the list.", 600, "👀:Rohan,Asha", nil},
		{"Neha", "PF challan paid, receipt saved in Drive > Finance.", 200, "✅:Priya", nil}}},
	{name: "random", topic: "Memes, music and lunch", desc: "Everything that is not work.", members: []string{"Rohan", "Sneha", "Arjun", "Divya", "Imran", "Karan"}, readBefore: -1, msgs: []cm{
		{"Arjun", "Friday lunch: veg thali or biryani? 🍛", 4000, "🍛:Rohan,Divya,Karan,Imran;😋:Sneha", nil},
		{"Sneha", "Office plants are thriving thanks to Divya 🌱", 3100, "🌱:Divya,Rohan;❤️:Karan", nil},
		{"Rohan", "Anyone for cricket on Sunday morning?", 2200, "🏏:Arjun,Imran,Karan,Divya", nil},
		{"Imran", "Which chai place near the Pune office is the best? Asking for a friend.", 1000, "☕:Rohan,Sneha,Arjun,Karan,Divya", nil},
		{"Divya", "Birthday cake for Karan at 4 pm! 🎂", 200, "🎂:Rohan,Sneha,Arjun,Imran,Asha;🎉:Neha", nil}}},
	{dm: "Rohan", readBefore: -1, msgs: []cm{
		{"Rohan", "Asha, can we move the Friday review to 11?", 500, "", nil},
		{"me", "Sure. 11 works.", 490, "👍:Rohan", nil},
		{"Rohan", "Also I'd like to bring Divya for the customer escalation item.", 480, "", nil},
		{"me", "Please do.", 470, "👍:Rohan", nil}}},
	{dm: "Priya", readBefore: 1, msgs: []cm{
		{"me", "Priya, can you share the receivables ageing by tomorrow?", 900, "", nil},
		{"Priya", "Yes. Also I need your approval on the Vindhya credit note.", 800, "", nil},
		{"Priya", "It is INR 5,900, 5 percent of the order.", 799, "", nil}}},
	{dm: "Neha", readBefore: -1, msgs: []cm{
		{"Neha", "Nikhil's offer letter is signed. Joining Monday.", 1700, "🎉:Asha", nil},
		{"me", "Great news. Thank you Neha.", 1690, "", nil}}},
}

func (s *seeder) v2Chat() {
	repo := chat.NewRepository(s.pool)
	store := chat.NewStore(repo)
	var adminName string
	_ = s.pool.QueryRow(s.ctx, `SELECT display_name FROM adrient.users WHERE id=$1::uuid`, s.admin).Scan(&adminName)
	uid := func(who string) (string, string) {
		if who == "me" || who == "Asha" {
			return s.admin, adminName
		}
		if p := teamByFirst(who); p != nil && p.UserID != "" {
			return p.UserID, p.name()
		}
		return "", ""
	}
	now := time.Now()
	nch, nmsg, nrx := 0, 0, 0
	for _, c := range chatChannels {
		var ch chat.TChannel
		var err error
		var members []string
		for _, f := range c.members {
			if id, _ := uid(f); id != "" {
				members = append(members, id)
			}
		}
		if c.dm != "" {
			other, _ := uid(c.dm)
			ch, err = store.CreateTChannel(s.ctx, s.org, s.admin, "dm", "", "", "", []string{other})
		} else {
			ch, err = store.CreateTChannel(s.ctx, s.org, s.admin, "public", c.name, c.topic, c.desc, members)
		}
		if err != nil {
			s.warn("chat channel", err)
			continue
		}
		nch++
		post := func(m cm, parent string) string {
			id, name := uid(m.who)
			if id == "" {
				return ""
			}
			res, err := store.Post(s.ctx, s.org, id, name, ch.ID, parent, m.text, nil, "")
			if err != nil {
				s.warn("chat post", err)
				return ""
			}
			at := now.Add(-time.Duration(m.minsAgo) * time.Minute)
			if _, err := s.pool.Exec(s.ctx, `UPDATE adrient.chat_messages SET sent_at=$2 WHERE id=$1::uuid`, res.Message.ID, at); err != nil {
				s.warn("chat time", err)
			}
			nmsg++
			for _, grp := range strings.Split(m.rx, ";") {
				parts := strings.SplitN(grp, ":", 2)
				if len(parts) != 2 || parts[0] == "" {
					continue
				}
				for _, who := range strings.Split(parts[1], ",") {
					rid, _ := uid(who)
					if rid == "" {
						continue
					}
					if _, err := repo.ToggleReaction(s.ctx, s.org, res.Message.ID, rid, parts[0]); err != nil {
						s.warn("chat reaction", err)
						continue
					}
					nrx++
				}
			}
			return res.Message.ID
		}
		var last time.Time
		for _, m := range c.msgs {
			pid := post(m, "")
			for _, r := range m.replies {
				post(r, pid)
			}
			if t := now.Add(-time.Duration(m.minsAgo) * time.Minute); t.After(last) {
				last = t
			}
		}
		if !last.IsZero() {
			_, _ = s.pool.Exec(s.ctx, `UPDATE adrient.chat_channels SET last_message_at=$2, updated_at=$2 WHERE id=$1::uuid`, ch.ID, last)
		}
		// The admin's read cursor: everything up to readBefore is read, the rest is unread.
		cursor := now
		if c.readBefore >= 0 && c.readBefore < len(c.msgs) {
			cursor = now.Add(-time.Duration(c.msgs[c.readBefore].minsAgo) * time.Minute)
		}
		_, _ = s.pool.Exec(s.ctx, `INSERT INTO adrient.chat_read_cursors (channel_id, user_id, last_read_at) VALUES ($1::uuid,$2::uuid,$3)
			ON CONFLICT (channel_id, user_id) DO UPDATE SET last_read_at=EXCLUDED.last_read_at`, ch.ID, s.admin, cursor)
	}
	s.count("chat_channels", nch)
	s.count("chat_messages", nmsg)
	s.count("chat_reactions", nrx)
}
