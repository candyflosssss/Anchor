import { describe, expect, it } from 'vitest';
import fixture from '../../../../contract/connections.json';
import { checkedProfiles, emptySSH } from './connection.model';
import { checkedSession } from '../../desktop/transport';

describe('connection public contract', () => {
  it('accepts the shared local and remote fixture', () => {
    expect(checkedProfiles(fixture.profiles.data)).toEqual(fixture.profiles.data);
    expect(checkedSession(fixture.session.data).transport).toBe('ssh');
    expect(emptySSH.trust_new_host).toBe(false);
  });
  it('rejects duplicate names and invalid transport/state', () => {
    const profiles = fixture.profiles.data;
    expect(() => checkedProfiles([profiles[0], profiles[0]])).toThrow();
    expect(() => checkedProfiles([{ ...profiles[0], transport: 'http' }])).toThrow();
    expect(() => checkedProfiles([{ ...profiles[0], active: 'yes' } as never])).toThrow();
    expect(() => checkedProfiles([{ ...profiles[0], selected: 'yes' } as never])).toThrow();
  });
  it('preserves selected independently of active and accepts older hosts without it', () => {
    expect(checkedProfiles([{ ...fixture.profiles.data[1], active: false }])[0]).toMatchObject({ selected: true, active: false });
    expect(checkedProfiles([fixture.profiles.data[0]])[0]).not.toHaveProperty('selected');
  });
  it('distinguishes password storage capability from older hosts and rejects invalid markers', () => {
    const row = fixture.profiles.data[1];
    expect(checkedProfiles([{ ...row, password_saved: true }])[0].password_saved).toBe(true);
    for (const missing of [undefined, null]) {
      expect(checkedProfiles([{ ...row, password_saved: missing }])[0]).not.toHaveProperty('password_saved');
    }
    expect(() => checkedProfiles([{ ...row, password_saved: 'yes' } as never])).toThrow();
  });
  it('selects public fields so unknown secrets cannot propagate', () => {
    const rows = checkedProfiles([{ ...fixture.profiles.data[1], token: 'synthetic-private', endpoint: 'http://arbitrary' } as never]);
    expect(JSON.stringify(rows)).not.toContain('synthetic-private');
    expect(rows[0]).not.toHaveProperty('endpoint');
  });
});
