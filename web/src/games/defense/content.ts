import { BALANCE } from "../realmguard/content";
import type {
  EnemyArchetype,
  HeroDefinition,
  RealmStage,
  RealmWave,
  TowerDefinition,
} from "../realmguard/types";
import type {
  AIModelProfile,
  AIResourceRules,
  DefenseContentPack,
  DefenseEducationEvent,
  DefensePresentation,
  DefenseQuestion,
  DefenseSlug,
} from "./types";
import { defenseMapLayout, defenseMapTowerSpots } from "./maps";

export const DEFENSE_SERIES_VERSION = "0.4.0";

const colors = [
  0x65d6ff, 0x72e0a6, 0xffc866, 0xb694ff, 0xff7c91, 0x67e8db, 0xf49b67,
  0x91a7ff, 0xe7d86e, 0x8bd28b,
];
function tower(
  id: string,
  name: string,
  role: string,
  index: number,
): TowerDefinition {
  return {
    id,
    name,
    role,
    color: colors[index % colors.length],
    cost: 70 + index * 12,
    damage: 17 + index * 5,
    range: 132 + index * 5,
    fireRate: 0.52 + (index % 3) * 0.25,
    projectileSpeed: 360 + index * 8,
    damageType: (["physical", "arcane", "siege", "frost"] as const)[index % 4],
    effectiveAgainst: [
      ["web_attack"],
      ["malware"],
      ["account_attack"],
      ["data_leak"],
      ["unknown"],
    ][index % 5],
    effectiveMultiplier: 1.65,
    branches: [
      {
        id: `${id}_precision`,
        name: `${name} 정밀화`,
        description: "핵심 대상에 대한 피해와 사거리를 강화합니다.",
        damageMultiplier: 1.75,
        rangeMultiplier: 1.2,
      },
      {
        id: `${id}_network`,
        name: `${name} 연계망`,
        description: "빠른 연계와 범위 대응을 강화합니다.",
        rateMultiplier: 0.64,
        splash: 62,
      },
    ],
  };
}

function enemy(
  id: string,
  name: string,
  index: number,
  boss = false,
): EnemyArchetype {
  const traitCycle: EnemyArchetype["traits"] = [
    "swift",
    "armored",
    "regenerating",
    "healer",
    "flying",
    "phasing",
    "siege",
    "stealth",
    "magic_resist",
    "berserk",
  ];
  return {
    id,
    name,
    color: colors[(index + 3) % colors.length],
    hp: boss ? 2100 + index * 340 : 52 + index * 24,
    speed: boss ? 22 + index : 38 + (index % 5) * 8,
    armor: boss ? 0.32 : (index % 4) * 0.07,
    reward: boss ? 240 : 9 + index * 3,
    lifeDamage: boss ? 8 + index : 1 + Math.floor(index / 6),
    radius: boss ? 31 + index : 11 + (index % 7),
    traits: boss
      ? ["boss", traitCycle[index % traitCycle.length]]
      : index === 0
        ? []
        : [traitCycle[index % traitCycle.length]],
    threatType: [
      "web_attack",
      "malware",
      "account_attack",
      "data_leak",
      "unknown",
    ][index % 5],
    resourceEffect: {
      trust: boss ? 18 : 5 + (index % 4),
      latency: boss ? 14 : 3 + (index % 3),
    },
  };
}

function hero(
  id: string,
  name: string,
  title: string,
  index: number,
): HeroDefinition {
  return {
    id,
    name,
    title,
    color: colors[index % colors.length],
    hp: 500 + index * 95,
    damage: 30 + index * 7,
    range: index % 2 ? 54 : 118,
    speed: 125 + index * 4,
    respawnSeconds: 9 + index,
    skill1: `${title} 분석`,
    skill2: `${title} 대응`,
    ultimate: `${title} 총력전`,
    unlockStage: index === 0 ? 1 : index === 1 ? 3 : 5,
  };
}

