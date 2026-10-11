import { Check, CircleHelp } from 'lucide-react';
import { Button } from '../../components/ui/button';
import type { ProblemCode } from './provider.model';

const problemTitle: Record<ProblemCode, string> = {
  unknown: '写入结果未知', conflict: '配置版本冲突', offline: '实例不可用', unauthorized: '管理授权不可用',
  read_error: '读取供应商失败', invalid: '配置未通过校验', referenced: '配置仍被引用',
};
export function ProblemBanner({ code, message, disabled, onReload }: { code: ProblemCode; message: string; disabled: boolean; onReload: () => void }) {
  return <div className="feedback error-feedback" role="alert"><CircleHelp size={17} /><div><strong>{problemTitle[code]}</strong><p>{message}</p></div>
    <Button variant="outline" disabled={disabled} onClick={onReload}>重新读取配置</Button></div>;
}
export function Notice({ text }: { text: string }) {
  if (!text) return null;
  return <div className="feedback success-feedback" role="status"><Check size={16} /><span>{text}</span></div>;
}
