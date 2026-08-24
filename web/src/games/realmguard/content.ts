import type { EnemyArchetype, HeroDefinition, Point, RealmBalance, RealmGuardConfig, RealmStage, RealmWave, SkillDefinition, TowerDefinition, WaveEntry } from './types';
import { calculateResult, calculateStartingGold as startingGold } from './systems/RewardSystem';

export const REALMGUARD_VERSION = '0.3.0';

export const ENEMIES: EnemyArchetype[] = [
  { id: 'mireling', name: '습지 꼬마', color: 0x7fcf75, hp: 45, speed: 54, armor: 0, reward: 8, lifeDamage: 1, radius: 12, traits: [] },
  { id: 'thornback', name: '가시등', color: 0x4f8958, hp: 120, speed: 34, armor: .24, reward: 15, lifeDamage: 1, radius: 15, traits: ['armored'] },
  { id: 'glintfox', name: '섬광여우', color: 0xf5c76a, hp: 58, speed: 92, armor: 0, reward: 11, lifeDamage: 1, radius: 10, traits: ['swift'] },
  { id: 'cloudray', name: '구름가오리', color: 0x8fe3ff, hp: 85, speed: 68, armor: .08, reward: 14, lifeDamage: 1, radius: 13, traits: ['flying'] },
  { id: 'bloomseer', name: '꽃점술사', color: 0xef8ed5, hp: 105, speed: 42, armor: .05, reward: 18, lifeDamage: 1, radius: 13, traits: ['healer'] },
  { id: 'shardling', name: '파편충', color: 0xb79cff, hp: 88, speed: 56, armor: .06, reward: 14, lifeDamage: 1, radius: 13, traits: ['splitting'] },
  { id: 'ironroot', name: '철근목', color: 0x8d8062, hp: 260, speed: 25, armor: .38, reward: 25, lifeDamage: 2, radius: 19, traits: ['armored', 'regenerating'] },
  { id: 'veilrunner', name: '장막질주자', color: 0x8b87d7, hp: 125, speed: 74, armor: .1, reward: 19, lifeDamage: 1, radius: 12, traits: ['phasing', 'swift'] },
  { id: 'rammer', name: '성문분쇄자', color: 0xda755d, hp: 360, speed: 28, armor: .32, reward: 34, lifeDamage: 3, radius: 21, traits: ['siege', 'armored'] },
  { id: 'rimeheart', name: '서리심장', color: 0x70c9e8, hp: 210, speed: 38, armor: .18, reward: 24, lifeDamage: 2, radius: 17, traits: ['regenerating'] },
  { id: 'hollow_king', name: '공허왕 오르반', color: 0x9d5be8, hp: 2200, speed: 22, armor: .35, reward: 220, lifeDamage: 10, radius: 32, traits: ['boss', 'phasing', 'armored'] },
  { id: 'timewyrm', name: '시간룡 세라크', color: 0xff866a, hp: 4100, speed: 27, armor: .3, reward: 400, lifeDamage: 15, radius: 38, traits: ['boss', 'swift', 'regenerating'] },
];

export const TOWERS: TowerDefinition[] = [
  { id: 'sunspire', name: '태양첨탑', role: '빠른 단일 사격', color: 0xffc857, cost: 75, damage: 18, range: 150, fireRate: .52, projectileSpeed: 440, damageType: 'physical', branches: [
    { id: 'dawn_volley', name: '여명 연사', description: '공격 속도와 관통 강화', rateMultiplier: .58, pierce: 2 },
    { id: 'eagle_oath', name: '독수리 맹세', description: '사거리와 치명 피해 강화', rangeMultiplier: 1.35, damageMultiplier: 1.75 },
  ] },
  { id: 'runebloom', name: '룬꽃 정원', role: '방어 무시 비전 공격', color: 0xc69cff, cost: 105, damage: 33, range: 138, fireRate: .92, projectileSpeed: 360, damageType: 'arcane', branches: [
    { id: 'star_lattice', name: '별 격자', description: '주변 적에게 연쇄 피해', damageMultiplier: 1.55, splash: 58 },
    { id: 'null_petal', name: '무효의 꽃잎', description: '강한 적 방어 관통', damageMultiplier: 1.85, pierce: 3 },
  ] },
  { id: 'stonepulse', name: '석맥 포대', role: '느린 광역 포격', color: 0xe8845c, cost: 125, damage: 62, range: 170, fireRate: 1.55, projectileSpeed: 290, damageType: 'siege', branches: [
    { id: 'quake_drum', name: '지진북', description: '거대한 폭발 반경', damageMultiplier: 1.5, splash: 96 },
    { id: 'ember_core', name: '잿불핵', description: '집중 고열탄', damageMultiplier: 1.9, splash: 44 },
  ] },
  { id: 'windward', name: '바람수호 병영', role: '병사 소환·길목 저지', color: 0x69dce4, cost: 95, damage: 16, range: 92, fireRate: .72, projectileSpeed: 390, damageType: 'physical', branches: [
    { id: 'shield_line', name: '방패선', description: '강력한 지상 저지와 방어', slow: .34, damageMultiplier: 1.35 },
    { id: 'skyrider_watch', name: '하늘기수 초소', description: '비행 대응과 빠른 공격', rateMultiplier: .58, rangeMultiplier: 1.45, damageMultiplier: 1.55 },
  ] },
];

