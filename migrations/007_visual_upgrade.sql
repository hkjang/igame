-- Visual content upgrade for RealmGuard and the Defense Series.
--
-- Published content is an immutable, session-pinned snapshot.  Consequently
-- this migration never edits a published document in place: it archives the
-- exact canonical predecessor and publishes a derived successor.  A Designer
-- pack (or any other non-canonical published version) remains untouched.

DO $realmguard_visual_upgrade$
DECLARE
  seed               realmguard_content_versions%ROWTYPE;
  next_version       integer;
  calculated_checksum text;
BEGIN
  SELECT * INTO seed
  FROM realmguard_content_versions
  WHERE status = 'published'
    AND label = 'v0.3.0'
    AND content_version = '0.3.0'
    AND asset_version = 'procedural-1'
  ORDER BY version_no DESC
  LIMIT 1;

  IF NOT FOUND THEN
    RAISE NOTICE 'RealmGuard: published content is not the canonical v0.3.0/procedural-1 snapshot; leaving it untouched.';
    RETURN;
  END IF;

  calculated_checksum := encode(digest(convert_to(seed.content::text, 'UTF8'), 'sha256'), 'hex');
  IF seed.checksum IS DISTINCT FROM calculated_checksum THEN
    RAISE EXCEPTION 'RealmGuard v0.3.0 checksum does not match its immutable content';
  END IF;

  -- v0.3.0 was produced by migration 006.  Guard its characteristic shape and
  -- retuned values before assigning the new procedural asset contract.
  IF jsonb_typeof(seed.content) IS DISTINCT FROM 'object'
     OR jsonb_typeof(seed.content->'stages') IS DISTINCT FROM 'array'
     OR jsonb_typeof(seed.content->'waves') IS DISTINCT FROM 'array'
     OR jsonb_typeof(seed.content->'towers') IS DISTINCT FROM 'array'
     OR jsonb_array_length(seed.content->'stages') <> 11
     OR jsonb_array_length(seed.content->'towers') <> 4
     OR NOT jsonb_path_exists(seed.content, '$.towers[*].branches[*] ? (@.id == "star_lattice" && @.damage_multiplier == 1.55)')
     OR NOT jsonb_path_exists(seed.content, '$.towers[*].branches[*] ? (@.id == "quake_drum" && @.damage_multiplier == 1.5)')
     OR NOT jsonb_path_exists(seed.content, '$.towers[*].branches[*] ? (@.id == "ember_core" && @.damage_multiplier == 1.9)')
     OR NOT jsonb_path_exists(seed.content, '$.towers[*].branches[*] ? (@.id == "shield_line" && @.slow == 0.34)')
  THEN
    RAISE EXCEPTION 'RealmGuard v0.3.0 snapshot does not have the canonical migration-006 shape';
  END IF;

  SELECT COALESCE(max(version_no), 0) + 1 INTO next_version
  FROM realmguard_content_versions;

  UPDATE realmguard_content_versions
  SET status = 'archived', updated_at = now()
  WHERE id = seed.id AND status = 'published';

  IF NOT FOUND THEN
    RAISE EXCEPTION 'RealmGuard v0.3.0 changed while the visual upgrade was running';
  END IF;

  INSERT INTO realmguard_content_versions(
    version_no, label, status, content_version, stage_version,
    balance_version, asset_version, checksum, notes, content, published_at)
  VALUES (
    next_version, 'v0.3.1', 'published', '0.3.1', seed.stage_version,
    seed.balance_version, 'procedural-2', calculated_checksum,
    'RealmGuard v0.3.1 visual upgrade: canonical v0.3.0 gameplay content retained with procedural character assets v2',
    seed.content, now());

  UPDATE games SET version = '0.3.1', updated_at = now()
  WHERE slug = 'realmguard';
END
$realmguard_visual_upgrade$;

