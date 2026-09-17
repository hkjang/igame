import { describe, expect, it } from 'vitest';
import { mcpMetadataUrlFor, mcpOAuthValues, mcpResourceFor, words } from './McpOAuthSettings';

describe('mcpResourceFor', () => {
  it('prefers what the administrator typed', () => {
    expect(mcpResourceFor('https://mcp.example.test/mcp', 'https://games.example.test', 'http://localhost')).toBe('https://mcp.example.test/mcp');
  });

  it('derives from the public URL the way the server does, and from the page origin last', () => {
    // The server trims trailing slashes before appending /mcp; the screen
    // must show the same string the metadata will advertise.
    expect(mcpResourceFor('', 'https://games.example.test/', 'http://localhost')).toBe('https://games.example.test/mcp');
    expect(mcpResourceFor(undefined, '', 'http://localhost:5173')).toBe('http://localhost:5173/mcp');
  });
});

describe('mcpMetadataUrlFor', () => {
  it('sits beside the resource identifier', () => {
    expect(mcpMetadataUrlFor('https://games.example.test/mcp')).toBe('https://games.example.test/.well-known/oauth-protected-resource/mcp');
    expect(mcpMetadataUrlFor('https://games.example.test/')).toBe('https://games.example.test/.well-known/oauth-protected-resource/mcp');
  });
});

describe('words and mcpOAuthValues', () => {
  it('reads a space- or comma-separated field into the list the server stores', () => {
    expect(words('claude-mcp cursor, codex')).toEqual(['claude-mcp', 'cursor', 'codex']);
    expect(words(['a', 'b'])).toEqual(['a', 'b']);
    expect(words(undefined)).toEqual([]);
  });

  it('survives a setting that was never stored', () => {
    expect(mcpOAuthValues({})).toEqual({});
    expect(mcpOAuthValues({ oauth: { enabled: true, audience: ['x'] } })).toEqual({ enabled: true, audience: ['x'] });
  });
});
