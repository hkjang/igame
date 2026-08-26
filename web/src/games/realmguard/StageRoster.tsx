import { alpha } from '@mui/material/styles';
import { Box, Card, Chip, Stack, Tooltip, Typography } from '@mui/material';
import { EnemyPortrait } from './EnemyPortrait';
import { resolveEnemyPresentation } from './enemyPresentation';
import type { HeroPresentationGame } from './heroPresentation';
import type { EnemyArchetype, RealmStage } from './types';

/** What a trait means for the player, and what to do about it. */
const TRAIT_COPY: Record<string, { label: string; hint: string }> = {
  armored: { label: '방어', hint: '물리 피해를 크게 줄입니다. 다른 피해 유형으로 상대하세요.' },
  flying: { label: '비행', hint: '지상 저지를 지나칩니다. 공중을 잡을 수 있는 배치가 필요합니다.' },
  swift: { label: '신속', hint: '이동이 빠릅니다. 둔화나 길목 저지가 유효합니다.' },
  regenerating: { label: '재생', hint: '스스로 체력을 회복합니다. 지속 화력으로 끊어내세요.' },
  healer: { label: '치유', hint: '주변 아군을 회복시킵니다. 먼저 처치하세요.' },
  splitting: { label: '분열', hint: '쓰러지면 작은 개체로 나뉩니다. 광역 화력을 준비하세요.' },
  phasing: { label: '위상', hint: '일부 공격을 그대로 통과시킵니다.' },
  siege: { label: '공성', hint: '가까운 타워를 잠시 무력화합니다.' },
  magic_resist: { label: '마법 저항', hint: '비전 피해를 크게 줄입니다.' },
  stealth: { label: '은신', hint: '경로 초반에는 짧은 사거리에서만 포착됩니다.' },
  berserk: { label: '광폭', hint: '체력이 낮아지면 더 빨라집니다.' },
  immune_stun: { label: '둔화 면역', hint: '둔화와 정지가 통하지 않습니다.' },
  boss: { label: '보스', hint: '체력 구간마다 전장을 바꿉니다.' },
};

/**
 * The creatures a stage actually sends, read from its own wave table.
 *
 * The battlefield now says what an enemy does through its shape and its marks,
 * but nothing taught that vocabulary. This is where it is taught, next to the
 * decision it informs: which towers and which hero to bring.
 */
export function StageRoster({
  stage,
  enemies,
  game = 'realmguard',
  noun = '적',
}: {
  stage: RealmStage;
  enemies: EnemyArchetype[];
  game?: HeroPresentationGame;
  /** What this game calls the things it sends: 적, 사이버 위협, 업무 위협. */
  noun?: string;
}) {
  const byId = new Map(enemies.map((enemy) => [enemy.id, enemy]));
  const seen = new Set<string>();
  const roster: EnemyArchetype[] = [];
  for (const wave of stage.waves) {
    for (const entry of wave.entries) {
      if (seen.has(entry.enemy)) continue;
      seen.add(entry.enemy);
      const enemy = byId.get(entry.enemy);
      if (enemy) roster.push(enemy);
    }
  }
  if (roster.length === 0) return null;

  return (
    <Box mt={3}>
      <Typography variant="h3" mb={.5}>이 전장의 {noun}</Typography>
      <Typography color="text.secondary" mb={1.5}>
        {stage.name}의 파동에 등장하는 개체입니다. 전장에서도 같은 모습과 표식으로 나타납니다.
      </Typography>
      <Stack direction="row" flexWrap="wrap" useFlexGap spacing={1.2}>
        {roster.map((enemy) => {
          const presentation = resolveEnemyPresentation(enemy.id, enemy.traits, game);
          const traits = enemy.traits.filter((trait) => TRAIT_COPY[trait]);
          return (
            <Card
              key={enemy.id}
              variant="outlined"
              sx={(theme) => ({
                p: 1.2,
                display: 'flex',
                gap: 1.2,
                alignItems: 'center',
                minWidth: 232,
                flex: '1 1 232px',
                bgcolor: alpha(theme.palette.surface.sunken, .55),
              })}
            >
              <EnemyPortrait presentation={presentation} size={64} label={`${enemy.name} 외형`} />
              <Box sx={{ minWidth: 0 }}>
                {/* Wrapping rather than truncating: "Credential Stuffing" cut
                    to "Credential Stu…" stops naming the thing it names. */}
                <Typography fontWeight={800} sx={{ lineHeight: 1.25 }}>{enemy.name}</Typography>
                <Typography variant="body2" color="text.secondary">
                  체력 {enemy.hp.toLocaleString('ko-KR')} · 생명 피해 {enemy.lifeDamage}
                </Typography>
                {traits.length > 0 && (
                  <Stack direction="row" flexWrap="wrap" useFlexGap spacing={.5} mt={.7}>
                    {traits.map((trait) => (
                      <Tooltip key={trait} title={TRAIT_COPY[trait].hint}>
                        <Chip
                          size="small"
                          label={TRAIT_COPY[trait].label}
                          color={trait === 'boss' ? 'warning' : 'default'}
                          variant={trait === 'boss' ? 'filled' : 'outlined'}
                        />
                      </Tooltip>
                    ))}
                  </Stack>
                )}
              </Box>
            </Card>
          );
        })}
      </Stack>
    </Box>
  );
}