function makeWave(
  stageNumber: number,
  waveNumber: number,
  enemies: EnemyArchetype[],
  bosses: EnemyArchetype[],
  laneCount: number,
): RealmWave {
  const first = enemies[(stageNumber + waveNumber) % enemies.length];
  const second = enemies[(stageNumber * 2 + waveNumber + 3) % enemies.length];
  const entries: RealmWave["entries"] = [
    {
      enemy: first.id,
      count: 4 + stageNumber + waveNumber,
      interval: Math.max(0.42, 0.9 - stageNumber * 0.025),
      pathIndex: waveNumber % laneCount,
    },
    {
      enemy: second.id,
      count: 2 + Math.floor((stageNumber + waveNumber) / 3),
      interval: 1.05,
      delay: 1.4,
      pathIndex: (waveNumber + 1) % laneCount,
      parallel: laneCount > 1,
    },
  ];
  if (waveNumber === 7 && (stageNumber === 5 || stageNumber >= 8))
    entries.push({
      enemy: bosses[(stageNumber - 1) % bosses.length].id,
      count: 1,
      interval: 1.5,
      delay: 2.2,
      pathIndex: waveNumber % laneCount,
    });
  return {
    id: `stage-${stageNumber}-wave-${waveNumber + 1}`,
    label: `${waveNumber + 1} 웨이브`,
    entries,
    reward: 30 + stageNumber * 5 + waveNumber * 2,
  };
}

function makeStages(
  slug: DefenseSlug,
  names: string[],
  enemies: EnemyArchetype[],
  bosses: EnemyArchetype[],
): RealmStage[] {
  return names.map((name, index) => {
    const number = index + 1;
    const id = `stage-${number}`;
    const map = defenseMapLayout(slug, index);
    return {
      id,
      number,
      name,
      subtitle: `${name} 방어 시나리오`,
      mode: "campaign",
      theme: map.theme,
      mapStyle: map.style,
      path: map.paths[0],
      paths: map.paths,
      towerSpots: defenseMapTowerSpots(slug, index, id),
      waves: Array.from({ length: 8 }, (_, wave) =>
        makeWave(number, wave, enemies, bosses, map.paths.length),
      ),
      startingGold: 300 + index * 18,
      lives: 20,
      version: `4.${number}.0`,
      gimmick:
        index % 4 === 1
          ? "time_surge"
          : index % 4 === 2
            ? "ember_vents"
            : index % 4 === 3
              ? "winter_blessing"
              : undefined,
    };
  });
}

const officePresentation: DefensePresentation = {
  name: "Office Guardians",
  shortName: "Office",
  eyebrow: "DEFENSE SERIES · COMPANY CITY",
  description:
    "개발·데이터·보안·운영 역량을 조합해 회사의 핵심 서비스를 지키세요.",
  story:
    "Company City의 서비스, 데이터와 고객 신뢰를 위협하는 업무 괴물에 맞서는 조직 협업 디펜스입니다.",
  primary: "#72e0a6",
  secondary: "#65d6ff",
  resourceName: "협업력",
  healthName: "서비스 안정성",
  heroName: "마스터",
  towerName: "조직",
  enemyName: "업무 위협",
};

const cyberPresentation: DefensePresentation = {
  name: "Cyber Fortress",
  shortName: "Cyber",
  eyebrow: "DEFENSE SERIES · SECURITY LEARNING",
  description:
    "보안 솔루션의 상성을 익히고 실제 사고 상황에서 올바른 판단을 내려보세요.",
  story:
    "Internet에서 Critical Data까지 이어지는 방어선에서 피싱, 악성코드와 데이터 유출을 차단합니다.",
  primary: "#65d6ff",
  secondary: "#ff7c91",
  resourceName: "대응 자원",
  healthName: "자산 무결성",
  heroName: "보안 전문가",
  towerName: "보안 통제",
  enemyName: "사이버 위협",
};

