<div align="center">

# iGame

### Air-Gapped Offline Enterprise Game Platform

**사내 폐쇄망에서 게임을 독립적으로 등록하고 운영하는 엔터프라이즈 게임 플랫폼**

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)](https://golang.org)
[![React Version](https://img.shields.io/badge/React-19-61DAFB?style=flat&logo=react)](https://reactjs.org)
[![Phaser Version](https://img.shields.io/badge/Phaser-3.80+-E74C3C?style=flat)](https://phaser.io)
[![MCP Ready](https://img.shields.io/badge/MCP-Streamable%20HTTP-FF6B6B?style=flat)](https://modelcontextprotocol.io)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

[🌐 웹 쇼케이스 (한국어)](docs/index.html) · [🌐 English Showcase](docs/en/index.html) · [🎬 3분 시연 영상](docs/igame-demo.mp4)  
[📘 공식 사용자 가이드 (PDF)](docs/igame_User_Guide.pdf) · [📗 CRU 매뉴얼 (PDF)](docs/igame_CRU_Operations_Manual.pdf) · [📙 아키텍처 백서 (PDF)](docs/igame_Architecture_and_Security_Whitepaper.pdf)

</div>

---

## 🎬 3분 실전 데모 영상

> 💡 **[3분 데모 비디오 파일 직접 다운로드 / 보기](docs/media/igame-demo.mp4)**  
> 로그인 → 포털 홈 → RealmGuard 타워디펜스 → Defense Series 3종 → 실시간 랭킹 및 AI Co-Pilot 분석 → 관리자 Designer/Studio까지 전 과정을 60fps HD 영상으로 확인하실 수 있습니다.

---

- Backend: Go, PostgreSQL, REST/SSE, MCP Streamable HTTP
- Frontend: React + TypeScript + Vite, Phaser(RealmGuard runtime)
- UI: 접근성과 장기 유지보수가 검증된 Material UI(MUI)를 사용하고 애플리케이션 자산은 번들에 포함합니다. 본문 기준 16px, 100~125% 개인 글자 확대, 키보드 포커스와 대비를 유지합니다. 어두운 화면과 밝은 화면을 모두 제공하며 기본값은 운영체제 설정을 따릅니다. 두 팔레트 모두 본문·버튼 대비가 WCAG AA를 만족하는지 테스트로 확인합니다.
- Deployment: `igame:v<version>` 단일 이미지. 최종 runtime은 package manager와 shell이 없는 `scratch` 기반이며 외부 CDN이나 실행 중 패키지 다운로드가 없습니다.

초기에는 PostgreSQL만 사용하는 모듈러 모놀리스로 운영하고, 동시 접속 규모가 커질 때에만 실시간 런타임이나 랭킹을 분리하는 것이 권장됩니다.

## RealmGuard

`v0.2.0`은 igame의 독자 타워 디펜스 IP인 **RealmGuard**를 포함합니다. 콘텐츠는 현재 `0.3.1`이며 재조정된 타워 분기와 캐릭터별 코드 생성 초상·전장 실루엣·공격 효과·실시간 HP/레벨 HUD를 제공합니다. 10개 캠페인 stage와 끝없는 균열, 일반 enemy 10종과 boss 2종, 4종 tower와 8개 upgrade branch, 3명 hero, 3개 active skill을 데이터 기반으로 구성합니다. `/games/realmguard`의 게임 화면은 Phaser를 사용하지만 엔진과 모든 실행 자산은 Vite/Go 정적 bundle과 단일 Docker 이미지 안에 포함되어 폐쇄망에서 CDN 없이 실행됩니다.

RealmGuard의 명칭·등장 개체·stage/balance 데이터와 코드 생성 그래픽은 이 프로젝트의 독자 구현이며, Kingdom Rush를 포함한 제3자 게임의 코드·서사·캐릭터·맵·시청각 자산을 포함하지 않습니다. Phaser는 MIT 라이선스의 실행 framework로만 사용하며 package metadata와 license를 이미지 `/licenses/phaser`에 함께 보관합니다.

`v0.6.0`부터 RealmGuard의 공식 결과는 서버가 재현합니다. 전투 규칙 전체가 renderer와 분리된 결정론적 kernel에 있고, 화면은 그 kernel이 만든 상태를 그리기만 합니다. 브라우저는 무슨 일이 일어났는지 보고하는 대신 플레이어가 무엇을 했는지만 원장으로 제출하며, 서버는 세션에 고정된 콘텐츠로 같은 전투를 처음부터 다시 실행해 남은 생명·자원·처치·유출·완료 wave·승패를 직접 계산합니다. 수정된 client는 자신의 입력을 바꿀 수는 있어도 그 입력의 결과를 바꿀 수 없습니다.

관리자는 `/admin/realmguard`의 Designer에서 checksum/`If-Match`로 콘텐츠 초안을 편집·Test·게시하고, 승인 정책을 켜면 `/reviews`에서 미리보기 후 승인·반려합니다. Manager 검토는 manager/작성자 모두 같은 비어 있지 않은 team일 때만 열립니다. 게임 세션은 화면이 읽은 published config UUID를 다시 확인해 pin하며, 공식 전투 결과는 UUID와 1-based sequence로 서버가 수신한 브라우저 자가보고 원장의 순서·시각·누적 일관성을 `server_received_telemetry_v1`으로 검증합니다. 이는 완전한 서버 게임 시뮬레이션은 아닙니다. 게시 version, 검증 경계, telemetry와 운영·백업 절차는 [RealmGuard 운영 가이드](docs/realmguard.md)를 참조하세요.

## Defense Series

Defense Series 콘텐츠 `0.4.0`은 RealmGuard의 데이터 기반 방어 메커니즘을 공통 Defense Engine으로 확장한 세 게임과 28개의 이름 있는 전술 지도를 제공합니다. 10개 전장 geometry에 게임별 `map_style`을 결합하며, stage 선택 미니맵에서 경로와 건설 지점, lane 수를 확인할 수 있고 일부 전장은 적이 실제 두 진입로로 나뉘어 들어옵니다.

- `/games/office-guardians`: 조직과 직무의 협업을 다루는 **Office Guardians**
- `/games/cyber-fortress`: 위협 인지와 보안 대응을 학습하는 **Cyber Fortress**
- `/games/ai-nexus-defense`: AI 보안·품질·비용·거버넌스를 학습하는 **AI Nexus Defense**

세 게임은 같은 실행 엔진과 포털 세션을 사용하지만 콘텐츠, 규칙, 진행도, 결과와 랭킹은 slug별로 분리됩니다. Cyber Fortress와 AI Nexus Defense의 교육 선택은 게임 점수와 별개의 학습 결과로 저장됩니다. 관리자는 `/admin/defense`의 Defense Content Studio에서 stage, wave, unit, 교육 이벤트와 balance를 편집하고 Test·연습 미리보기·승인/반려·게시할 수 있습니다. 브라우저가 읽은 published content UUID는 세션의 `defense_content_version_id`로 정확히 고정되며, 전용 결과·랭킹 경로만 공식 기록을 생성합니다. 자세한 운영 및 API 계약은 [Defense Series 운영 가이드](docs/defense-series.md)를 참조하세요.

## 접근성과 운영

`v0.7.0`은 네 게임의 적과 타워에 개성을 부여합니다. 50종의 적이 같은 색 원 대신 자기 행동에서 파생된 실루엣과 특성 표식을 갖고, 타워는 3레벨에서 고른 분기를 전장에서 보여줍니다. 각 게임의 스테이지 선택 화면에는 그 전장이 보내는 개체와 대처법을 읽을 수 있는 로스터가 생겼습니다. 게시된 콘텐츠와 전투 규칙은 그대로이며, `v0.6.0`의 서버 재현 확정과 기존 포털 접근성·운영 도구·실행 성능 계약도 유지됩니다. 게시된 콘텐츠는 그대로이므로 새 콘텐츠 UUID로 전환하지 않고 캠페인·영웅·스킬 진행도와 기존 랭킹을 모두 보존합니다. 이전 릴리스에서 `server_received_telemetry_v1`로 검증된 기록은 그대로 남고, 새 기록은 `server_replay_v1`로 표시됩니다.

포털은 본문 건너뛰기 link, 화면 전환 시 focus 이동과 음성 안내, route별 브라우저 제목을 제공합니다. 어두운 화면과 밝은 화면을 모두 지원하고 기본값은 운영체제 설정을 따르며, 두 palette 모두 본문·버튼 대비가 WCAG AA를 만족하는지 테스트로 확인합니다. 게시된 공지는 `/notices`에서 전체를 검색해 볼 수 있습니다. 사용자에게 보이는 API 오류는 한국어로 표시하고, session이 만료되면 로그인 화면으로 돌려보낸 뒤 보던 위치로 복귀합니다.

관리자 콘솔은 감사 로그·사용자·게임 목록을 page 단위로 조회하며 필터를 적용한 뒤의 전체 건수를 함께 표시합니다. 감사 로그는 같은 검색 조건으로 CSV 내보내기가 가능하며, 화면의 page가 아니라 조건에 맞는 전체 기록을 streaming합니다. `/admin` 대시보드의 서비스 상태 card는 DB 도달 여부와 커넥션 pool, 정책 5종 on-off, 게임별 게시 콘텐츠 version, 증가형 table의 누적 행 수를 한곳에 모읍니다. 최종 runtime image에는 shell이 없어 컨테이너 내부를 직접 볼 수 없으므로 이 화면이 1차 진단 지점입니다.

로컬 암호 로그인에는 rate limiter가 내장되고, 알 수 없는 계정도 실제 계정과 같은 비용의 bcrypt 비교를 수행해 응답 시간으로 계정 존재 여부를 알아낼 수 없습니다. 거부된 Defense Series 결과는 RealmGuard와 같은 방식으로 감사에 남고 운영 report에 건수가 집계됩니다. 진입 bundle은 route 단위 분할로 2,127 kB에서 638 kB(gzip 618 → 200 kB)로 줄어, 게임을 실행하지 않는 사용자는 Phaser를 내려받지 않습니다. 자세한 동작과 업그레이드 시 확인할 항목은 [운영 및 장애 대응](docs/operations.md)과 [보안 및 키 관리](docs/security.md)를 참조하세요.

## 빠른 시작

PostgreSQL 데이터베이스를 먼저 준비한 뒤 `.env.example`을 `.env`로 복사하고 네 값을 채웁니다. 애플리케이션이 받는 환경변수는 다음 네 개뿐입니다.

| 변수 | 용도 |
| --- | --- |
| `POSTGRES_DSN` | PostgreSQL 연결 문자열 |
| `BOOTSTRAP_ADMIN` | 최초 로컬 관리자 ID |
| `BOOTSTRAP_ADMIN_PASSWORD` | 최초 로컬 관리자 암호 |
| `ENCRYPTION_KEY` | 저장 비밀을 감싸는 32바이트 마스터 키 |

```bash
umask 077
cp .env.example .env
docker compose up -d
docker compose ps
bash ./scripts/smoke-test.sh http://127.0.0.1:8080
```

`ENCRYPTION_KEY` 생성 예:

```bash
printf 'ENCRYPTION_KEY=base64:%s\n' "$(openssl rand -base64 32 | tr -d '\n')"
```

최초 로그인 후 `/admin`에서 공개 URL, Keycloak, AI 공급자, 개인정보, 시간 정책, 승인 흐름, 역할과 키 권한을 설정합니다. 설정이 없는 승인 흐름은 실행 경로 자체에서 제외됩니다. 서비스 버전은 로그인 화면과 프로필 메뉴에서 확인할 수 있습니다.

상세 설치 절차는 [오프라인 설치](docs/offline-install.md), SSO는 [Keycloak 연동](docs/keycloak.md)을 따릅니다.

## 개발과 검증

```bash
make test
make test-race
make build
make docker-build
make smoke
```

릴리스 이미지는 `VERSION`을 기준으로 만듭니다. 결과물 `dist/igame-v<version>.tar.gz`는 별도 tar 포장 없이 `docker save igame:v<version> | gzip`의 출력입니다.

서비스, Docker image, web application과 `gamehub-js` SDK는 이 release에서 root `VERSION` `0.7.0`으로 정렬됩니다. RealmGuard 콘텐츠 `0.3.1`과 Defense Series 콘텐츠 `0.4.0`은 별도 수명 주기를 가지므로 서비스 버전으로 덮어쓰지 않습니다.

```bash
make release
bash ./scripts/verify-release.sh dist/igame-v$(cat VERSION).tar.gz
gzip -dc dist/igame-v$(cat VERSION).tar.gz | docker load
```

## 문서

- [오프라인 설치](docs/offline-install.md)
- [Keycloak OIDC](docs/keycloak.md)
- [운영 및 장애 대응](docs/operations.md)
- [백업과 복구](docs/backup-restore.md)
- [보안 및 키 관리](docs/security.md)
- [REST/SSE API](docs/api.md)
- [MCP](docs/mcp.md)
- [RealmGuard 운영 가이드](docs/realmguard.md)
- [Defense Series 운영 가이드](docs/defense-series.md)
- [릴리스 절차](docs/release.md)

## 라이선스

Apache License 2.0. 자세한 내용은 [LICENSE](LICENSE)를 참조하세요.
