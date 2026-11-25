# Chat (Teams-class messaging)

Module id `chat`, route `/chat`, tile name "Chat". Upgrade of the original DM/group chat into a Teams/Slack-style app.

## Features

| Area | Status |
|---|---|
| Public + private channels (create, browse, join, leave, topic, description, rename, archive) | done |
| Direct messages, notes-to-self, group DMs (3-9 people, de-duplicated by member set) | done |
| Sidebar sections: Starred, Channels, Direct messages, custom sections (create/rename/delete/move), collapse, bold unread, mention badges, mute | done |
| Threads: side panel, reply count and last reply on the parent | done |
| Activity view: @mentions, thread replies, fired reminders, unread highlight, badge | done |
| Later (saved messages) and reminders on messages (presets) | done |
| Drafts (autosaved per channel), Sent, Scheduled sends (send later), cancel | done |
| Pins and channel bookmarks | done |
| Emoji reactions (quick bar + full emoji-mart picker) | done |
| @mentions with autocomplete (org directory) + `@channel/@here/@everyone`; highlighted in messages | done |
| Edit and delete own messages ("edited" marker) | done |
| File attachments (existing Drive-backed chat attachments), GIF picker, markdown | done (existing) |
| Search (all channels or one, date range), jump to date in a channel, permalinks `/chat/c/<channel>/m/<message>` | done |
| Channel details (members add/remove, topic, description, bookmarks, archive) | done |
| People directory, profiles (title, status text/emoji), presence (online/away from live socket) | done |
| Notifications: in-app toast + browser Notification API when the tab is hidden; tab title badge | done |
| Huddles: one click creates/reuses a Meet room for the channel and opens `/meet/<code>?audio=1` (camera off); posts a "Huddle started" message | done |
| Mail sidebar chat panel (`ChatPanel`) and legacy gRPC `ChatService` | unchanged; their posts also reach the new live feed |

## Backend

- Migration `0115_chat_teams.sql` (additive: kind widened to public/private, topic/description/archived/huddle_code, message edited_at + mention_ids, sections, per-user prefs, profiles, pins, bookmarks, saved, reminders, drafts, scheduled).
- `internal/chat/teams_store.go` (queries, all scoped by org_id and channel membership), `teams_http.go` (REST under `/api/v1/chat/x/`, plain JSON handlers like chat attachments, not proto), `teams_hub.go` (per-user live feed `/api/v1/chat/x/live` + presence). The scheduler goroutine (10 s tick) delivers send-later messages and reminders with `FOR UPDATE SKIP LOCKED`.
- Existing proto routes and tables are untouched. New code is REST rather than proto/buf to avoid regenerating the gRPC stubs for ~50 endpoints.