export const HEROES: HeroDefinition[] = [
  { id: 'aerin', name: '에어린', title: '새벽 추적자', color: 0xffd36b, hp: 520, damage: 32, range: 115, speed: 150, respawnSeconds: 9, skill1: '별빛 화살', skill2: '황혼 도약', ultimate: '새벽의 비' },
  { id: 'brann', name: '브란', title: '석문 파수꾼', color: 0xd98762, hp: 880, damage: 48, range: 48, speed: 112, respawnSeconds: 12, skill1: '방패 강타', skill2: '철벽진', ultimate: '대지의 맹세' },
  { id: 'nyra', name: '니라', title: '서리결 마도사', color: 0x7cdcf2, hp: 430, damage: 38, range: 130, speed: 132, respawnSeconds: 10, skill1: '빙결파', skill2: '거울 서리', ultimate: '백야' },
];

export const SKILLS: SkillDefinition[] = [
  { id: 'meteor', name: '별똥 낙하', description: '선택 지점에 강력한 범위 피해', cooldown: 38, color: '#ff8b5e' },
  { id: 'reinforcement', name: '수호대 소집', description: '길목을 지키는 수호대 배치', cooldown: 28, color: '#ffd36b' },
  { id: 'freeze', name: '시간 서리', description: '모든 적을 잠시 둔화', cooldown: 44, color: '#73dcff' },
];

export const BALANCE: RealmBalance = {
  difficulties: {
    casual: { enemyHp: .82, enemySpeed: .92, gold: 1.18, score: .8 },
    normal: { enemyHp: 1, enemySpeed: 1, gold: 1, score: 1 },
    veteran: { enemyHp: 1.38, enemySpeed: 1.12, gold: .9, score: 1.5 },
  },
  towerUpgradeCost: [0, 70, 120], heroLevelXp: [0, 8, 20, 38, 62, 92, 130, 176, 230, 292], endlessRamp: .085, endlessWaveBonus: 1000, sellRefundRate: .65,
  difficultyBonus: { casual: 0, normal: 5000, veteran: 10000 }, clearTimeBonusPerSecond: 10, parTimeSeconds: 900,
};

const PATHS: Point[][] = [
  [{ x: -30, y: 250 }, { x: 210, y: 250 }, { x: 210, y: 430 }, { x: 520, y: 430 }, { x: 520, y: 190 }, { x: 900, y: 190 }, { x: 900, y: 390 }, { x: 1310, y: 390 }],
  [{ x: -30, y: 150 }, { x: 250, y: 150 }, { x: 360, y: 350 }, { x: 640, y: 350 }, { x: 760, y: 560 }, { x: 990, y: 560 }, { x: 1080, y: 270 }, { x: 1310, y: 270 }],
  [{ x: -30, y: 520 }, { x: 180, y: 520 }, { x: 330, y: 300 }, { x: 560, y: 300 }, { x: 680, y: 120 }, { x: 930, y: 120 }, { x: 1060, y: 450 }, { x: 1310, y: 450 }],
  [{ x: -30, y: 360 }, { x: 180, y: 360 }, { x: 310, y: 130 }, { x: 530, y: 130 }, { x: 650, y: 500 }, { x: 880, y: 500 }, { x: 1020, y: 260 }, { x: 1310, y: 260 }],
];

const SPOTS: Point[][] = [
  [{ x: 120, y: 160 }, { x: 310, y: 340 }, { x: 400, y: 520 }, { x: 610, y: 300 }, { x: 780, y: 100 }, { x: 820, y: 300 }, { x: 1030, y: 480 }, { x: 1130, y: 300 }],
  [{ x: 140, y: 260 }, { x: 310, y: 190 }, { x: 430, y: 450 }, { x: 570, y: 250 }, { x: 700, y: 460 }, { x: 850, y: 620 }, { x: 970, y: 430 }, { x: 1130, y: 180 }],
];

