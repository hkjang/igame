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
