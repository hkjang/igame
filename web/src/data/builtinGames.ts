import type { Game } from '../types';

export const BUILTIN_GAMES: Game[] = [
  {
    id: 'office-guardians', slug: 'office-guardians', name: 'Office Guardians', category: 'Defense Series', tags: ['Defense Series', '조직', '타워 디펜스'],
    description: '조직과 직무의 강점을 조합해 Company City의 핵심 서비스를 지키세요.', game_type: 'builtin', status: 'active',
    ranking: true, achievement: true, version: '0.4.0', developer: 'igame', accent: '#72e0a6', icon: 'OG', thumbnail: '/assets/games/office-guardians.svg', banner: '/assets/games/office-guardians-banner.svg',
  },
  {
    id: 'cyber-fortress', slug: 'cyber-fortress', name: 'Cyber Fortress', category: 'Defense Series', tags: ['Defense Series', '보안교육', '타워 디펜스'],
    description: '보안 위협의 상성을 익히고 사고 상황에서 안전한 판단을 내리세요.', game_type: 'builtin', status: 'active',
    ranking: true, achievement: true, version: '0.4.0', developer: 'igame', accent: '#65d6ff', icon: 'CF', thumbnail: '/assets/games/cyber-fortress.svg', banner: '/assets/games/cyber-fortress-banner.svg',
  },
  {
    id: 'ai-nexus-defense', slug: 'ai-nexus-defense', name: 'AI Nexus Defense', category: 'Defense Series', tags: ['Defense Series', 'AI교육', '타워 디펜스'],
    description: 'AI 플랫폼의 품질·비용·지연·신뢰를 균형 있게 방어하세요.', game_type: 'builtin', status: 'active',
    ranking: true, achievement: true, version: '0.4.0', developer: 'igame', accent: '#b694ff', icon: 'AI', thumbnail: '/assets/games/ai-nexus-defense.svg', banner: '/assets/games/ai-nexus-defense-banner.svg',
  },
  {
    id: 'realmguard', slug: 'realmguard', name: 'RealmGuard', category: '전략', tags: ['전략', '타워 디펜스', '캠페인'],
    description: '영웅과 네 종류의 타워로 장막 너머 Realm을 수호하세요.', game_type: 'builtin', status: 'active',
    ranking: true, achievement: true, version: '0.3.1', developer: 'igame', accent: '#7fe0c1', icon: 'RG',
  },
  {
    id: '2048', slug: '2048', name: '2048', category: '퍼즐', tags: ['퍼즐', '숫자', '싱글'],
    description: '같은 숫자 타일을 합쳐 2048을 완성하세요.', game_type: 'builtin', status: 'active',
    ranking: true, achievement: true, version: '1.0.0', developer: 'igame', accent: '#ffad5c', icon: '2048',
  },
  {
    id: 'snake', slug: 'snake', name: 'Snake', category: '아케이드', tags: ['아케이드', '반응', '싱글'],
    description: '벽과 꼬리를 피해 먹이를 모으는 클래식 게임입니다.', game_type: 'builtin', status: 'active',
    ranking: true, achievement: true, version: '1.0.0', developer: 'igame', accent: '#73df9b', icon: 'S',
  },
  {
    id: 'memory', slug: 'memory', name: 'Memory Cards', category: '퍼즐', tags: ['기억력', '카드', '싱글'],
    description: '제한된 시도 안에 같은 모양의 카드 짝을 찾으세요.', game_type: 'builtin', status: 'active',
    ranking: true, achievement: true, version: '1.0.0', developer: 'igame', accent: '#af8cff', icon: 'M',
  },
  {
    id: 'reaction', slug: 'reaction', name: 'Reaction Test', category: '캐주얼', tags: ['반응', '스피드', '싱글'],
    description: '신호가 바뀌는 순간 눌러 나의 반응 속도를 측정하세요.', game_type: 'builtin', status: 'active',
    ranking: true, achievement: true, version: '1.0.0', developer: 'igame', accent: '#67d7ff', icon: 'R',
  },
  {
    id: 'typing', slug: 'typing', name: 'Typing Sprint', category: '캐주얼', tags: ['타이핑', '스피드', '싱글'],
    description: '60초 동안 제시되는 문장을 빠르고 정확하게 입력하세요.', game_type: 'builtin', status: 'active',
    ranking: true, achievement: true, version: '1.0.0', developer: 'igame', accent: '#ff718f', icon: 'T',
  },
];

export function mergeGames(remote?: Game[]): Game[] {
	if (remote === undefined) return BUILTIN_GAMES.map((game) => ({ ...game }));
  const map = new Map(remote.map((game) => [game.slug, game]));
	for (const builtin of BUILTIN_GAMES) {
		const registered = map.get(builtin.slug);
		if (registered) map.set(builtin.slug, { ...builtin, ...registered, game_type: 'builtin' });
	}
  return [...map.values()];
}
