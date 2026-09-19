import { describe, expect, it } from 'vitest';
import { mailDefaults, mailEvents, mailPayload, mailProblem } from './MailSettings';

describe('mailProblem', () => {
  it('accepts the default, which is off and empty', () => {
    expect(mailProblem(mailDefaults)).toBe('');
  });

  it('only demands a relay and a sender once mail is on', () => {
    expect(mailProblem({ ...mailDefaults, enabled: true })).toContain('릴레이 주소');
    expect(mailProblem({ ...mailDefaults, enabled: true, smtp_host: 'smtp.corp' })).toContain('보내는 주소');
    expect(mailProblem({ ...mailDefaults, enabled: true, smtp_host: 'smtp.corp', from_address: 'igame@corp.example' })).toBe('');
  });

  it('names the field the server would refuse', () => {
    expect(mailProblem({ ...mailDefaults, smtp_port: 70000 })).toContain('포트');
    expect(mailProblem({ ...mailDefaults, timeout_seconds: 0 })).toContain('제한 시간');
    expect(mailProblem({ ...mailDefaults, security: 'ssl' })).toContain('보안');
    expect(mailProblem({ ...mailDefaults, from_address: 'igame' })).toContain('이메일 주소');
    expect(mailProblem({ ...mailDefaults, base_url: 'igame.corp' })).toContain('절대 주소');
  });
});

describe('mailPayload', () => {
  it('leaves a blank password out so the stored one is kept', () => {
    // The screen never receives the password; sending an empty one would
    // read as "erase it" if the server did not carry the old value over.
    expect(mailPayload({ ...mailDefaults, username: 'igame', password: '' })).not.toHaveProperty('password');
    expect(mailPayload({ ...mailDefaults, username: 'igame', password: 'new' })).toHaveProperty('password', 'new');
  });

  it('sends numbers as numbers and trims what was typed', () => {
    const payload = mailPayload({ ...mailDefaults, smtp_port: '2525', timeout_seconds: '15', smtp_host: ' smtp.corp ' });
    expect(payload.smtp_port).toBe(2525);
    expect(payload.timeout_seconds).toBe(15);
    expect(payload.smtp_host).toBe('smtp.corp');
  });
});

describe('mailEvents', () => {
  it('uses the notify_<event> keys the standard names', () => {
    expect(mailEvents.map((event) => event.key)).toEqual(['notify_approval_requested', 'notify_approval_decided', 'notify_ranking_moderated']);
  });
});
