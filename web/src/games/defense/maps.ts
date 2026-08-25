import type { Point, RealmStage, TowerSpot } from "../realmguard/types";
import type { DefenseSlug } from "./types";

export interface DefenseMapLayout {
  style: string;
  label: string;
  theme: RealmStage["theme"];
  paths: Point[][];
  towerSpots: Point[];
}

interface Geometry {
  paths: Point[][];
  towerSpots: Point[];
}

const GEOMETRIES: Geometry[] = [
  {
    paths: [[
      { x: -40, y: 190 }, { x: 240, y: 190 }, { x: 240, y: 505 },
      { x: 535, y: 505 }, { x: 535, y: 235 }, { x: 880, y: 235 },
      { x: 880, y: 455 }, { x: 1320, y: 455 },
    ]],
    towerSpots: [
      { x: 105, y: 300 }, { x: 335, y: 105 }, { x: 355, y: 390 },
      { x: 440, y: 610 }, { x: 650, y: 390 }, { x: 745, y: 125 },
      { x: 985, y: 330 }, { x: 1120, y: 570 },
    ],
  },
  {
    paths: [[
      { x: -40, y: 570 }, { x: 170, y: 570 }, { x: 345, y: 365 },
      { x: 520, y: 155 }, { x: 760, y: 155 }, { x: 905, y: 350 },
      { x: 1085, y: 560 }, { x: 1320, y: 560 },
    ]],
    towerSpots: [
      { x: 95, y: 445 }, { x: 255, y: 660 }, { x: 345, y: 210 },
      { x: 485, y: 480 }, { x: 640, y: 270 }, { x: 790, y: 65 },
      { x: 925, y: 505 }, { x: 1120, y: 380 },
    ],
  },
  {
    paths: [
      [
        { x: -40, y: 185 }, { x: 220, y: 185 }, { x: 420, y: 360 },
        { x: 655, y: 360 }, { x: 850, y: 185 }, { x: 1060, y: 185 },
        { x: 1320, y: 360 },
      ],
      [
        { x: -40, y: 535 }, { x: 220, y: 535 }, { x: 420, y: 360 },
        { x: 655, y: 360 }, { x: 850, y: 535 }, { x: 1060, y: 535 },
        { x: 1320, y: 360 },
      ],
    ],
    towerSpots: [
      { x: 105, y: 80 }, { x: 105, y: 640 }, { x: 315, y: 330 },
      { x: 505, y: 245 }, { x: 575, y: 475 }, { x: 750, y: 360 },
      { x: 930, y: 345 }, { x: 1135, y: 360 },
    ],
  },
  {
    paths: [[
      { x: -40, y: 125 }, { x: 1090, y: 125 }, { x: 1090, y: 590 },
      { x: 180, y: 590 }, { x: 180, y: 350 }, { x: 1320, y: 350 },
    ]],
    towerSpots: [
      { x: 120, y: 235 }, { x: 335, y: 45 }, { x: 570, y: 230 },
      { x: 805, y: 45 }, { x: 1000, y: 245 }, { x: 940, y: 485 },
      { x: 600, y: 675 }, { x: 320, y: 455 },
    ],
  },
  {
    paths: [[
      { x: -40, y: 610 }, { x: 245, y: 610 }, { x: 245, y: 420 },
      { x: 500, y: 420 }, { x: 500, y: 185 }, { x: 760, y: 185 },
      { x: 760, y: 495 }, { x: 1040, y: 495 }, { x: 1040, y: 250 },
      { x: 1320, y: 250 },
    ]],
    towerSpots: [
      { x: 105, y: 495 }, { x: 345, y: 520 }, { x: 355, y: 305 },
      { x: 610, y: 310 }, { x: 650, y: 80 }, { x: 875, y: 380 },
      { x: 900, y: 610 }, { x: 1150, y: 365 },
    ],
  },
  {
    paths: [
      [
        { x: -40, y: 160 }, { x: 270, y: 160 }, { x: 425, y: 315 },
        { x: 650, y: 315 }, { x: 810, y: 160 }, { x: 1060, y: 160 },
        { x: 1320, y: 315 },
      ],
      [
        { x: -40, y: 560 }, { x: 270, y: 560 }, { x: 425, y: 405 },
        { x: 650, y: 405 }, { x: 810, y: 560 }, { x: 1060, y: 560 },
        { x: 1320, y: 405 },
      ],
    ],
    towerSpots: [
      { x: 120, y: 300 }, { x: 305, y: 60 }, { x: 305, y: 660 },
      { x: 525, y: 210 }, { x: 525, y: 510 }, { x: 735, y: 365 },
      { x: 940, y: 350 }, { x: 1150, y: 280 },
    ],
  },
  {
    paths: [[
      { x: -40, y: 95 }, { x: 190, y: 170 }, { x: 345, y: 540 },
      { x: 555, y: 620 }, { x: 745, y: 295 }, { x: 910, y: 105 },
      { x: 1080, y: 410 }, { x: 1320, y: 520 },
    ]],
    towerSpots: [
      { x: 110, y: 285 }, { x: 275, y: 65 }, { x: 200, y: 450 },
      { x: 500, y: 400 }, { x: 650, y: 560 }, { x: 785, y: 90 },
      { x: 955, y: 290 }, { x: 1155, y: 610 },
    ],
  },
  {
    paths: [[
      { x: -40, y: 180 }, { x: 235, y: 180 }, { x: 480, y: 360 },
      { x: 725, y: 540 }, { x: 980, y: 540 }, { x: 1080, y: 360 },
      { x: 980, y: 180 }, { x: 725, y: 180 }, { x: 480, y: 360 },
      { x: 1320, y: 360 },
    ]],
    towerSpots: [
      { x: 105, y: 315 }, { x: 330, y: 70 }, { x: 400, y: 500 },
      { x: 570, y: 220 }, { x: 650, y: 550 }, { x: 820, y: 650 },
      { x: 890, y: 295 }, { x: 1150, y: 495 },
    ],
  },
  {
    paths: [
      [
        { x: -40, y: 95 }, { x: 260, y: 95 }, { x: 420, y: 300 },
        { x: 640, y: 360 }, { x: 860, y: 300 }, { x: 1040, y: 95 },
        { x: 1320, y: 250 },
      ],
      [
        { x: -40, y: 625 }, { x: 260, y: 625 }, { x: 420, y: 420 },
        { x: 640, y: 360 }, { x: 860, y: 420 }, { x: 1040, y: 625 },
        { x: 1320, y: 470 },
      ],
    ],
    towerSpots: [
      { x: 120, y: 240 }, { x: 120, y: 480 }, { x: 330, y: 360 },
      { x: 500, y: 205 }, { x: 500, y: 515 }, { x: 755, y: 185 },
      { x: 755, y: 535 }, { x: 1060, y: 360 },
    ],
  },
  {
    paths: [[
      { x: -40, y: 360 }, { x: 175, y: 360 }, { x: 175, y: 105 },
      { x: 1035, y: 105 }, { x: 1035, y: 615 }, { x: 370, y: 615 },
      { x: 370, y: 285 }, { x: 790, y: 285 }, { x: 790, y: 465 },
      { x: 1320, y: 465 },
    ]],
    towerSpots: [
      { x: 75, y: 230 }, { x: 315, y: 205 }, { x: 530, y: 35 },
      { x: 815, y: 205 }, { x: 945, y: 370 }, { x: 900, y: 670 },
      { x: 535, y: 470 }, { x: 1115, y: 575 },
    ],
  },
];