const aiPresentation: DefensePresentation = {
  name: "AI Nexus Defense",
  shortName: "AI Nexus",
  eyebrow: "DEFENSE SERIES · AI LITERACY",
  description:
    "정확도·비용·지연·신뢰의 균형을 잡아 엔터프라이즈 AI 플랫폼을 보호하세요.",
  story:
    "AI Gateway, Agent, LLM, RAG와 업무 시스템을 연결한 Nexus를 AI 위협과 잘못된 자동화에서 지킵니다.",
  primary: "#b694ff",
  secondary: "#67e8db",
  resourceName: "배치 크레딧",
  healthName: "Nexus 무결성",
  heroName: "Agent",
  towerName: "AI 구성요소",
  enemyName: "AI 위험",
};

function eventsFromQuestions(
  questions: DefenseQuestion[],
): DefenseEducationEvent[] {
  return questions
    .slice(0, 30)
    .map((question, index) => ({
      ...question,
      trigger: index < 10 ? "battle_start" : `wave_${(index % 8) + 1}`,
    }));
}

function createPack(input: {
  slug: DefenseSlug;
  presentation: DefensePresentation;
  stageNames: string[];
  towerNames: Array<[string, string, string]>;
  enemyNames: Array<[string, string]>;
  bossNames: Array<[string, string]>;
  heroes: Array<[string, string, string]>;
  education?: DefenseQuestion[];
  modelProfiles?: AIModelProfile[];
  resourceRules?: AIResourceRules;
}): DefenseContentPack {
  const towers = input.towerNames.map(([id, name, role], index) =>
    tower(id, name, role, index),
  );
  const enemies = input.enemyNames.map(([id, name], index) =>
    enemy(id, name, index),
  );
  const bosses = input.bossNames.map(([id, name], index) =>
    enemy(id, name, index + enemies.length, true),
  );
  const heroes = input.heroes.map(([id, name, title], index) =>
    hero(id, name, title, index),
  );
  const education = input.education ?? [];
  const runtimeTowers = towers.map((item) => ({
    ...item,
    profiles: input.modelProfiles
      ?.filter((profile) => profile.tower_id === item.id)
      .map((profile) => ({
        id: profile.id,
        name: profile.name,
        damageMultiplier: profile.damage_multiplier,
      })),
  }));
  return {
    slug: input.slug,
    presentation: input.presentation,
    education,
    events: eventsFromQuestions(education),
    policyVersion: "2026.08",
    educationEnabled: education.length > 0,
    modelProfiles: input.modelProfiles,
    resourceRules: input.resourceRules,
    config: {
      versionId: `00000000-0000-4000-8000-${input.slug === "office-guardians" ? "000000000301" : input.slug === "cyber-fortress" ? "000000000302" : "000000000303"}`,
      contentVersion: DEFENSE_SERIES_VERSION,
      balanceVersion: "2026.08.21.1",
      assetVersion: "procedural-defense-2",
      stages: makeStages(input.slug, input.stageNames, enemies, bosses),
      enemies: [...enemies, ...bosses],
      towers: runtimeTowers,
      heroes,
      skills: [
        {
          id: "meteor",
          name:
            input.slug === "cyber-fortress"
              ? "긴급 격리"
              : input.slug === "ai-nexus-defense"
                ? "회로 차단"
                : "긴급 지원",
          description: "선택 지점의 위협을 집중 제거합니다.",
          cooldown: 38,
          color: input.presentation.secondary,
        },
        {
          id: "reinforcement",
          name:
            input.slug === "ai-nexus-defense" ? "Human Review" : "대응팀 투입",
          description: "길목에 임시 대응팀을 배치합니다.",
          cooldown: 28,
          color: input.presentation.primary,
        },
        {
          id: "freeze",
          name: input.slug === "cyber-fortress" ? "전사 차단" : "흐름 제어",
          description: "모든 위협의 진행을 잠시 늦춥니다.",
          cooldown: 44,
          color: "#91a7ff",
        },
      ],
      balance: { ...BALANCE, parTimeSeconds: 720 },
    },
  };
}

