import { describe, expect, it } from 'vitest';
import fixture from '../../../../contract/proxies.json';
import { bindingFromValue, bindingValue, checkedWorkspace, listenPortValue, proxyAction, proxyError } from './proxies.model';
import { DesktopProblem } from '../../desktop/transport';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { ProxyBindings } from './proxies.view';
describe('proxy-v1 shared contract', () => {
  it('keeps inherit, explicit direct and proxy distinct', () => {
    const w = checkedWorkspace(fixture.workspace);
    expect(bindingValue(w, 'demo', '')).toBe('proxy:edge');
    expect(bindingValue(w, 'demo', 'second')).toBe('direct');
    expect(bindingValue(w, 'demo', 'system')).toBe('system');
    expect(bindingFromValue('demo', 'system', 'system')).toEqual(fixture.system_bind.binding);
    expect(bindingValue(w, 'demo', 'other')).toBe('inherit');
    expect(bindingFromValue('demo', 'second', 'proxy:edge')).toEqual(fixture.bind.binding);
    expect(proxyAction('bind', 4).secret).toBeNull();
  });
  it('redacts unexpected secret fields and rejects invalid shapes', () => {
    const input = { ...fixture.workspace, secret: 'sentinel', profiles: [{ ...fixture.workspace.profiles[0], secret: 'sentinel' }] };
    expect(JSON.stringify(checkedWorkspace(input))).not.toContain('sentinel');
    expect(() => checkedWorkspace({ ...input, revision: NaN })).toThrow();
    expect(proxyError(new DesktopProblem('unknown'))).toContain('不要直接重复');
  });
  it('renders provider and key selectors using the same public fixture', () => {
    const c = { data: checkedWorkspace(fixture.workspace), busy: false, error: '', needsRead: false, disabled: false, refresh: async () => {}, save: async () => true };
    const html = renderToStaticMarkup(createElement(ProxyBindings, { c }));
    expect(html).toContain('使用系统代理'); expect(html).toContain('不能保证绕过系统 TUN'); expect(html).toContain('未配置'); expect(html).toContain('继承供应商'); expect(html).toContain('直连'); expect(html).toContain('Example node'); expect(html).not.toContain('password');
  });
});

describe('mihomo listener settings', () => {
  it('accepts automatic and integer ports, rejecting malformed or out of range input', () => {
    expect(listenPortValue('')).toBe(0);
    expect(listenPortValue('0')).toBe(0);
    expect(listenPortValue('17890')).toBe(17890);
    expect(listenPortValue('65535')).toBe(65535);
    for (const value of ['-1', '65536', '1.2', '1e3', 'abc']) expect(listenPortValue(value)).toBeNull();
  });
  it('reads configured and runtime ports while dropping untrusted fields', () => {
    const data = structuredClone(fixture.workspace);
    Object.assign(data.engine.listeners[0], { password: 'runtime-sentinel' });
    const checked = checkedWorkspace(data);
    expect(checked.profiles[0].listen_port).toBe(17890);
    expect(checked.engine.listeners?.[0].port).toBe(17890);
    expect(JSON.stringify(checked)).not.toContain('runtime-sentinel');
    data.engine.listeners[0].port = 65536;
    expect(() => checkedWorkspace(data)).toThrow();
    data.engine.listeners[0].port = 17890;
    data.profiles[0].listen_port = -1;
    expect(() => checkedWorkspace(data)).toThrow();
  });
});