DO $defense_visual_upgrade$
DECLARE
  target_slug          text;
  target_game_id       uuid;
  seed                 defense_content_versions%ROWTYPE;
  next_version         integer;
  expected_stage_count integer;
  new_stages           jsonb;
  new_waves            jsonb;
  new_content          jsonb;
  calculated_checksum  text;
  old_path             jsonb := '[{"x":-30,"y":360},{"x":180,"y":360},{"x":310,"y":130},{"x":530,"y":130},{"x":650,"y":500},{"x":880,"y":500},{"x":1020,"y":260},{"x":1310,"y":260}]'::jsonb;
  old_spots            jsonb := '[{"x":140,"y":240},{"x":300,"y":280},{"x":430,"y":450},{"x":570,"y":250},{"x":700,"y":420},{"x":850,"y":620},{"x":970,"y":430},{"x":1130,"y":180}]'::jsonb;
  geometries           jsonb := '[
    {
      "paths":[[{"x":-40,"y":190},{"x":240,"y":190},{"x":240,"y":505},{"x":535,"y":505},{"x":535,"y":235},{"x":880,"y":235},{"x":880,"y":455},{"x":1320,"y":455}]],
      "tower_spots":[{"x":105,"y":300},{"x":335,"y":105},{"x":355,"y":390},{"x":440,"y":610},{"x":650,"y":390},{"x":745,"y":125},{"x":985,"y":330},{"x":1120,"y":570}]
    },
    {
      "paths":[[{"x":-40,"y":570},{"x":170,"y":570},{"x":345,"y":365},{"x":520,"y":155},{"x":760,"y":155},{"x":905,"y":350},{"x":1085,"y":560},{"x":1320,"y":560}]],
      "tower_spots":[{"x":95,"y":445},{"x":255,"y":660},{"x":345,"y":210},{"x":485,"y":480},{"x":640,"y":270},{"x":790,"y":65},{"x":925,"y":505},{"x":1120,"y":380}]
    },
    {
      "paths":[
        [{"x":-40,"y":185},{"x":220,"y":185},{"x":420,"y":360},{"x":655,"y":360},{"x":850,"y":185},{"x":1060,"y":185},{"x":1320,"y":360}],
        [{"x":-40,"y":535},{"x":220,"y":535},{"x":420,"y":360},{"x":655,"y":360},{"x":850,"y":535},{"x":1060,"y":535},{"x":1320,"y":360}]
      ],
      "tower_spots":[{"x":105,"y":80},{"x":105,"y":640},{"x":315,"y":330},{"x":505,"y":245},{"x":575,"y":475},{"x":750,"y":360},{"x":930,"y":345},{"x":1135,"y":360}]
    },
    {
      "paths":[[{"x":-40,"y":125},{"x":1090,"y":125},{"x":1090,"y":590},{"x":180,"y":590},{"x":180,"y":350},{"x":1320,"y":350}]],
      "tower_spots":[{"x":120,"y":235},{"x":335,"y":45},{"x":570,"y":230},{"x":805,"y":45},{"x":1000,"y":245},{"x":940,"y":485},{"x":600,"y":675},{"x":320,"y":455}]
    },
    {
      "paths":[[{"x":-40,"y":610},{"x":245,"y":610},{"x":245,"y":420},{"x":500,"y":420},{"x":500,"y":185},{"x":760,"y":185},{"x":760,"y":495},{"x":1040,"y":495},{"x":1040,"y":250},{"x":1320,"y":250}]],
      "tower_spots":[{"x":105,"y":495},{"x":345,"y":520},{"x":355,"y":305},{"x":610,"y":310},{"x":650,"y":80},{"x":875,"y":380},{"x":900,"y":610},{"x":1150,"y":365}]
    },
    {
      "paths":[
        [{"x":-40,"y":160},{"x":270,"y":160},{"x":425,"y":315},{"x":650,"y":315},{"x":810,"y":160},{"x":1060,"y":160},{"x":1320,"y":315}],
        [{"x":-40,"y":560},{"x":270,"y":560},{"x":425,"y":405},{"x":650,"y":405},{"x":810,"y":560},{"x":1060,"y":560},{"x":1320,"y":405}]
      ],
      "tower_spots":[{"x":120,"y":300},{"x":305,"y":60},{"x":305,"y":660},{"x":525,"y":210},{"x":525,"y":510},{"x":735,"y":365},{"x":940,"y":350},{"x":1150,"y":280}]
    },
    {
      "paths":[[{"x":-40,"y":95},{"x":190,"y":170},{"x":345,"y":540},{"x":555,"y":620},{"x":745,"y":295},{"x":910,"y":105},{"x":1080,"y":410},{"x":1320,"y":520}]],
      "tower_spots":[{"x":110,"y":285},{"x":275,"y":65},{"x":200,"y":450},{"x":500,"y":400},{"x":650,"y":560},{"x":785,"y":90},{"x":955,"y":290},{"x":1155,"y":610}]
    },
    {
      "paths":[[{"x":-40,"y":180},{"x":235,"y":180},{"x":480,"y":360},{"x":725,"y":540},{"x":980,"y":540},{"x":1080,"y":360},{"x":980,"y":180},{"x":725,"y":180},{"x":480,"y":360},{"x":1320,"y":360}]],
      "tower_spots":[{"x":105,"y":315},{"x":330,"y":70},{"x":400,"y":500},{"x":570,"y":220},{"x":650,"y":550},{"x":820,"y":650},{"x":890,"y":295},{"x":1150,"y":495}]
    },
    {
      "paths":[
        [{"x":-40,"y":95},{"x":260,"y":95},{"x":420,"y":300},{"x":640,"y":360},{"x":860,"y":300},{"x":1040,"y":95},{"x":1320,"y":250}],
        [{"x":-40,"y":625},{"x":260,"y":625},{"x":420,"y":420},{"x":640,"y":360},{"x":860,"y":420},{"x":1040,"y":625},{"x":1320,"y":470}]
      ],
      "tower_spots":[{"x":120,"y":240},{"x":120,"y":480},{"x":330,"y":360},{"x":500,"y":205},{"x":500,"y":515},{"x":755,"y":185},{"x":755,"y":535},{"x":1060,"y":360}]
    },
    {
      "paths":[[{"x":-40,"y":360},{"x":175,"y":360},{"x":175,"y":105},{"x":1035,"y":105},{"x":1035,"y":615},{"x":370,"y":615},{"x":370,"y":285},{"x":790,"y":285},{"x":790,"y":465},{"x":1320,"y":465}]],
      "tower_spots":[{"x":75,"y":230},{"x":315,"y":205},{"x":530,"y":35},{"x":815,"y":205},{"x":945,"y":370},{"x":900,"y":670},{"x":535,"y":470},{"x":1115,"y":575}]
    }
  ]'::jsonb;
  identities           jsonb := '{
    "office-guardians":[
      {"style":"office-plaza","theme":"verdant"},
      {"style":"office-cloud","theme":"frost"},
      {"style":"office-datacenter","theme":"void"},
      {"style":"office-deploy","theme":"ember"},
      {"style":"office-api","theme":"frost"},
      {"style":"office-data","theme":"verdant"},
      {"style":"office-legacy","theme":"ember"},
      {"style":"office-crisis","theme":"void"}
    ],
    "cyber-fortress":[
      {"style":"cyber-mail","theme":"frost"},
      {"style":"cyber-identity","theme":"void"},
      {"style":"cyber-endpoint","theme":"ember"},
      {"style":"cyber-web","theme":"frost"},
      {"style":"cyber-ddos","theme":"void"},
      {"style":"cyber-insider","theme":"verdant"},
      {"style":"cyber-dlp","theme":"ember"},
      {"style":"cyber-supply","theme":"verdant"},
      {"style":"cyber-zero-day","theme":"void"},
      {"style":"cyber-vault","theme":"frost"}
    ],
    "ai-nexus-defense":[
      {"style":"ai-prompt","theme":"void"},
      {"style":"ai-rag","theme":"verdant"},
      {"style":"ai-agent","theme":"frost"},
      {"style":"ai-guardrail","theme":"ember"},
      {"style":"ai-routing","theme":"void"},
      {"style":"ai-review","theme":"verdant"},
      {"style":"ai-enterprise","theme":"frost"},
      {"style":"ai-context","theme":"ember"},
      {"style":"ai-trust","theme":"verdant"},
      {"style":"ai-core","theme":"void"}
    ]
  }'::jsonb;
