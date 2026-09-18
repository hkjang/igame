import { useState } from 'react';
import ContentCopyRounded from '@mui/icons-material/ContentCopyRounded';
import SaveRounded from '@mui/icons-material/SaveRounded';
import { Alert, Button, Card, CardContent, Divider, FormControlLabel, IconButton, Stack, Switch, TextField, Tooltip, Typography } from '@mui/material';
import { api } from '../../api/client';
import { copyText } from '../../api/clipboard';
import { useRetainFocus } from '../../hooks/useRetainFocus';
import { useSnackbar } from '../../state/SnackbarContext';

type Values = Record<string, unknown>;

/** The scopes an SSO token gets when the administrator names none. */
export const defaultMcpOAuthScopes = ['mcp:access', 'games:read', 'rankings:read', 'profile:read'];

/** The `oauth` object inside the stored `mcp` setting, or its defaults. */
export function mcpOAuthValues(mcp: Values): Values {
  const oauth = mcp.oauth;
  return oauth && typeof oauth === 'object' ? { ...(oauth as Values) } : {};
}

/** A space-separated field, as a list the server stores. */
export function words(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  return String(value ?? '').split(/[\s,]+/).filter(Boolean);
}

/**
 * The resource identifier a token must be minted for: what the administrator
 * typed, or else the service public URL plus /mcp — the same derivation the
 * server makes, so the screen shows what the metadata will say.
 */
export function mcpResourceFor(resource: unknown, publicUrl: unknown, origin: string): string {
  const typed = String(resource ?? '').trim();
  if (typed) return typed;
  const base = String(publicUrl ?? '').trim().replace(/\/+$/, '') || origin;
  return `${base}/mcp`;
}

/** Where a refused client is sent: beside the resource identifier. */
export function mcpMetadataUrlFor(resource: string): string {
  const base = resource.endsWith('/mcp') ? resource.slice(0, -'/mcp'.length) : resource.replace(/\/+$/, '');
  return `${base}/.well-known/oauth-protected-resource/mcp`;
}

function CopyField({ label, value, help }: { label: string; value: string; help: string }) {
  const { notify } = useSnackbar();
  const copy = async () => {
    if (await copyText(value)) notify('복사했습니다.', 'success');
    else notify('브라우저가 복사를 허용하지 않았습니다. 값을 직접 선택해 복사해 주세요.', 'warning');
  };
  return <TextField label={label} value={value} helperText={help} InputProps={{ readOnly: true, endAdornment: <Tooltip title="복사"><IconButton aria-label={`${label} 복사`} onClick={() => void copy()}><ContentCopyRounded /></IconButton></Tooltip> }} />;
}

/**
 * The second door into /mcp: a Keycloak access token beside the personal key.
 * Off by default; the switch needs the issuer from the OIDC card above it.
 */
export function McpOAuthSettings({ initial, oidc, service }: { initial: Values; oidc: Values; service: Values }) {
  const { notify } = useSnackbar();
  const [values, setValues] = useState<Values>(mcpOAuthValues(initial));
  const [busy, setBusy] = useState(false);
  const saveRef = useRetainFocus<HTMLButtonElement>(busy);
  const change = (key: string, value: unknown) => setValues({ ...values, [key]: value });
  const issuer = String(oidc.issuer ?? '').trim();
  const resource = mcpResourceFor(values.resource, service.public_url, window.location.origin);
  const metadataUrl = mcpMetadataUrlFor(resource);
  const save = async () => {
    setBusy(true);
    try {
      const oauth: Values = { enabled: Boolean(values.enabled), resource: String(values.resource ?? '').trim(), audience: words(values.audience), scopes: words(values.scopes) };
      await api.saveAdminSetting('mcp', { ...initial, oauth });
      notify('MCP SSO 설정을 저장했습니다.', 'success');
    } catch (cause) {
      notify(cause instanceof Error ? cause.message : '저장하지 못했습니다.', 'error');
    } finally {
      setBusy(false);
    }
  };
  return <Card><CardContent sx={{ p: { xs: 2.5, md: 3.5 } }}>
    <Typography variant="h3">MCP SSO (OAuth)</Typography>
    <Typography color="text.secondary" mt={.7}>개인 키 없이 Keycloak 액세스 토큰으로 /mcp에 접속하게 합니다. 키는 그대로 동작하고, 토큰은 /mcp에서만 받습니다.</Typography>
    <Divider sx={{ my: 3 }} />
    <Stack spacing={2}>
      <FormControlLabel control={<Switch checked={Boolean(values.enabled)} disabled={!issuer} onChange={(event) => change('enabled', event.target.checked)} />} label="SSO 토큰으로 MCP 접속 허용" />
      {!issuer && <Alert severity="warning">위 Keycloak OIDC 카드에 Issuer URL을 먼저 저장해야 켤 수 있습니다. 토큰은 그 issuer의 서명 키로 검사합니다.</Alert>}
      {Boolean(values.enabled) && <Alert severity="info">클라이언트에는 아래 MCP 주소 하나만 주면 됩니다. 클라이언트가 401의 <code>resource_metadata</code>를 따라 메타데이터를 읽고 Keycloak에서 스스로 토큰을 받아 옵니다. 계정은 만들지 않으므로, 사용자는 웹으로 먼저 한 번 로그인해 두어야 합니다.</Alert>}
      <TextField label="리소스 식별자 (mcp.oauth.resource)" value={String(values.resource ?? '')} onChange={(event) => change('resource', event.target.value)} placeholder={mcpResourceFor('', service.public_url, window.location.origin)} helperText="비우면 서비스 공개 URL + /mcp 를 씁니다. 클라이언트가 실제로 접속하는 공개 HTTPS 주소여야 합니다." />
      <CopyField label="MCP 주소" value={resource} help="MCP 클라이언트에 넣는 URL이자, Keycloak Audience 매퍼의 Included Custom Audience 값입니다." />
      <CopyField label="메타데이터 주소" value={metadataUrl} help="401 응답의 WWW-Authenticate가 가리키는 RFC 9728 문서입니다. 인증 없이 curl로 확인할 수 있습니다." />
      <TextField label="허용 대상 (mcp.oauth.audience)" value={Array.isArray(values.audience) ? values.audience.join(' ') : String(values.audience ?? '')} onChange={(event) => change('audience', event.target.value)} helperText="공백으로 구분. 토큰의 aud 또는 azp와 비교합니다. Keycloak 26은 클라이언트 ID를 azp에 담으므로, Audience 매퍼 없이 쓰려면 MCP 클라이언트 ID를 여기에 적으세요." />
      <TextField label="범위 (mcp.oauth.scopes)" value={Array.isArray(values.scopes) ? values.scopes.join(' ') : String(values.scopes ?? '')} onChange={(event) => change('scopes', event.target.value)} placeholder={defaultMcpOAuthScopes.join(' ')} helperText="공백으로 구분. SSO 토큰 주체가 쓸 수 있는 권한이며 개인 키와 같은 역할 정책의 교집합만 인정됩니다. 비우면 읽기 범위 기본값입니다. admin:* 은 줄 수 없습니다." />
    </Stack>
    <Button ref={saveRef} variant="contained" startIcon={<SaveRounded />} onClick={() => void save()} disabled={busy} sx={{ mt: 3 }}>설정 저장</Button>
  </CardContent></Card>;
}
