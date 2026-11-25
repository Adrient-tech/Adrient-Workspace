# Stories (the Podcasts app)

Stories is the inside-the-company audio stories app. Colleagues write stories
or series, the server turns the text into audio with a text-to-speech voice, and
everyone they allow can listen. In the UI and on the launcher tile it is called
**Stories**. The route stays `/podcasts` (`/stories` redirects to it), the app id
stays `podcasts` for roles, app access and app controls, and the API lives under
`/api/v1/stories`.

## What it does

- **Home**: a hero with large cover art, title and a Resume/Play button, the
  episode list beside it with progress underlines and durations, then shelves:
  Continue listening, New this week, From people and series you follow,
  Trending in the company, Genres.
- **Shows** (browse by genre chips, sort), **Search**, **Library** (saved
  episodes and shows, liked, history).
- **Player**: a floating bar with 10 second back and forward, seek, speed
  0.75 to 2x, sleep timer (minutes or end of episode), position remembered per
  user, mark played, next episode. Keyboard: Space or K play/pause, Left/Right
  10 seconds, Shift+Left/Right previous/next episode, `[` and `]` speed.
  Download only when the author allows it and the `stories.download` control is on.
- **Read along / Story text**: every episode row and the player have an
  expander with the show notes, summary and the story text. The sentence being
  spoken is highlighted and a click jumps to it. Highlighting uses the timings
  stored with the audio (each synthesized part has its real length, sentences
  share it by length); with the browser voice it follows the utterance events.
- **Write a story** (the studio, `/podcasts/write`): title, cover (upload,
  pick from Drive, or a generated gradient cover with the title in big type),
  genre, language, description, series with episodes (add, reorder), a rich
  text editor with autosave, draft / publish / unpublish, scheduled publishing,
  who can listen (everyone, a group, or a department with its sub-departments),
  optional comments, optional downloads, voice settings, and listening stats
  (plays, listeners, completion rate, plays per day).
- **Social**: likes, library bookmarks, follow a series or an author (new
  episodes arrive through the notification center), comments with report /
  hide / delete (the author and org admins moderate).
- **Shows from the web**: subscribe to an outside audio feed by address (kept
  from the old Podcasts app: the server fetches the feed through an
  address-checked proxy). There is no built-in directory search.
- **Assistant**: the assistant panel in the studio can draft or improve a
  story and write a summary. Its tools read the episode text, replace or append
  text and set the summary; every change can be undone from the panel.

## Voices and audio generation

Audio is generated from the text, never uploaded. No voices ship with Adrient:
the administrator connects one (Admin > Voices), or listeners use the browser voice.

| Provider | What you need | Notes |
|---|---|---|
| OpenAI-compatible endpoint | Any server that answers `POST {base}/audio/speech` (OpenAI itself, Kokoro-FastAPI, openedai-speech, Piper wrappers) | Optional `GET {base}/audio/voices` is used to list voices. API key is stored encrypted (`ADRIENT_SECRETS_KEY`) or read from a named environment variable. Format WAV (exact lengths) or MP3. |
| Local command | A program that reads text on stdin and writes WAV or MP3 to stdout or to a file (`{out}`), for example `espeak-ng` or `piper` | Admin > Voices detects `espeak-ng` and `piper` with `which`. Arguments are a list (no shell), the voice id is validated, and the text is only ever on stdin. |
| Browser voice | Nothing | Always available. Each listener's device reads the text aloud with the system voices; nothing is stored. Hindi and Tamil depend on the voices installed on the device. |

Languages: English, Hindi, Tamil and others work when the chosen engine and
voice support them. eSpeak NG has `hi` and `ta` voices (robotic); Piper has
Hindi voice models and community models for other languages; Kokoro has Hindi
voices. Adrient does not check that a voice speaks the story's language.

How a request runs:

1. The author saves or publishes an episode. If a server voice applies and
   "make audio automatically" is on, a job is queued. The HTTP request never
   waits for audio.
2. A worker (2 at a time, `FOR UPDATE SKIP LOCKED`) splits the text into
   sentences, groups them into parts of up to 3000 characters per speaker,
   calls the provider for each part, joins WAV (or MP3) parts, stores the file
   in object storage (`stories/audio/<org>/<hash>`), and records duration and
   sentence timings.
3. The cache key is a hash of the sentences, speakers, provider (and its last
   change), voice, speed, pitch and language. The same hash is never generated
   twice, even for another episode; changing the text or the voice makes the
   audio stale and queues a new job for published episodes.
4. Status (queued, generating with percent, ready, failed with the error) is
   shown in the studio. Failed jobs keep any older audio playable.
5. Character voices: start a paragraph with `[Name]`; `[Narrator]` returns to
   the narrator. Each name can have its own voice per story.

Limits (Admin > Voices): characters per person per day (default 200000),
requests per person per minute (6), longest episode (60000 characters). Every
request and result is written to the audit log (`story.audio_requested`,
`story.audio_ready`, `story.audio_failed`, `story.published_scheduled`), and
all mutating API calls are audited by the generic API audit.

## Access and controls

- App access: the `podcasts` app id (Admin > App access, roles).
- App controls (Admin > App controls): `stories.publish` (write and publish
  stories), `stories.generate_audio` (server-side audio), `stories.download`
  (download audio). Guests and external users default to off.
- Visibility: a story is visible to its author, and to others only through
  published episodes and an audience they belong to. Drafts are never visible.
  Org scoping applies to every query.

## Data

Migrations 0355 to 0357: `story_shows`, `story_episodes`, `story_progress`,
`story_plays`, `story_voice_providers`, `story_settings`, `story_audio_jobs`,
`story_likes`, `story_library`, `story_follows`, `story_comments`,
`story_reports`. The old `podcast_subscriptions` table (0090) still backs
"Shows from the web". The demo seed (step 6) adds six fictional stories
(onboarding, a safety series, founder notes, a quarterly update as a story, a
short fiction in two parts, and one in Hindi) with generated gradient covers
and no audio, so they play with the browser voice until a provider is set up.

## API (all under `/api/v1/stories`)

`GET /home`, `/search?q=`, `/shows`, `/shows/{id}`, `/episodes/{id}`,
`/library`, `/voices`, `/quota`, `/audiences`, `/moderation`;
`POST /shows`, `/shows/{id}/episodes`, `/episodes/{id}/publish|unpublish|audio|play|comments`;
`PATCH /shows/{id}`, `/episodes/{id}`; `PUT /shows/{id}/cover`, `/episodes/{id}/progress|played|like`,
`/library/{show|episode}/{id}`, `/follows/{show|author}/{id}`;
`GET /episodes/{id}/audio` (supports Range; `?download=1` when allowed);
admin: `/admin/providers`, `/admin/detect`, `/admin/settings`, `/admin/jobs`.

The old `/api/v1/podcasts/subscriptions` and `/api/v1/podcasts/feed` routes are unchanged.

## Not built / known limits

- No voices are included. Sentence highlighting is exact per audio part, and an
  estimate (by sentence length) inside a part.
- Audio is read fully into memory to serve ranges (files up to 200 MB).
- The browser voice cannot be paused mid-sentence; pausing stops and resuming
  restarts the current sentence.
- Pitch only works for command engines that accept it (eSpeak NG).
- No offline downloads other than the audio file, no per-user notification
  preferences beyond Follow.
