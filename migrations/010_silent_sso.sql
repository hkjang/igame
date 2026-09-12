-- A login started with prompt=none asks the identity provider to answer from
-- an existing session only. When there is none the provider comes back with
-- error=login_required, which is an ordinary answer, not a failure — but the
-- callback can only tell that apart from a real provider error if it knows
-- the flow was silent. The flag lives with the rest of the flow's state so a
-- caller cannot claim a silent flow it did not start.
ALTER TABLE oidc_flows ADD COLUMN IF NOT EXISTS silent boolean NOT NULL DEFAULT false;
