-- Server-authoritative battle verification.
--
-- Results are now decided by replaying the player's recorded inputs against the
-- session-pinned content, so the submitted ledger is kept beside the result: it
-- is the evidence for the score, and the only way to re-derive an old battle if
-- a dispute or a rules change ever calls one into question.

ALTER TABLE realmguard_results
  ADD COLUMN IF NOT EXISTS ledger jsonb NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN realmguard_results.ledger IS
  'Recorded player input the server replayed to derive this result.';

CREATE INDEX IF NOT EXISTS realmguard_results_verification_idx
  ON realmguard_results(verification_method, created_at DESC);
