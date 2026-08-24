-- RealmGuard content 0.3.0: tower branch retune.
--
-- Measured against the shipped roster, the branch choices were not choices.
-- stonepulse/ember_core led damage per gold by 13% over the next branch while
-- also carrying splash, so nothing competed with it; quake_drum and
-- star_lattice trailed the alternative inside their own tower by 24-69%; and
-- windward/shield_line was worse than not upgrading at all — 0.174 damage per
-- gold against 0.188 for the un-upgraded tower — while its slow value of 0.68
-- made the enemy *faster* than the tower's own 0.52 default, the opposite of
-- the "강력한 지상 저지" it promises.
--
-- The new pack is derived from the published snapshot rather than regenerated,
-- so the procedurally built stages and waves stay byte-identical and only the
-- four numbers move.
--
-- It applies only when the published pack is still the untouched canonical
-- seed. An operator who has published their own pack through the Designer owns
-- that content, and this migration leaves it alone.
DO $$
DECLARE
  seed         realmguard_content_versions%ROWTYPE;
  new_towers   jsonb;
  new_content  jsonb;
  next_version integer;
BEGIN
  SELECT * INTO seed FROM realmguard_content_versions
   WHERE status = 'published' AND version_no = 1 AND label = 'v0.2.0' AND content_version = '0.2.0';

  IF NOT FOUND THEN
    RAISE NOTICE 'RealmGuard: published content is not the canonical v0.2.0 seed; leaving it untouched.';
    RETURN;
  END IF;

  -- Fail loudly rather than retune something that is not what this migration
  -- was written against.
  IF NOT (
    jsonb_path_exists(seed.content, '$.towers[*].branches[*] ? (@.id == "star_lattice" && @.damage_multiplier == 1.4)')
    AND jsonb_path_exists(seed.content, '$.towers[*].branches[*] ? (@.id == "quake_drum" && @.damage_multiplier == 1.3)')
    AND jsonb_path_exists(seed.content, '$.towers[*].branches[*] ? (@.id == "ember_core" && @.damage_multiplier == 2.2)')
    AND jsonb_path_exists(seed.content, '$.towers[*].branches[*] ? (@.id == "shield_line" && @.slow == 0.68)')
  ) THEN
    RAISE EXCEPTION 'RealmGuard v0.2.0 seed does not carry the branch values this retune expects';
  END IF;

  SELECT jsonb_agg(
           CASE WHEN tower.value ? 'branches'
             THEN jsonb_set(tower.value, '{branches}', (
               SELECT jsonb_agg(
                        CASE branch.value->>'id'
                          WHEN 'star_lattice' THEN jsonb_set(branch.value, '{damage_multiplier}', '1.55')
                          WHEN 'quake_drum'   THEN jsonb_set(branch.value, '{damage_multiplier}', '1.5')
                          WHEN 'ember_core'   THEN jsonb_set(branch.value, '{damage_multiplier}', '1.9')
                          WHEN 'shield_line'  THEN jsonb_set(branch.value, '{slow}', '0.34')
                          ELSE branch.value
                        END
                        ORDER BY branch.ordinality)
               FROM jsonb_array_elements(tower.value->'branches') WITH ORDINALITY AS branch(value, ordinality)
             ))
             ELSE tower.value
           END
           ORDER BY tower.ordinality)
    INTO new_towers
    FROM jsonb_array_elements(seed.content->'towers') WITH ORDINALITY AS tower(value, ordinality);

  -- The retune must move values only: same towers, same branches, same order.
  IF jsonb_array_length(new_towers) <> jsonb_array_length(seed.content->'towers')
     OR (SELECT jsonb_agg(value->>'id' ORDER BY ordinality) FROM jsonb_array_elements(new_towers) WITH ORDINALITY AS x(value, ordinality))
        IS DISTINCT FROM
        (SELECT jsonb_agg(value->>'id' ORDER BY ordinality) FROM jsonb_array_elements(seed.content->'towers') WITH ORDINALITY AS y(value, ordinality))
  THEN
    RAISE EXCEPTION 'RealmGuard retune changed the tower roster';
  END IF;

  new_content := jsonb_set(seed.content, '{towers}', new_towers);

  SELECT COALESCE(max(version_no), 0) + 1 INTO next_version FROM realmguard_content_versions;

  -- Only one row may be published at a time.
  UPDATE realmguard_content_versions SET status = 'archived', updated_at = now() WHERE id = seed.id;

  INSERT INTO realmguard_content_versions(
    version_no, label, status, content_version, stage_version, balance_version, asset_version,
    checksum, notes, content, published_at)
  VALUES (
    next_version, 'v0.3.0', 'published', '0.3.0', seed.stage_version, '2026.08.2', seed.asset_version,
    encode(digest(convert_to(new_content::text, 'UTF8'), 'sha256'), 'hex'),
    'RealmGuard 0.3.0 tower branch retune: shield_line slow corrected, ember_core reduced, quake_drum and star_lattice raised',
    new_content, now());

  -- The catalogue entry carries the game's content version, as it did when the
  -- v0.2.0 pack was seeded.
  UPDATE games SET version = '0.3.0', updated_at = now()
   WHERE slug = 'realmguard' AND version = '0.2.0';
END $$;
