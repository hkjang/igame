ALTER TABLE realmguard_results
  ADD COLUMN verification_method text NOT NULL DEFAULT 'aggregate_bounds_v1',
  ADD COLUMN attestation jsonb NOT NULL DEFAULT '{}'::jsonb;

-- Economy bounds are intentionally larger than PostgreSQL integer because a
-- published endless content pack can legitimately accumulate more than 2^31
-- gold while still remaining within the server's validated int64 envelope.
ALTER TABLE realmguard_results
  ALTER COLUMN remaining_gold TYPE bigint,
  ALTER COLUMN earned_gold TYPE bigint,
  ALTER COLUMN spent_gold TYPE bigint,
  ALTER COLUMN sold_gold TYPE bigint;

-- Client-supplied proof/event blobs were never authoritative. Preserve the
-- columns for schema compatibility, but clear legacy input and reserve proof
-- for a server-minted AES-GCM receipt from this migration onward.
UPDATE realmguard_results SET proof='', events='[]'::jsonb;

ALTER TABLE game_telemetry
  ADD COLUMN client_event_id uuid,
  ADD COLUMN sequence_no integer;

CREATE UNIQUE INDEX game_telemetry_session_client_event_idx
  ON game_telemetry(session_id,client_event_id)
  WHERE client_event_id IS NOT NULL;
CREATE UNIQUE INDEX game_telemetry_session_sequence_idx
  ON game_telemetry(session_id,sequence_no)
  WHERE sequence_no IS NOT NULL;

COMMENT ON COLUMN realmguard_results.verification_method IS
  'Server verification profile applied to the result.';
COMMENT ON COLUMN realmguard_results.attestation IS
  'Server-generated summary of authenticated telemetry received during play.';
COMMENT ON COLUMN realmguard_results.proof IS
  'AES-GCM server receipt; client-supplied proof is never persisted.';
COMMENT ON COLUMN realmguard_results.events IS
  'Reserved compatibility field; untrusted client event bundles are never persisted.';

CREATE INDEX game_telemetry_session_event_received_idx
  ON game_telemetry(session_id,event,received_at,id);

-- Split profile mutation from profile reads for personal API keys. Existing
-- keys keep their stored scopes, while administrators can explicitly grant
-- the new write capability after this migration.
UPDATE system_settings
SET value = jsonb_set(
  jsonb_set(
    jsonb_set(
      jsonb_set(
        jsonb_set(value,
          '{available_permissions}',
          COALESCE(value->'available_permissions','[]'::jsonb) || '["profile:write"]'::jsonb),
        '{role_permissions,user}',
        COALESCE(value#>'{role_permissions,user}','[]'::jsonb) || '["profile:write"]'::jsonb),
      '{role_permissions,manager}',
      COALESCE(value#>'{role_permissions,manager}','[]'::jsonb) || '["profile:write"]'::jsonb),
    '{role_permissions,operator}',
    COALESCE(value#>'{role_permissions,operator}','[]'::jsonb) || '["profile:write"]'::jsonb),
  '{role_permissions,admin}',
  COALESCE(value#>'{role_permissions,admin}','[]'::jsonb) || '["profile:write"]'::jsonb)
WHERE key='api_keys' AND NOT (COALESCE(value->'available_permissions','[]'::jsonb) ? 'profile:write');
