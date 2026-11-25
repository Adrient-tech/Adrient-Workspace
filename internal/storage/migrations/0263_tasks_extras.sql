-- Tasks extras: assignee, priority, repeat rule, reminder, board column per
-- task; list sharing; reminder delivery log. Idempotent.
CREATE TABLE IF NOT EXISTS adrient.task_extras (
    task_id        UUID PRIMARY KEY REFERENCES adrient.tasks(id) ON DELETE CASCADE,
    org_id         UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    assignee_id    UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    priority       INT  NOT NULL DEFAULT 0,        -- 0 none, 1 low, 2 medium, 3 high
    recurrence     TEXT NOT NULL DEFAULT '',       -- '' | daily | weekdays | weekly | monthly | yearly
    remind_minutes INT,                            -- minutes before due, NULL = no reminder
    board_status   TEXT NOT NULL DEFAULT 'todo',   -- todo | doing (done = completed)
    link_url       TEXT NOT NULL DEFAULT '',
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS task_extras_assignee_idx ON adrient.task_extras (org_id, assignee_id) WHERE assignee_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS adrient.task_list_shares (
    list_id    UUID NOT NULL REFERENCES adrient.task_lists(id) ON DELETE CASCADE,
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL DEFAULT 'editor',     -- viewer | editor
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (list_id, user_id)
);
CREATE INDEX IF NOT EXISTS task_list_shares_user_idx ON adrient.task_list_shares (org_id, user_id);

CREATE TABLE IF NOT EXISTS adrient.task_reminders_sent (
    task_id UUID NOT NULL REFERENCES adrient.tasks(id) ON DELETE CASCADE,
    due_at  TIMESTAMPTZ NOT NULL,
    user_id UUID NOT NULL,
    sent_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (task_id, due_at, user_id)
);