function makeWaves(stage: number, endless = false): RealmWave[] {
  const pool = ENEMIES.slice(0, Math.min(10, 2 + stage));
  const waveCount = endless ? 15 : Math.min(15, 8 + Math.ceil(stage / 2));
  return Array.from({ length: waveCount }, (_, index) => {
    const tier = index + stage;
    const modifiers = stage >= 9 && index % 4 === 3 ? ['immune_stun'] : stage >= 8 && index % 4 === 2 ? ['berserk'] : stage >= 7 && index % 4 === 1 ? ['stealth'] : stage >= 6 && index % 4 === 0 ? ['magic_resist'] : [];
    const entries: WaveEntry[] = [{ enemy: pool[tier % pool.length].id, count: 5 + Math.floor(tier * .75), interval: Math.max(.35, .85 - stage * .025), modifiers }];
    if (index > 1) entries.push({ enemy: pool[(tier + 3) % pool.length].id, count: 2 + Math.floor(tier / 3), interval: 1.05, delay: 1.5 });
    if (!endless && stage === 5 && index === waveCount - 1) entries.push({ enemy: 'hollow_king', count: 1, interval: 1.5, delay: 2 });
    if (!endless && stage === 10 && index === waveCount - 1) entries.push({ enemy: 'timewyrm', count: 1, interval: 1.5, delay: 2 });
    return { id: `s${stage}-w${index + 1}`, label: `${index + 1} 파동`, entries, reward: 28 + stage * 4 + index * 3 };
  });
}

const stageNames = ['이끼빛 관문', '유리바람 평원', '속삭임 습지', '잿불 고개', '별뿌리 성소', '부서진 월교', '서리결 골짜기', '무명의 회랑', '시간의 균열', '새벽 없는 왕좌'];
const subtitles = ['첫 장막이 흔들린다', '빛나는 들판의 추격전', '늪의 속삭임을 잠재워라', '불씨를 지키는 길', '공허왕의 첫 강림', '두 세계를 잇는 마지막 다리', '멈춘 겨울의 심장', '기억을 잃는 길', '시간룡이 깨어난다', 'Realm의 운명을 건 수호전'];

export const STAGES: RealmStage[] = stageNames.map((name, index) => {
  const number = index + 1;
  return {
    id: `stage-${number}`, number, name, subtitle: subtitles[index], mode: 'campaign',
    theme: (['verdant', 'verdant', 'verdant', 'ember', 'void', 'ember', 'frost', 'void', 'frost', 'void'] as RealmStage['theme'][])[index],
    path: PATHS[index % PATHS.length], towerSpots: SPOTS[index % SPOTS.length].map((spot, spotIndex) => ({ ...spot, id: `s${number}-spot-${spotIndex + 1}` })),
    waves: makeWaves(number), startingGold: 280 + number * 10, lives: 20, version: `1.${number}.0`,
    gimmick: number === 4 ? 'ember_vents' : number === 7 ? 'winter_blessing' : number === 9 ? 'time_surge' : undefined,
  };
});

STAGES.push({
  id: 'endless-rift', number: 11, name: '끝없는 균열', subtitle: '한계 없이 밀려오는 장막', mode: 'endless', theme: 'void',
  path: PATHS[3], towerSpots: SPOTS[1].map((spot, index) => ({ ...spot, id: `endless-spot-${index + 1}` })), waves: makeWaves(11, true), startingGold: 420, lives: 25, version: '1.0.0',
});

export const DEFAULT_REALMGUARD_CONFIG: RealmGuardConfig = {
  versionId: '',
  contentVersion: REALMGUARD_VERSION,
  balanceVersion: '2026.08.1',
  assetVersion: 'procedural-1',
  stages: STAGES,
  enemies: ENEMIES,
  towers: TOWERS,
  heroes: HEROES,
  skills: SKILLS,
  balance: BALANCE,
};

export function calculateStartingGold(stage: Pick<RealmStage, 'startingGold'>, balance: RealmBalance, difficulty: keyof RealmBalance['difficulties']) {
  return startingGold(stage, balance, difficulty);
}

export function advanceFixedSimulation(accumulator: number, wallDelta: number, speed: 1 | 2, tickMs = 50) {
  const available = accumulator + Math.min(Math.max(0, wallDelta), 250) * speed;
  const steps = Math.floor(available / tickMs);
  return { steps, remainder: available - steps * tickMs, elapsed: steps * tickMs };
}

export function simulationCooldownReady(now: number, last: number, cooldownMs: number) {
  return now - last >= cooldownMs;
}

export function calculateLocalResult(input: { victory: boolean; lives: number; kills: number; waves: number; gold: number; duration_ms?: number; difficulty: keyof RealmBalance['difficulties']; mode: 'campaign' | 'endless' }, balance: RealmBalance = BALANCE) {
  return calculateResult(input, balance);
}