const MAP_IDENTITIES: Record<DefenseSlug, Array<[string, string, RealmStage["theme"]]>> = {
  "office-guardians": [
    ["office-plaza", "서비스 플라자", "verdant"],
    ["office-cloud", "클라우드 브리지", "frost"],
    ["office-datacenter", "데이터센터 분기선", "void"],
    ["office-deploy", "배포 순환로", "ember"],
    ["office-api", "API 게이트웨이", "frost"],
    ["office-data", "데이터 동기화선", "verdant"],
    ["office-legacy", "레거시 협곡", "ember"],
    ["office-crisis", "Company Core", "void"],
  ],
  "cyber-fortress": [
    ["cyber-mail", "메일 경계망", "frost"],
    ["cyber-identity", "인증 능선", "void"],
    ["cyber-endpoint", "엔드포인트 분기망", "ember"],
    ["cyber-web", "웹 방화벽 순환로", "frost"],
    ["cyber-ddos", "DDoS 우회망", "void"],
    ["cyber-insider", "내부자 이중 경로", "verdant"],
    ["cyber-dlp", "데이터 유출 협곡", "ember"],
    ["cyber-supply", "공급망 교차로", "verdant"],
    ["cyber-zero-day", "Zero Day 균열", "void"],
    ["cyber-vault", "Critical Data Vault", "frost"],
  ],
  "ai-nexus-defense": [
    ["ai-prompt", "Prompt Gateway", "void"],
    ["ai-rag", "RAG 지식 능선", "verdant"],
    ["ai-agent", "Agent 분기망", "frost"],
    ["ai-guardrail", "Guardrail 순환로", "ember"],
    ["ai-routing", "Model Router", "void"],
    ["ai-review", "Human Review 이중선", "verdant"],
    ["ai-enterprise", "Enterprise AI Mesh", "frost"],
    ["ai-context", "Context 교차 균열", "ember"],
    ["ai-trust", "Trust Recovery Mesh", "verdant"],
    ["ai-core", "AI Nexus Core", "void"],
  ],
};

export function defenseMapLayout(slug: DefenseSlug, index: number): DefenseMapLayout {
  const identities = MAP_IDENTITIES[slug];
  const identity = identities[index % identities.length];
  const geometry = GEOMETRIES[index % GEOMETRIES.length];
  return {
    style: identity[0],
    label: identity[1],
    theme: identity[2],
    paths: geometry.paths.map((path) => path.map((point) => ({ ...point }))),
    towerSpots: geometry.towerSpots.map((point) => ({ ...point })),
  };
}

export function defenseMapTowerSpots(
  slug: DefenseSlug,
  index: number,
  stageId: string,
): TowerSpot[] {
  return defenseMapLayout(slug, index).towerSpots.map((spot, spotIndex) => ({
    ...spot,
    id: `${stageId}-spot-${spotIndex + 1}`,
  }));
}

export function defenseMapLabel(style?: string): string {
  for (const identities of Object.values(MAP_IDENTITIES)) {
    const match = identities.find(([candidate]) => candidate === style);
    if (match) return match[1];
  }
  return "전술 전장";
}

export const DEFENSE_MAP_GEOMETRY_COUNT = GEOMETRIES.length;
