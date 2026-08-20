import type { Game } from '../types';

export const BUILTIN_GAMES: Game[] = [
  {
    id: 'realmguard', slug: 'realmguard', name: 'RealmGuard', category: '전략', tags: ['전략', '타워 디펜스', '캠페인'],
    description: '영웅과 네 종류의 타워로 장막 너머 Realm을 수호하세요.', game_type: 'builtin', status: 'active',
    ranking: true, achievement: true, version: '0.2.0', developer: 'igame', accent: '#7fe0c1', icon: 'RG',
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
