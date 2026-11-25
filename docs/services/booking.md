# Booking pages (Calendar "Appointment schedule")

Calendly-style public pages. Owner creates a schedule in Calendar > Create > Appointment schedule; guests book at `/book/{slug}` without an account.

- Rules: duration, buffers before/after, minimum notice, horizon, slot step, max per day, weekly hours in the page time zone (DST aware), questions.
- Free time = weekly hours minus the owner's busy Calendar spans (own and accepted/pending invited events, recurring expanded, Out of office and Focus time included, free events and tasks ignored).
- Booking is re-validated on the server under a per-owner advisory lock; a unique index stops double booking.
- A booking creates a Calendar event (guest as attendee) and, if enabled, a Meet room linked to it.
- Confirmation mail with an .ics attachment goes to guest and owner only when a mail sender is configured; otherwise the page says so.
- Guests cancel through `/book/cancel/{token}`; owners cancel from the dialog.
- API: `/api/v1/booking/*` (signed in), `/api/v1/public/booking/*` (public, rate limited, honeypot). Migration 0260.
