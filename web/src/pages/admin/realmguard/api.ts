import { api } from '../../../api/client';
import { normalizeRealmGuardConfig } from '../../../games/realmguard/api';
import type { RealmGuardConfig, RealmSection } from '../../../games/realmguard/types';

export interface RealmGuardVersionRecord {
  id: string;
  version_no: number;
  label: string;
  status: 'draft' | 'testing' | 'pending_approval' | 'approved' | 'published' | 'archived';
  content_version: string;
  stage_version: string;
  balance_version: string;
  asset_version: string;
  checksum: string;
  notes?: string;
  created_by?: string;
  created_at: string;
  updated_at: string;
  tested_at?: string;
  approval_requested_at?: string;
  approved_at?: string;
  published_at?: string;
  review_comment?: string;
  creator?: { id: string; username: string; display_name?: string; team?: string };
  changed_sections?: RealmSection[];
}

export const realmGuardDesignerAPI = {
  versions: () => api.request<{ items: RealmGuardVersionRecord[] }>('/api/v1/admin/realmguard/versions'),
  createVersion: (input: { label?: string; notes: string; asset_version?: string }) => api.request<{ version: RealmGuardVersionRecord }>('/api/v1/admin/realmguard/versions', { method: 'POST', body: JSON.stringify(input) }),
  section: (section: RealmSection, versionId?: string) => api.requestEnvelope<{ version: RealmGuardVersionRecord; section: RealmSection; data: unknown }>(`/api/v1/admin/realmguard/drafts/${section}${versionId ? `?version_id=${encodeURIComponent(versionId)}` : ''}`),
  saveSection: (section: RealmSection, data: unknown, versionId: string, checksum: string) => api.requestEnvelope<{ version: RealmGuardVersionRecord; section: RealmSection; data: unknown }>(`/api/v1/admin/realmguard/drafts/${section}?version_id=${encodeURIComponent(versionId)}`, { method: 'PUT', headers: { 'If-Match': `"${checksum}"` }, body: JSON.stringify({ data }) }),
  testVersion: (id: string) => api.request<{ version: RealmGuardVersionRecord; validation: Record<string, unknown> }>(`/api/v1/admin/realmguard/versions/${encodeURIComponent(id)}/test`, { method: 'POST' }),
  approveVersion: (id: string, comment = '') => api.request<{ version: RealmGuardVersionRecord }>(`/api/v1/admin/realmguard/versions/${encodeURIComponent(id)}/approve`, { method: 'POST', body: JSON.stringify({ decision: 'approved', comment }) }),
  publishVersion: (id: string) => api.request<{ version: RealmGuardVersionRecord; published?: boolean; approval_required?: boolean }>(`/api/v1/admin/realmguard/versions/${encodeURIComponent(id)}/publish`, { method: 'POST', body: '{}' }),
  telemetry: (days = 30) => api.request<Record<string, unknown>>(`/api/v1/admin/realmguard/telemetry?days=${days}`),
};

export async function getRealmGuardPreview(id: string): Promise<{ config: RealmGuardConfig; version: RealmGuardVersionRecord }> {
  const raw = await api.request<Record<string, unknown>>(`/api/v1/realmguard/versions/${encodeURIComponent(id)}/preview`);
  return { config: normalizeRealmGuardConfig(raw), version: raw.version as unknown as RealmGuardVersionRecord };
}

export function pendingRealmGuardVersions() {
  return api.request<{ items: RealmGuardVersionRecord[] }>('/api/v1/realmguard/versions/pending');
}

export function reviewRealmGuardVersion(id: string, decision: 'approved' | 'rejected', comment: string) {
  return api.request<{ version: RealmGuardVersionRecord; decision: string }>(`/api/v1/realmguard/versions/${encodeURIComponent(id)}/review`, { method: 'POST', body: JSON.stringify({ decision, comment }) });
}
