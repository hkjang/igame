# igame CRU 실무 운영 매뉴얼 (CRU Operations Manual)

본 매뉴얼은 igame 플랫폼에서 게임 콘텐츠, 시즌, 이벤트, 밸런스를 등록(Create), 조회 및 검증(Read), 수정 및 배포(Update)하는 관리자 실무 지침서입니다.

---

## 🛠️ 1. 게임 콘텐츠 수명주기 (Content Lifecycle)

igame의 모든 게임 콘텐츠는 데이터 무결성과 밸런스 안정을 위해 엄격한 단계를 거칩니다:

1. **초안 작성 (Draft):** Designer / Studio에서 파라미터 및 스테이지 구성
2. **샌드박스 테스트 (Preview & Test):** 임시 UUID로 실제 게임 런타임에서 동작 검증
3. **승인 검토 (Approval Queue):** 팀장 및 검토자의 변경 내역 및 diff 확인
4. **버전 고정 게시 (Publish):** 불변 해시(Content Hash)와 함께 서비스 적용
5. **텔레메트리 검증 (Telemetry Validation):** 클라이언트가 보고한 전투 기록의 시계열 정합성 검증

---

## 🏰 2. RealmGuard Designer 운영 (`/admin/realmguard`)

- **스테이지 구성:** 10개 캠페인 스테이지의 경로 노드, 스폰 시간, 웨이브 수 정의
- **타워 및 업그레이드:** 4종 기본 타워(궁수, 보병, 마법사, 포병)의 공격력, 사거리, 쿨다운, 골드 비용 조정
- **적 및 보스 데이터:** 일반 몹 및 보스의 이동 속도, 체력, 방어력, 특수 스킬(무적, 순간이동 등) 지정
- **동시성 제어:** `If-Match` ETag 헤더를 이용한 낙관적 락(Optimistic Concurrency Control) 적용

---

## 🛡️ 3. Defense Content Studio 운영 (`/admin/defense`)

- **게임별 슬러그 분리:** Office Guardians, Cyber Fortress, AI Nexus Defense 각각의 규칙 세트 독립 관리
- **교육 시나리오 이벤트:** 게임 도중 발생하는 퀴즈, 선택지, 위협 시나리오 및 보상 배점 설정
- **실시간 연습 모드:** 게시 전 관리자 전용 세션으로 즉시 플레이 테스트

---

## 📋 4. 팀 승인 거버넌스 (`/reviews`)

- 관리자가 승인 정책을 활성화하면 모든 게임 및 밸런스 변경은 `/reviews` 대기열에 진입
- 검토자는 전후 diff와 샌드박스 미리보기를 확인한 뒤 승인 또는 반려 사유 작성
- 모든 처리 내역은 `/admin/audit` 감사 로그에 영구 기록
