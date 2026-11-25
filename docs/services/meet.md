# Meet (video meetings)

Peer-to-peer WebRTC mesh; signalling over the existing websocket `/api/v1/meet/rooms/{id}/connect`.
layout maths adapted with credit comments).

## Done
- Lobby: instant meeting, named meeting, join by code, persistent rooms with copy-link, delete.
- Green room: camera preview, mic/cam toggles, mic/camera/speaker pickers, display name, background blur / virtual background, noise suppression toggle.
- Call: group grid with active-speaker highlight, pin, layouts (auto / tiled / spotlight / sidebar), screen share (presenter auto-spotlighted), picture-in-picture, chat panel, emoji reactions (floating + tile badge), hand raise, participants panel (state icons, pin), meeting details panel (link + code copy), device switching mid-call, clear mic/cam state, shortcuts Ctrl/Cmd+D (mic) and Ctrl/Cmd+E (camera), join/leave/message/hand sounds, bandwidth-adaptive video (capture size and sender bitrate drop as tile count grows).
- Backend: new signal types `reaction`, `screen_share`; chat/reaction names stamped server-side, size limits; `?name=` (green-room name) and `?cid=` (one user in several tabs); fix: leave was never broadcast because the writer goroutine outlived the socket.
- Calendar already attaches a Meet room to an event (EventDialog); unchanged.
- Blur / virtual background: `@mediapipe/tasks-vision` (Apache-2.0), lazy-loaded on first use; model at `web/app/public/models/`, presets at `web/app/public/meet-backgrounds/`.

## When a meeting ends

A meeting runs from the first join until the last participant has been gone for 15 seconds (a page refresh does not end it), the host ends it for everyone, or the room is deleted. Then:

- The meeting row keeps `ended_at`, the reason, the start time and the participants (migration 0345). `GET /api/v1/meet/codes/{code}` returns `ended`, `ended_at`, `duration_s` and `participant_count`.
- A live `meet_ended` event reaches every open Chat/Calendar tab of the organisation, so the Chat meeting card turns into "Meeting ended · duration · N participants" with no Join button, and the Calendar event shows an "Ended" chip, without a reload.
- Everyone who took part gets an in-app "Meeting ended" notification.
- `/meet/<code>` of an ended meeting shows "This meeting has ended". The host (room owner) can press "Start a new meeting" (`POST /api/v1/meet/codes/{code}/restart`), which pushes `meet_restarted` so cards go back to Join. Others are told to ask the host.
- If the person who started it leaves but others stay, the meeting keeps running; it ends only when the room is empty.
- Joining an ended meeting is refused (`410 meeting_ended`) until it is restarted.