BEGIN
  -- These constants mirror web/src/games/defense/maps.ts.  Validate the
  -- catalogue itself so a transcription error cannot publish a partial pack.
  IF jsonb_array_length(geometries) <> 10
     OR (SELECT count(DISTINCT geometry.value->'paths')
         FROM jsonb_array_elements(geometries) AS geometry(value)) <> 10
     OR EXISTS (
       SELECT 1 FROM jsonb_array_elements(geometries) AS geometry(value)
       WHERE jsonb_typeof(geometry.value->'paths') IS DISTINCT FROM 'array'
          OR jsonb_typeof(geometry.value->'tower_spots') IS DISTINCT FROM 'array'
          OR jsonb_array_length(geometry.value->'paths') NOT BETWEEN 1 AND 2
          OR jsonb_array_length(geometry.value->'tower_spots') <> 8
     )
  THEN
    RAISE EXCEPTION 'Defense v0.4.0 map catalogue is incomplete or contains duplicate geometry';
  END IF;

  FOREACH target_slug IN ARRAY ARRAY['office-guardians', 'cyber-fortress', 'ai-nexus-defense']
  LOOP
    SELECT id INTO target_game_id FROM games WHERE slug = target_slug;
    IF NOT FOUND THEN
      RAISE NOTICE 'Defense visual upgrade: game % is not installed; skipping it.', target_slug;
      CONTINUE;
    END IF;

    SELECT * INTO seed
    FROM defense_content_versions
    WHERE game_id = target_game_id
      AND status = 'published'
      AND version_no = 1
      AND label = 'v0.3.0'
      AND content_version = '0.3.0'
      AND asset_version = 'procedural-1';

    IF NOT FOUND THEN
      RAISE NOTICE 'Defense visual upgrade: % is not on the published canonical v0.3.0/procedural-1 seed; leaving it untouched.', target_slug;
      CONTINUE;
    END IF;

    calculated_checksum := encode(digest(convert_to(seed.content::text, 'UTF8'), 'sha256'), 'hex');
    IF seed.checksum IS DISTINCT FROM calculated_checksum THEN
      RAISE EXCEPTION 'Defense % v0.3.0 checksum does not match its immutable content', target_slug;
    END IF;

    expected_stage_count := jsonb_array_length(identities->target_slug);

    IF jsonb_typeof(seed.content) IS DISTINCT FROM 'object'
       OR seed.content->>'schema_version' IS DISTINCT FROM '0.3.0'
       OR jsonb_typeof(seed.content->'stages') IS DISTINCT FROM 'array'
       OR jsonb_typeof(seed.content->'waves') IS DISTINCT FROM 'array'
    THEN
      RAISE EXCEPTION 'Defense % v0.3.0 seed does not contain canonical stage and wave arrays', target_slug;
    END IF;

    IF jsonb_array_length(seed.content->'stages') <> expected_stage_count
       OR jsonb_array_length(seed.content->'waves') <> expected_stage_count * 8
    THEN
      RAISE EXCEPTION 'Defense % v0.3.0 seed has an unexpected stage or wave count', target_slug;
    END IF;

    -- Migration 005 gave every canonical stage this exact shared layout.  The
    -- strict check distinguishes that seed from an operator-authored pack even
    -- if its version metadata was accidentally copied.
    IF EXISTS (
      SELECT 1
      FROM jsonb_array_elements(seed.content->'stages') WITH ORDINALITY AS stage(value, ordinality)
      WHERE stage.value->>'id' IS DISTINCT FROM 'stage-' || stage.ordinality
         OR stage.value->>'number' IS DISTINCT FROM stage.ordinality::text
         OR stage.value->>'version' IS DISTINCT FROM '3.' || stage.ordinality || '.0'
         OR stage.value->'path' IS DISTINCT FROM old_path
         OR stage.value ? 'paths'
         OR stage.value ? 'map_style'
         OR jsonb_typeof(stage.value->'tower_spots') IS DISTINCT FROM 'array'
         OR stage.value->'tower_spots' IS DISTINCT FROM (
           SELECT jsonb_agg(
             jsonb_build_object(
               'id', stage.value->>'id' || '-spot-' || spot.ordinality,
               'x', spot.value->'x',
               'y', spot.value->'y')
             ORDER BY spot.ordinality)
           FROM jsonb_array_elements(old_spots) WITH ORDINALITY AS spot(value, ordinality)
         )
    )
    THEN
      RAISE EXCEPTION 'Defense % v0.3.0 stage geometry is not the canonical migration-005 seed', target_slug;
    END IF;

    IF EXISTS (
      SELECT 1
      FROM jsonb_array_elements(seed.content->'waves') WITH ORDINALITY AS wave(value, ordinality)
      WHERE wave.value->>'id' IS DISTINCT FROM
              'stage-' || (((wave.ordinality - 1) / 8) + 1) || '-wave-' || (((wave.ordinality - 1) % 8) + 1)
         OR wave.value->>'stage_id' IS DISTINCT FROM 'stage-' || (((wave.ordinality - 1) / 8) + 1)
         OR wave.value->>'number' IS DISTINCT FROM (((wave.ordinality - 1) % 8) + 1)::text
         OR jsonb_typeof(wave.value->'entries') IS DISTINCT FROM 'array'
         OR jsonb_array_length(wave.value->'entries') NOT BETWEEN 2 AND 3
    )
    THEN
      RAISE EXCEPTION 'Defense % v0.3.0 wave ordering is not the canonical migration-005 seed', target_slug;
    END IF;

    IF EXISTS (
      SELECT 1
      FROM jsonb_array_elements(seed.content->'waves') AS wave(value)
      CROSS JOIN LATERAL jsonb_array_elements(wave.value->'entries') WITH ORDINALITY AS entry(value, ordinality)
      WHERE entry.ordinality <= 2
        AND entry.value ?| ARRAY['path_index', 'parallel', 'delay']
    )
    THEN
      RAISE EXCEPTION 'Defense % v0.3.0 wave lanes were already customized; leaving the snapshot immutable', target_slug;
    END IF;

    SELECT jsonb_agg(
             stage.value || jsonb_build_object(
               'theme', map.identity->>'theme',
               'map_style', map.identity->>'style',
               'path', (map.geometry->'paths')->0,
               'paths', map.geometry->'paths',
               'tower_spots', (
                 SELECT jsonb_agg(
                   jsonb_build_object(
                     'id', stage.value->>'id' || '-spot-' || spot.ordinality,
                     'x', spot.value->'x',
                     'y', spot.value->'y')
                   ORDER BY spot.ordinality)
                 FROM jsonb_array_elements(map.geometry->'tower_spots')
                      WITH ORDINALITY AS spot(value, ordinality)
               ),
               'version', '4.' || stage.ordinality || '.0')
             ORDER BY stage.ordinality)
    INTO new_stages
    FROM jsonb_array_elements(seed.content->'stages') WITH ORDINALITY AS stage(value, ordinality)
    CROSS JOIN LATERAL (
      SELECT identities->target_slug->((stage.ordinality - 1)::integer) AS identity,
             geometries->((stage.ordinality - 1)::integer) AS geometry
    ) AS map;

    SELECT jsonb_agg(
             CASE WHEN lanes.lane_count > 1 THEN
               wave.value || jsonb_build_object(
                 'entries', (
                   SELECT jsonb_agg(
                     CASE entry.ordinality
                       WHEN 1 THEN entry.value || jsonb_build_object(
                         'path_index', mod((wave.value->>'number')::integer - 1, lanes.lane_count))
                       WHEN 2 THEN entry.value || jsonb_build_object(
                         'path_index', mod((wave.value->>'number')::integer, lanes.lane_count),
                         'parallel', true,
                         'delay', 1.4)
                       ELSE entry.value
                     END
                     ORDER BY entry.ordinality)
                   FROM jsonb_array_elements(wave.value->'entries')
                        WITH ORDINALITY AS entry(value, ordinality)
                 ))
               ELSE wave.value
             END
             ORDER BY wave.ordinality)
    INTO new_waves
    FROM jsonb_array_elements(seed.content->'waves') WITH ORDINALITY AS wave(value, ordinality)
    JOIN LATERAL (
      SELECT stage.ordinality
      FROM jsonb_array_elements(seed.content->'stages') WITH ORDINALITY AS stage(value, ordinality)
      WHERE stage.value->>'id' = wave.value->>'stage_id'
    ) AS stage ON true
    CROSS JOIN LATERAL (
      SELECT jsonb_array_length(
        (geometries->((stage.ordinality - 1)::integer))->'paths') AS lane_count
    ) AS lanes;

    -- Only the declared stage presentation keys and multi-lane routing keys
    -- may differ.  Enemy identities, counts, entry order, rewards, policy,
    -- education, balance, heroes, towers and every other field are retained.
    IF EXISTS (
      SELECT 1
      FROM jsonb_array_elements(seed.content->'stages') WITH ORDINALITY AS old_stage(value, ordinality)
      JOIN jsonb_array_elements(new_stages) WITH ORDINALITY AS upgraded(value, ordinality)
        USING (ordinality)
      WHERE old_stage.value - ARRAY['theme', 'map_style', 'path', 'paths', 'tower_spots', 'version']::text[]
            IS DISTINCT FROM
            upgraded.value - ARRAY['theme', 'map_style', 'path', 'paths', 'tower_spots', 'version']::text[]
    ) OR EXISTS (
      SELECT 1
      FROM jsonb_array_elements(seed.content->'waves') WITH ORDINALITY AS old_wave(value, ordinality)
      JOIN jsonb_array_elements(new_waves) WITH ORDINALITY AS upgraded(value, ordinality)
        USING (ordinality)
      WHERE old_wave.value - 'entries' IS DISTINCT FROM upgraded.value - 'entries'
         OR jsonb_array_length(old_wave.value->'entries') <> jsonb_array_length(upgraded.value->'entries')
         OR EXISTS (
           SELECT 1
           FROM jsonb_array_elements(old_wave.value->'entries') WITH ORDINALITY AS old_entry(value, entry_ordinality)
           JOIN jsonb_array_elements(upgraded.value->'entries') WITH ORDINALITY AS upgraded_entry(value, entry_ordinality)
             USING (entry_ordinality)
           WHERE old_entry.value - ARRAY['path_index', 'parallel', 'delay']::text[]
                 IS DISTINCT FROM
                 upgraded_entry.value - ARRAY['path_index', 'parallel', 'delay']::text[]
         )
    )
    THEN
      RAISE EXCEPTION 'Defense % v0.4.0 transform changed protected gameplay content', target_slug;
    END IF;

    -- Assert the exact maps and lane assignment after transformation.
    IF (SELECT count(DISTINCT stage.value->>'map_style')
        FROM jsonb_array_elements(new_stages) AS stage(value)) <> expected_stage_count
       OR (SELECT count(DISTINCT stage.value->'paths')
           FROM jsonb_array_elements(new_stages) AS stage(value)) <> expected_stage_count
       OR EXISTS (
         SELECT 1
         FROM jsonb_array_elements(new_stages) WITH ORDINALITY AS stage(value, ordinality)
         CROSS JOIN LATERAL (
           SELECT identities->target_slug->((stage.ordinality - 1)::integer) AS identity,
                  geometries->((stage.ordinality - 1)::integer) AS geometry
         ) AS map
         WHERE stage.value->>'theme' IS DISTINCT FROM map.identity->>'theme'
            OR stage.value->>'map_style' IS DISTINCT FROM map.identity->>'style'
            OR stage.value->>'version' IS DISTINCT FROM '4.' || stage.ordinality || '.0'
            OR stage.value->'path' IS DISTINCT FROM (map.geometry->'paths')->0
            OR stage.value->'paths' IS DISTINCT FROM map.geometry->'paths'
       )
       OR EXISTS (
         SELECT 1
         FROM jsonb_array_elements(new_waves) AS wave(value)
         JOIN LATERAL (
           SELECT stage.ordinality
           FROM jsonb_array_elements(new_stages) WITH ORDINALITY AS stage(value, ordinality)
           WHERE stage.value->>'id' = wave.value->>'stage_id'
         ) AS stage ON true
         CROSS JOIN LATERAL (
           SELECT jsonb_array_length(stage_doc.value->'paths') AS lane_count
           FROM jsonb_array_elements(new_stages) WITH ORDINALITY AS stage_doc(value, ordinality)
           WHERE stage_doc.ordinality = stage.ordinality
         ) AS lanes
         WHERE lanes.lane_count > 1 AND (
           (wave.value->'entries'->0->'path_index') IS DISTINCT FROM
             to_jsonb(mod((wave.value->>'number')::integer - 1, lanes.lane_count))
           OR (wave.value->'entries'->1->'path_index') IS DISTINCT FROM
             to_jsonb(mod((wave.value->>'number')::integer, lanes.lane_count))
           OR (wave.value->'entries'->1->'parallel') IS DISTINCT FROM 'true'::jsonb
           OR (wave.value->'entries'->1->'delay') IS DISTINCT FROM '1.4'::jsonb
         )
       )
    THEN
      RAISE EXCEPTION 'Defense % v0.4.0 map or multi-lane wave assertion failed', target_slug;
    END IF;

    new_content := seed.content || jsonb_build_object(
      'schema_version', '0.4.0',
      'stages', new_stages,
      'waves', new_waves);

    IF seed.content - ARRAY['schema_version', 'stages', 'waves']::text[]
       IS DISTINCT FROM
       new_content - ARRAY['schema_version', 'stages', 'waves']::text[]
    THEN
      RAISE EXCEPTION 'Defense % v0.4.0 content clone changed protected root fields', target_slug;
    END IF;

    calculated_checksum := encode(digest(convert_to(new_content::text, 'UTF8'), 'sha256'), 'hex');
    SELECT COALESCE(max(version_no), 0) + 1 INTO next_version
    FROM defense_content_versions
    WHERE game_id = target_game_id;

    UPDATE defense_content_versions
    SET status = 'archived', updated_at = now()
    WHERE id = seed.id AND status = 'published';

    IF NOT FOUND THEN
      RAISE EXCEPTION 'Defense % v0.3.0 changed while the visual upgrade was running', target_slug;
    END IF;

    INSERT INTO defense_content_versions(
      game_id, version_no, label, status, content_version, policy_version,
      asset_version, checksum, notes, content, source_version_id, published_at)
    VALUES (
      target_game_id, next_version, 'v0.4.0', 'published', '0.4.0', seed.policy_version,
      'procedural-defense-2', calculated_checksum,
      'Defense Series v0.4.0 tactical map upgrade: unique stage geometry, named map styles, stage-specific tower spots, and validated multi-lane routing; gameplay roster, balance, education, and policy retained',
      new_content, seed.id, now());

    UPDATE games SET version = '0.4.0', updated_at = now()
    WHERE id = target_game_id;
  END LOOP;
END
$defense_visual_upgrade$;
