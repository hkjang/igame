import { describe, expect, it } from 'vitest';
import { maxSnippetBytes, policyOrigins, providers, trackingProblem, withAllowedHost } from './TrackingSettings';

describe('providers', () => {
  it('offers Momento first', () => {
    // The self-hosted collector is the one choice that keeps visit data inside
    // the network, so it is the first thing an administrator sees.
    expect(providers[0].value).toBe('momento');
  });
});

describe('trackingProblem', () => {
  it('accepts the default and a complete Momento setup', () => {
    expect(trackingProblem({ enabled: false, provider: 'none' })).toBe('');
    expect(trackingProblem({ enabled: true, provider: 'momento', momento_url: 'https://momento.company.local', momento_site_id: 'igame' })).toBe('');
  });

  it('names the missing field before the server answers in English', () => {
    expect(trackingProblem({ provider: 'momento', momento_url: 'https://m.local' })).toContain('사이트 ID');
    expect(trackingProblem({ provider: 'momento', momento_url: 'momento.local', momento_site_id: '1' })).toContain('절대 주소');
    expect(trackingProblem({ provider: 'ga4' })).toContain('측정 ID');
    expect(trackingProblem({ provider: 'matomo', matomo_site_id: '1' })).toContain('Matomo 주소');
    expect(trackingProblem({ provider: 'custom', custom_snippet: '  ' })).toContain('비어');
  });

  it('measures the snippet in bytes, the way the server does', () => {
    expect(trackingProblem({ provider: 'custom', custom_snippet: '<script>' + 'x'.repeat(maxSnippetBytes - 17) + '</script>' })).toBe('');
    // Hangul is three bytes a letter; a snippet that looks short can still be
    // over the limit.
    expect(trackingProblem({ provider: 'custom', custom_snippet: '한'.repeat(maxSnippetBytes / 3 + 1) })).toContain('바이트');
  });

  it('refuses an allow-list entry that is not an origin', () => {
    expect(trackingProblem({ provider: 'momento', momento_url: 'https://m.local', momento_site_id: '1', allowed_hosts: 'tracker.local' })).toContain('허용 출처');
    expect(trackingProblem({ provider: 'momento', momento_url: 'https://m.local', momento_site_id: '1', allowed_hosts: 'https://a.local, https://*.b.local\nhttp://c.local:8080' })).toBe('');
  });
});

describe('policyOrigins', () => {
  it('adds nothing for Momento behind the proxy', () => {
    expect(policyOrigins({ provider: 'momento', momento_url: 'https://momento.company.local', momento_site_id: 'igame' })).toEqual([]);
    expect(policyOrigins({ provider: 'momento', momento_url: 'https://momento.company.local/', momento_site_id: 'igame', momento_proxy: false })).toEqual(['https://momento.company.local']);
  });

  it('reads the origins a pasted snippet names', () => {
    const snippet = `<script src="https://tracker.local/t.js"></script><script>fetch('https://tracker.local/collect');new Image().src='http://pixel.local:8080/p.gif?id=1'</script>`;
    expect(policyOrigins({ provider: 'custom', custom_snippet: snippet, allowed_hosts: 'https://*.wild.local' })).toEqual(['https://tracker.local', 'http://pixel.local:8080', 'https://*.wild.local']);
  });
});

describe('withAllowedHost', () => {
  it('appends once and keeps what was there', () => {
    const once = withAllowedHost({ allowed_hosts: 'https://a.local' }, 'https://b.local');
    expect(once.allowed_hosts).toBe('https://a.local, https://b.local');
    expect(withAllowedHost(once, 'https://B.local')).toBe(once);
    expect(withAllowedHost({}, 'https://a.local').allowed_hosts).toBe('https://a.local');
  });
});
