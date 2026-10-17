import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import fixture from '../../../../contract/model-settings.json';
import catalogFixture from '../../../../contract/model-catalog.json';
import { checkedModelCatalog } from '../model-catalog/model-catalog.model';
import { checkedModelSettings } from './model-settings.model';
vi.mock('../../components/ui/dialog', () => ({ Dialog: ({ children }: { children: React.ReactNode }) => children, DialogContent: ({ children }: { children: React.ReactNode }) => children, DialogTitle: ({ children }: { children: React.ReactNode }) => createElement('h2', null, children), DialogDescription: ({ children }: { children: React.ReactNode }) => createElement('p', null, children) }));
const controller = vi.hoisted(() => ({ value: {} }));
vi.mock('./use-model-settings', () => ({ useModelSettings: () => controller.value }));
import { ModelSettingsDialog } from './model-settings.view';
function render(overrides = {}) {
  controller.value = { state: checkedModelSettings(fixture, 'mapped'), retryCount: '3', retryValidation: '', error: '', notice: '', busy: false, needsRead: false, providers: [], models: [], provider: '', model: '', ...overrides };
  return renderToStaticMarkup(createElement(ModelSettingsDialog, { id: 'mapped', active: true, close: vi.fn(), saved: vi.fn() }));
}
describe('mapping model settings form', () => {
  it('shows limits, supported efforts, defaults and candidate review notice', () => {
    const html = render();
    expect(html).toContain('同一渠道重试次数'); expect(html).toContain('0 表示不重试'); expect(html).toContain('max="10"');
    expect(html).toContain('272000'); expect(html).toContain('最大输出'); expect(html).toContain('默认思考等级');
    expect(html).toContain('2 个上游候选'); expect(html).toContain('补充未设置的默认值'); expect(html).toContain('用目录值替换草稿');
    expect(html).toContain('不自动改写请求'); expect(html).toContain('未关联参考来源');
    expect(html).toContain('value="&quot;high&quot;" selected'); expect(html).not.toContain('value="&quot;medium&quot;"');
  });
  it('keeps secondary settings and explanations collapsed without hiding errors', () => {
    const html = render({ error: '保存结果未知' });
    for (const title of ['模型能力', '思考设置', '高级设置', '目录与参考价格', '使用说明']) {
      expect(html).toContain(`<summary>${title}`);
    }
    expect(html).not.toContain('<details open');
    expect(html).toContain('role="alert">保存结果未知');
    expect(html).not.toContain('打开时自动从 models.dev');
  });
  it('moves automatic defaults guidance into the catalog section, not an opening banner', () => {
    const state = checkedModelSettings(fixture, 'mapped'); state.defaults_status = 'matched';
    const html = render({ state, notice: '已自动从 models.dev 补充基础模型默认值；速度变体不套用标准参考价格，点击「保存模型设置」后生效。' });
    expect(html).not.toContain('role="status"');
    expect(html).toContain('已补全 · 待保存');
    expect(html).toMatch(/<details[^>]*model-settings-catalog[^>]*>.*已自动从 models.dev/);
  });
  it('shows explicit zero and blocks invalid retry drafts', () => {
    expect(render({ retryCount: '0' })).toContain('value="0"');
    const html = render({ retryCount: '11', retryValidation: '重试次数须为 0–10 的整数。' });
    expect(html).toContain('aria-invalid="true"'); expect(html).toMatch(/<button[^>]*disabled[^>]*>保存模型设置/);
  });
  it('disables repeated submission of an already saved draft', () => {
    expect(render({ unchanged: true })).toMatch(/<button[^>]*disabled[^>]*>保存模型设置/);
  });
  it('shows saved reference prices including free and unknown values and restores the provider option', () => {
    const state = checkedModelSettings(fixture, 'mapped');
    state.reference = checkedModelCatalog(structuredClone(catalogFixture.catalog));
    state.reference.models[0].pricing!.rates.input = '0'; state.reference.models[0].pricing!.rates.cache_read = null;
    const html = render({ state, provider: 'lab', model: 'vision-reasoner' });
    expect(html).toContain('models.dev / lab / vision-reasoner');
    expect(html).toContain('<dt>输入</dt><dd>0</dd>'); expect(html).toContain('<dt>缓存读取</dt><dd>未知</dd>');
    expect(html).toContain('USD / 百万 tokens'); expect(html).toContain('按单次输入量选择整次请求单价');
    expect(html).toContain('&gt; 250,000');
    expect(html).toContain('value="lab" selected');
  });
  it('locks editing after an uncertain write until the user reads again', () => {
    const html = render({ needsRead: true, error: '保存结果未知' });
    expect(html).toContain('保存结果未知'); expect(html).toContain('重新读取设置'); expect(html).toContain('fieldset disabled');
  });
  it('renders budget thinking without fabricating a default effort', () => {
    const state = checkedModelSettings(fixture, 'mapped');
    state.configuration.supported_reasoning = [{ type: 'budget_tokens', values: [], min: 1024, max: 4096 }]; state.configuration.default_reasoning = null;
    const html = render({ state }); expect(html).toContain('默认思考预算'); expect(html).not.toContain('默认思考等级');
  });
});
