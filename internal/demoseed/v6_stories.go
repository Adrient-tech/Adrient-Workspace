package demoseed

import (
	"fmt"

	"github.com/Adrient-tech/Adrient-Workspace/internal/podcasts"
)

// v6Stories adds six fictional company stories for the Stories app. Covers are
// the generated gradient covers (no images), and no audio is generated: with no
// voice provider set up, listeners hear them read aloud by their browser.
func (s *seeder) v6Stories() {
	svc := podcasts.NewStories(s.pool, nil)
	// authors: the admin and up to two other people of the org
	authors := []string{s.admin}
	if rows, err := s.pool.Query(s.ctx, `SELECT id::text FROM adrient.users WHERE org_id=$1 AND id<>$2::uuid ORDER BY created_at LIMIT 2`, s.org, s.admin); err == nil {
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				authors = append(authors, id)
			}
		}
		rows.Close()
	}
	who := func(i int) string { return authors[i%len(authors)] }

	type ep struct{ title, summary, body string }
	type story struct {
		author                          int
		title, genre, lang, description string
		style                           string
		daysAgo                         int
		eps                             []ep
	}
	stories := []story{
		{1, "Welcome aboard: your first week", "Onboarding", "en", "A gentle walk through your first five days: where to sit, whom to ask, and what nobody tells you about the canteen.", "172,205,140", 2, []ep{
			{"Day one", "Arriving, badges and the first cup of tea.", `<p>The lift opens on the third floor and the first thing you notice is the smell of fresh coffee. Nobody expects you to know where anything is today. That is the whole point of day one.</p><p>[Meera] You must be the new joiner. I am Meera, your buddy for this week. Come, I will show you your desk, and then we will get you a proper cup of tea.</p><p>[Narrator] Your laptop is already waiting, with your name on a small green card. Take your time. Read nothing. Smile a lot.</p><p>By lunch you will have met eleven people and remembered the names of three. That is a good score. By Friday it will be all eleven.</p>`},
			{"Meeting your team", "Who does what, and how we decide things.", `<p>On the second day you sit in on the weekly planning meeting. It looks chaotic. It is not. Every voice in the room gets a turn, and the person who speaks last is usually the most senior.</p><p>[Ravi] We ship small things often. If you see something broken, say so, and say it kindly. Nobody here has ever been fired for asking a question.</p><p>[Narrator] Write down the questions you are embarrassed to ask. Ask them anyway. Someone else in the room has the same ones.</p>`},
			{"Finding your way around", "Tools, folders and the quiet corner on the fourth floor.", `<p>Everything you need is in four places: Mail for talking to people, Chat for quick questions, Drive for files, and Calendar for the rest of your life.</p><p>[Meera] When you cannot find a file, do not search the whole drive. Ask in chat. Somebody will point you to the folder in half a minute.</p><p>[Narrator] The fourth floor has a quiet corner with a green sofa. It is not on any map. Now you know.</p>`},
		}},
		{2, "Safety Moments", "Safety", "en", "Three minutes, once a week. Small stories about staying safe at work, told by the people who learned the lesson.", "18,48,150", 5, []ep{
			{"Walk, don't run", "A short story about a wet floor and a very full tray.", `<p>It was a Tuesday, and the canteen floor had just been mopped. The yellow sign was there, folded flat against the wall, the way it always was when someone was in a hurry.</p><p>Priya was carrying a tray with two bowls of soup and she was late for a call. Her foot slid, only a little, but enough. The soup went one way and Priya went the other.</p><p>Nobody was hurt. But the sign stayed open for the rest of the week, and so did everyone's eyes. Walk, do not run. Open the sign. Close it only when the floor is dry.</p>`},
			{"The yellow line", "Why the painted line in the warehouse matters.", `<p>The yellow line in the warehouse is not decoration. On one side, people walk. On the other side, forklifts drive.</p><p>[Ravi] I have worked here nine years. The line has never once been the problem. The problem is the shortcut.</p><p>[Narrator] When you want to cut across, wait. Look both ways. Make eye contact with the driver. It costs you ten seconds. It protects everything else.</p>`},
			{"When the alarm sounds", "What to do in the first minute.", `<p>When the alarm sounds, you do not finish the email. You do not go back for your bag. You stand up, you walk to the nearest green exit sign, and you follow the person in the yellow vest.</p><p>Outside, find your floor's flag and tell the warden your name. That is the whole drill. Practise it once, and you will not have to think about it twice.</p>`},
		}},
		{0, "Founder Notes", "Leadership", "en", "Short, honest notes from the founder about why we do things the way we do.", "262,300,140", 9, []ep{
			{"Why we started", "The first desk, the first customer, the first mistake.", `<p>We started with one desk, one laptop and a list of problems that annoyed us. The list was longer than the plan. That was fine.</p><p>The first customer paid late, and taught us the most important lesson in business: write down what you promised. We did, and we still do.</p><p>If you remember one thing from this note, let it be this. Build the thing you would want to use on a Monday morning, tired, with five minutes to spare.</p>`},
			{"Small team, long memory", "How we keep what we learn.", `<p>A company is a group of people who remember things together. When someone leaves, a piece of memory leaves with them, unless we wrote it down.</p><p>So we write things down. Decisions, reasons, even the mistakes. Especially the mistakes. They are the cheapest lessons we will ever get.</p>`},
		}},
		{0, "Quarterly update, as a story", "Updates", "en", "The numbers matter, but people remember stories. This quarter, told as one.", "30,70,150", 1, []ep{
			{"The quarter we found our rhythm", "Revenue, hiring, and the launch that nearly slipped.", `<p>In July we had a plan and a nervous feeling. In September we had a launch and a team that finished each other's sentences.</p><p>Revenue grew by a fifth, which is a polite way of saying we sold more than we hoped and shipped later than we promised. Eleven people joined. Two moved to new teams. Nobody left, and that is the number I am proudest of.</p><p>The next quarter has one theme: make what we have faster, calmer and kinder. Fewer new things. Better old things. Thank you for a quarter that felt like one.</p>`},
		}},
		{1, "The Lighthouse Keeper's Ledger", "Fiction", "en", "A short story in two parts, about a keeper, a ledger, and a ship that was never recorded.", "205,250,160", 4, []ep{
			{"Part one: the empty page", "A keeper finds a page he did not write.", `<p>Every night for thirty years, Anand wrote one line in the ledger: the date, the weather, and the word "quiet". On the night of the storm he found a page he had not written.</p><p>[Anand] Who has been in my lighthouse?</p><p>[Narrator] The page held a single line, in careful handwriting that was not his. "Ship sighted, no name, heading home."</p><p>The wind pressed against the glass. Far below, in the black water, something small and bright was moving toward the rocks.</p>`},
			{"Part two: the ship with no name", "The light turns, and a stranger answers.", `<p>Anand climbed the stairs two at a time, which he had not done in years. The great lamp turned, and the beam swept across the water, and there it was: a small wooden boat with a lantern at the bow.</p><p>[Stranger] Thank you for keeping the light. We have been lost for a very long time.</p><p>[Narrator] In the morning the sea was flat and silver. In the ledger, below the strange line, Anand wrote his own. "Ship found. Everyone home."</p>`},
		}},
		{2, "कंपनी की पहली सुबह", "Culture", "hi", "एक नई कर्मचारी की पहली सुबह की छोटी सी कहानी, हिंदी में।", "345,20,150", 3, []ep{
			{"पहली सुबह", "नई जगह, नए लोग और एक कप चाय।", `<p>सुबह के नौ बजे थे। अनीता ने दफ़्तर के दरवाज़े पर एक गहरी साँस ली और अंदर क़दम रखा।</p><p>[मीरा] आप अनीता हैं न? मैं मीरा हूँ। आज मैं आपके साथ रहूँगी। चलिए, पहले एक कप चाय पीते हैं।</p><p>[Narrator] अनीता मुस्कुराई। घबराहट अब थोड़ी कम हो गई थी। यहाँ हर कोई उसे नाम से बुला रहा था।</p>`},
		}},
	}
	var firstEps []string
	for _, st := range stories {
		author := who(st.author)
		v, err := svc.LoadViewer(s.ctx, s.org, author, author == s.admin)
		if err != nil {
			s.warn("stories viewer", err)
			return
		}
		sh, err := svc.CreateShow(s.ctx, v, podcasts.ShowInput{Title: &st.title, Genre: &st.genre, Language: &st.lang, Description: &st.description, CoverStyle: &st.style})
		if err != nil {
			s.warn("story", err)
			return
		}
		s.count("stories", 1)
		for i, e := range st.eps {
			e := e
			d, err := svc.CreateEpisode(s.ctx, v, sh.ID, podcasts.EpisodeInput{Title: &e.title, Summary: &e.summary, BodyHTML: &e.body})
			if err != nil {
				s.warn("story episode", err)
				return
			}
			if _, err := svc.Publish(s.ctx, v, d.ID, nil); err != nil {
				s.warn("story publish", err)
				return
			}
			// spread publish dates over the last days so "New this week" has an order
			when := st.daysAgo + (len(st.eps)-1-i)*0
			if _, err := s.pool.Exec(s.ctx, `UPDATE adrient.story_episodes SET published_at=now() - ($2::int * interval '1 day') - ($3::int * interval '1 hour') WHERE id=$1::uuid`, d.ID, when, i); err != nil {
				s.warn("story dates", err)
			}
			if i == 0 {
				firstEps = append(firstEps, d.ID)
			}
			s.count("story_episodes", 1)
		}
	}
	// a little listening history for the admin so the home page has a "Continue listening" shelf
	if len(authors) > 1 && len(firstEps) >= 3 {
		adminV, _ := svc.LoadViewer(s.ctx, s.org, s.admin, true)
		for k, id := range []string{firstEps[0], firstEps[1], firstEps[4%len(firstEps)]} {
			pos := 40000 + k*25000
			if err := svc.SaveProgress(s.ctx, adminV, id, podcasts.Progress{PositionMs: pos, DurationMs: 160000 + k*20000}); err != nil {
				s.warn("story progress", err)
			}
			_ = svc.RecordPlay(s.ctx, adminV, id)
			_ = svc.SetLike(s.ctx, adminV, id, k != 1)
		}
		_ = svc.SetFollow(s.ctx, adminV, "author", authors[1], true)
	}
	_ = fmt.Sprint
}
