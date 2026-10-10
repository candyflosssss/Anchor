import { windowRuntime } from '../../desktop/window-runtime';

export const windowMessages = {
  unavailable: '窗口控制不可用，请使用系统窗口快捷键关闭界面；后台核心不会因此停止。',
  failed: '窗口操作或状态读取失败，请重试；后台核心不受影响。',
  minimise: '最小化',
  maximise: '最大化',
  restore: '还原窗口',
  toggle: '最大化或还原窗口',
  close: '关闭窗口',
  closeHint: '关闭窗口（后台核心继续运行）',
};

export const windowModel = {
  available: windowRuntime.available,
  minimise: windowRuntime.minimise,
  toggleMaximise: windowRuntime.toggleMaximise,
  close: windowRuntime.close,
  async isMaximised(): Promise<boolean> {
    let timer: ReturnType<typeof setTimeout> | undefined;
    try {
      const result = await Promise.race([
        windowRuntime.isMaximised(),
        new Promise<never>((_, reject) => {
          timer = setTimeout(() => reject(new Error('window_state_timeout')), 1500);
        }),
      ]);
      if (typeof result !== 'boolean') throw new Error('window_state_invalid');
      return result;
    } finally {
      clearTimeout(timer);
    }
  },
};
