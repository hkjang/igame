-- now() is the transaction's start time, not the moment a row is written, and
-- the attestation compares these two columns as if they were receipt times.
--
-- Measured on a single passing run: of 355 telemetry rows, three carried a
-- received_at earlier than the row inserted before them, the worst by 1197ms.
-- The battle attestation allows a telemetry row to precede its session by one
-- second, so an honest result was refused whenever a transaction happened to
-- open more than a second before its insert landed.
--
-- clock_timestamp() is the reading at the moment of the statement. Each of
-- these rows is written by its own request, so nothing wanted them to share a
-- transaction's timestamp in the first place.
ALTER TABLE game_telemetry ALTER COLUMN received_at SET DEFAULT clock_timestamp();
ALTER TABLE game_sessions ALTER COLUMN started_at SET DEFAULT clock_timestamp();