export const OFFICE_GUARDIANS = createPack({
  slug: "office-guardians",
  presentation: officePresentation,
  stageNames: [
    "신규 서비스 오픈",
    "트래픽 폭주",
    "DB 장애",
    "배포 장애",
    "API 공격",
    "데이터 오류",
    "레거시 시스템",
    "Company Crisis",
  ],
  towerNames: [
    ["developer", "개발자", "빠른 문제 해결"],
    ["dba", "DBA", "데이터 방어"],
    ["security", "보안 담당자", "위협 약화"],
    ["infra", "인프라 담당자", "범위 지원"],
    ["ai_engineer", "AI 엔지니어", "광역 분석"],
    ["windward", "운영 담당자", "현장 저지와 복구"],
  ],
  enemyNames: [
    ["bug", "Bug"],
    ["traffic_monster", "Traffic Monster"],
    ["legacy_beast", "Legacy Beast"],
    ["data_corruptor", "Data Corruptor"],
    ["bot", "Bot"],
    ["shadow_user", "Shadow User"],
    ["incident", "Incident"],
    ["deadline", "Deadline"],
    ["dependency_breaker", "Dependency Breaker"],
    ["alert_storm", "Alert Storm"],
  ],
  bossNames: [
    ["outage_overlord", "Outage Overlord"],
    ["crisis_core", "Company Crisis Core"],
  ],
  heroes: [
    ["architect", "Architect", "설계 마스터"],
    ["security_master", "Security Master", "보안 마스터"],
    ["operations_master", "Operations Master", "운영 마스터"],
  ],
});

export const CYBER_FORTRESS = createPack({
  slug: "cyber-fortress",
  presentation: cyberPresentation,
  stageNames: [
    "Phishing Attack",
    "Password Siege",
    "Malware Outbreak",
    "Web Breach",
    "DDoS Storm",
    "Insider Shadow",
    "Data Leakage",
    "Supply Chain",
    "Zero Day",
    "Critical Incident",
  ],
  towerNames: [
    ["firewall", "Firewall", "네트워크 차단"],
    ["waf", "WAF", "웹 공격 차단"],
    ["ids", "IDS", "공격 탐지"],
    ["ips", "IPS", "능동 차단"],
    ["edr", "EDR", "Endpoint 방어"],
    ["mfa", "MFA", "계정 보호"],
    ["dlp", "DLP", "유출 방지"],
    ["windward", "SOC", "현장 대응과 저지"],
  ],
  enemyNames: [
    ["phishing", "Phishing"],
    ["malware", "Malware"],
    ["ransomware", "Ransomware"],
    ["sql_injection", "SQL Injection"],
    ["xss", "XSS"],
    ["credential_stuffing", "Credential Stuffing"],
    ["ddos", "DDoS"],
    ["insider", "Insider"],
    ["zero_day", "Zero Day"],
    ["supply_chain", "Supply Chain"],
    ["data_exfiltration", "Data Exfiltration"],
    ["botnet", "Botnet"],
    ["privilege_escalation", "Privilege Escalation"],
    ["session_hijack", "Session Hijack"],
    ["cloud_misconfig", "Cloud Misconfiguration"],
  ],
  bossNames: [
    ["ransom_lord", "Ransom Lord"],
    ["zero_day_phantom", "Zero Day Phantom"],
    ["apt_commander", "APT Commander"],
  ],
  heroes: [
    ["incident_commander", "Incident Commander", "사고 대응"],
    ["threat_hunter", "Threat Hunter", "위협 추적"],
    ["forensic_lead", "Forensic Lead", "디지털 포렌식"],
  ],
});

