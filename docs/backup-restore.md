# 백업과 복구

PostgreSQL에는 사용자, 관리자 설정, 암호화된 OIDC/AI secret, 키 메타데이터, 게임·점수·감사 기록과 RealmGuard content version, draft/published snapshot, progress/loadout/result 및 세션 telemetry 원장이 저장됩니다. RealmGuard result의 `verification_method`/`attestation`/서버 생성 암호화 proof와 `game_telemetry.client_event_id`/`sequence_no`는 결과 검증 증적이므로 result와 같은 복구 시점으로 보존합니다. `/app/data`를 사용하는 업로드 자산이 있다면 DB와 같은 복구 시점으로 함께 보관합니다. `ENCRYPTION_KEY`가 없으면 DB의 암호화된 값은 복구할 수 없으므로 데이터 백업과 분리된 비밀 관리소에 escrow해야 합니다.

## 권장 정책

- 매일 custom-format 논리 백업, 주 1회 복구 훈련용 복제
- 운영 요구에 맞는 RPO/RTO를 문서화하고 백업 보존 기간 설정
- 백업 파일 AES-256 수준 저장 암호화, 전송 암호화, 최소 권한 접근
- 각 파일 SHA-256 기록 및 월 1회 실제 restore 검증
- `ENCRYPTION_KEY`는 백업 파일과 같은 위치에 저장하지 않음

## DB 백업

PostgreSQL server와 같은 major 버전의 `pg_dump`를 사용합니다.

```bash
umask 077
export POSTGRES_DSN='postgres://...'
bash ./scripts/backup.sh /secure/igame-backups
unset POSTGRES_DSN
```

스크립트는 임시 파일에 custom-format dump를 쓴 뒤 원자적으로 이름을 바꾸고 `.sha256`을 생성합니다. 대규모 설치에서는 조직 표준 WAL/PITR 백업도 함께 운영합니다.

## 업로드 자산 백업

DB 백업 시점과 맞춰 짧은 유지보수 창에 애플리케이션을 중지한 다음 named volume을 승인된 백업 도구로 보관합니다.

```bash
docker compose stop igame
docker cp igame:/app/data ./igame-data-backup
docker compose start igame
```

조직 백업 agent가 Docker volume을 직접 snapshot할 수 있다면 해당 방식을 우선합니다. 라이브 파일을 일관성 보장 없이 복사하지 마십시오.

## 복구

복구는 대상 DB의 기존 객체를 정리하므로 먼저 격리된 새 DB에서 연습합니다.

```bash
export POSTGRES_DSN='postgres://.../igame_restore?sslmode=verify-full'
bash ./scripts/restore.sh /secure/igame-backups/igame-YYYYMMDDTHHMMSSZ.dump RESTORE
unset POSTGRES_DSN
```

필요한 경우 중지된 컨테이너의 `/app/data`에 자산을 복원하고 ownership을 이미지의 비root 사용자와 맞춥니다. 네 런타임 환경변수를 다시 설정하되, 암호화된 secret을 읽으려면 반드시 백업 당시와 동일한 `ENCRYPTION_KEY`로 기동합니다.

복구 검증 항목:

1. `/readyz` 성공과 migration 오류 없음
2. 로컬 관리자 및 Keycloak 시험 로그인
3. 관리자 설정 secret 연결 시험
4. 개인 키가 기존 정책대로 인증되고 폐기 키는 거부됨
5. 게임 실행, 점수, 랭킹, 감사 로그의 기준 시점 일치
6. 로그인 화면과 프로필 버전 확인
7. RealmGuard active content/balance version, published checksum, progress와 result version tuple 일치
8. RealmGuard result의 `server_replay_v1` attestation, 재현에 사용한 입력 원장(`realmguard_results.ledger`), telemetry digest와 연결 session의 1-based sequence·UUID가 보존됨
9. `/games/realmguard`와 ready/wave/battle telemetry를 거친 공식 결과 제출이 외부 연결 없이 동작

## 키 분실과 회전

`ENCRYPTION_KEY` 분실은 DB restore로 해결되지 않습니다. 유실 시 암호화된 OIDC/AI secret을 새로 등록해야 합니다. 현재 마스터 키 hot rotation은 제공하지 않으므로 환경 값만 먼저 바꾸면 기존 secret 복호화가 실패합니다. 회전이 필요하면 유지보수 창에 기존 키로 secret 평문을 안전하게 재입력할 준비를 하고, 새 키로 기동한 뒤 OIDC/AI secret을 다시 저장·시험합니다. 개인 API 키는 별도의 단방향 검증값을 사용하므로 마스터 키 변경과 직접 연결되지 않습니다.
