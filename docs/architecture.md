# igame 아키텍처 및 보안 백서 (Architecture & Security Whitepaper)

본 문서는 igame의 단일 바이너리 모듈러 모놀리스 아키텍처, 2계층 봉투 암호화, Keycloak OIDC SSO 및 Model Context Protocol (MCP) 연동에 대한 기술 사양서입니다.

---

## 🏛️ 1. 시스템 토폴로지 및 런타임 구조

igame은 Go 1.26+ 기반의 단일 실행 바이너리 안에 React 19 정적 자산과 Phaser 엔진 번들을 내장(go:embed)하여, 외부 인터넷 통신이 일체 없는 폐쇄망(Air-Gapped) 환경에서 동작합니다.

```
Client Browser (React 19 + Phaser)
                 │ (HTTPS / Bearer Token / Session)
                 ▼
┌──────────────────────────────────────────────────────────────┐
│ igame Modular Monolith (:8080)                               │
│  ├─ Core Control Plane (REST / SSE / MCP Streamable HTTP)    │
│  ├─ Keycloak OIDC SSO & Local Bootstrap Auth                 │
│  ├─ Deterministic Battle Kernel & Replay Verifier            │
│  ├─ Game Session & Telemetry Ledger Validator                │
│  ├─ Leaderboard, Season & Tournament Engine                  │
│  ├─ Per-User AES-256-GCM Envelope Encryption Vault           │
│  └─ Embedded Web Assets (dist/*)                             │
└──────────────────────────────────────────────────────────────┘
                 │ (SQL / pgxpool)
                 ▼
┌──────────────────────────────────────────────────────────────┐
│ PostgreSQL 16+ (ACID Transactional Data Store)               │
└──────────────────────────────────────────────────────────────┘
```

### 1.1 서버 권위 전투 검증 (Server-Authoritative Battle Replay)

RealmGuard의 전투 규칙은 renderer와 완전히 분리된 결정론적 kernel 하나로 존재하며, 브라우저(TypeScript)와 서버(Go)에 같은 알고리즘으로 구현되어 있습니다. kernel은 고정 50ms step으로만 진행하고 벽시계와 난수를 사용하지 않으며, 사칙연산과 제곱근만으로 작성해 두 언어가 같은 IEEE-754 배정밀도 결과를 내도록 보장합니다.

```
Browser                                    Server
  BattleKernel  ──inputs──▶ ledger ──────▶ battle.Kernel (Go)
  (renders state)                            │ replays from published content
  Phaser scene                               ▼
  (pixels only)                            score · stars · progress
```

브라우저는 무슨 일이 일어났는지 보고하지 않고 플레이어가 무엇을 했는지(`{tick, op, …}`)만 제출합니다. 서버는 세션에 고정된 published 콘텐츠를 kernel 입력으로 투영해 digest를 재계산하고, 브라우저가 보낸 digest와 일치할 때만 원장을 재생해 남은 생명·자원·처치·유출·완료 wave·승패를 직접 산출합니다. 두 구현의 동치성은 저장소에 커밋된 replay vector와 콘텐츠 투영 fixture로 CI에서 강제됩니다.

---

## 🔐 2. 2계층 봉투 암호화 (Envelope Encryption)

- **마스터 키 (MEK):** 환경변수 `ENCRYPTION_KEY` (32바이트 AES-256-GCM)로 관리
- **데이터 암호화 키 (DEK):** 사용자 및 테넌트별로 고유한 DEK를 생성하여 마스터 키로 래핑
- **비밀 정보 보호:** 모든 개인 API 키, OIDC 클라이언트 시크릿, AI API 토큰은 DEK로 암호화되어 DB에 저장
- **Zero-Downtime Key Rotation:** 구 키와 신 키의 유예 기간을 두어 서비스 중단 없는 실시간 키 회전 지원

---

## 🔌 3. Model Context Protocol (MCP) 연동

- **규격:** Model Context Protocol (Streamable HTTP)
- **엔드포인트:** `/mcp`
- **세션 유지:** `GET /mcp`은 서버가 먼저 보내는 message가 없는 열린 SSE stream이며 25초마다 keep-alive만 전송합니다. reverse proxy의 stream idle timeout은 이보다 길게 설정해야 합니다
- **제공 도구:**
  - `games_list` / `game_get`: 등록된 게임 목록과 개별 메타데이터 조회
  - `leaderboard_get`: 게임별 개인·부서·팀 랭킹 조회
  - `defense_config_get` / `defense_rankings_get`: Defense Series의 게시된 콘텐츠와 버전 고정 랭킹 조회
  - `profile_get` / `events_list`: 인증된 사용자 프로필과 사내 이벤트 조회
  - `game_session_start` / `score_submit`: 서명된 게임 세션 시작과 점수 제출
- **권한:** API 키로 접근할 때는 도구별로 `games:read`, `rankings:read`, `profile:read`, `sessions:write`, `scores:write` scope를 각각 요구하며 `mcp:access`가 함께 있어야 합니다
