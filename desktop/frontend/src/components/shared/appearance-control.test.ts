import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { AppearanceControl } from './appearance-control';

for (const labelled of [false, true]) describe(`appearance controls (${labelled ? 'settings' : 'sidebar'})`, () => {
  it('names all choices and exposes one selected preference', () => {
    const html = renderToStaticMarkup(createElement(AppearanceControl, { value: 'system', onChange: () => {}, labelled }));
    expect(html.match(/aria-pressed="true"/g)).toHaveLength(1);
    for (const name of ['浅色', '深色', '跟随系统']) expect(html).toContain(`aria-label="${name}"`);
    expect(html).toContain('type="button"');
  });
});