export const AI_NEXUS_DEFENSE = createPack({
  slug: "ai-nexus-defense",
  presentation: aiPresentation,
  stageNames: [
    "LLM Basics",
    "RAG Pipeline",
    "Agent Network",
    "AI Security",
    "Model Routing",
    "Responsible AI",
    "Enterprise AI",
    "Context Crisis",
    "Trust Recovery",
    "AI Incident",
  ],
  towerNames: [
    ["ai_gateway", "AI Gateway", "요청 제어"],
    ["prompt_guard", "Prompt Guard", "주입 방어"],
    ["rag", "RAG", "근거 강화"],
    ["vector_search", "Vector Search", "지식 검색"],
    ["model_router", "Model Router", "비용·품질 최적화"],
    ["guardrail", "Guardrail", "안전성 검증"],
    ["evaluator", "Evaluator", "결과 평가"],
    ["cache", "Cache", "응답 가속"],
    ["agent_node", "Agent", "자동 대응"],
    ["windward", "Human Review", "최종 검토와 저지"],
  ],
  enemyNames: [
    ["hallucination", "Hallucination"],
    ["prompt_injection", "Prompt Injection"],
    ["bad_context", "Bad Context"],
    ["data_poisoning", "Data Poisoning"],
    ["token_monster", "Token Monster"],
    ["latency_beast", "Latency Beast"],
    ["model_drift", "Model Drift"],
    ["rogue_agent", "Rogue Agent"],
    ["context_overflow", "Context Overflow"],
    ["sensitive_leak", "Sensitive Data Leak"],
    ["tool_abuse", "Tool Abuse"],
    ["bias_wraith", "Bias Wraith"],
    ["retrieval_noise", "Retrieval Noise"],
    ["cost_spike", "Cost Spike"],
    ["shadow_model", "Shadow Model"],
  ],
  bossNames: [
    ["hallucination_king", "Hallucination King"],
    ["injection_master", "Prompt Injection Master"],
    ["token_hydra", "Token Hydra"],
    ["rogue_overseer", "Rogue Agent Overseer"],
  ],
  heroes: [
    ["research_agent", "Research Agent", "RAG 연구"],
    ["security_agent", "Security Agent", "AI 보안"],
    ["coding_agent", "Coding Agent", "시스템 복구"],
    ["data_agent", "Data Agent", "데이터 정제"],
    ["supervisor_agent", "Supervisor Agent", "Agent 감독"],
  ],
  modelProfiles: [
    {
      id: "small",
      name: "Small Model",
      tower_id: "ai_gateway",
      compute_cost: 20,
      token_cost: 10,
      latency_cost: 2,
      accuracy: 72,
      damage_multiplier: 0.85,
    },
    {
      id: "medium",
      name: "Medium Model",
      tower_id: "prompt_guard",
      compute_cost: 45,
      token_cost: 24,
      latency_cost: 4,
      accuracy: 82,
      damage_multiplier: 1,
    },
    {
      id: "large",
      name: "Large Model",
      tower_id: "rag",
      compute_cost: 100,
      token_cost: 60,
      latency_cost: 9,
      accuracy: 92,
      damage_multiplier: 1.35,
    },
    {
      id: "reasoning",
      name: "Reasoning Model",
      tower_id: "vector_search",
      compute_cost: 120,
      token_cost: 80,
      latency_cost: 12,
      accuracy: 96,
      damage_multiplier: 1.55,
    },
    {
      id: "vision",
      name: "Vision Model",
      tower_id: "model_router",
      compute_cost: 90,
      token_cost: 55,
      latency_cost: 8,
      accuracy: 89,
      damage_multiplier: 1.3,
    },
  ],
  resourceRules: {
    compute_start: 1000,
    token_start: 1000,
    trust_start: 100,
    latency_max: 100,
    wave_compute_cost: 18,
    wave_token_cost: 24,
    escaped_trust_cost: 8,
    escaped_latency_cost: 5,
  },
});

export const DEFENSE_PACKS: Record<DefenseSlug, DefenseContentPack> = {
  "office-guardians": OFFICE_GUARDIANS,
  "cyber-fortress": CYBER_FORTRESS,
  "ai-nexus-defense": AI_NEXUS_DEFENSE,
};

export const DEFENSE_SLUGS = Object.keys(DEFENSE_PACKS) as DefenseSlug[];

export function isDefenseSlug(value: string): value is DefenseSlug {
  return DEFENSE_SLUGS.includes(value as DefenseSlug);
}
