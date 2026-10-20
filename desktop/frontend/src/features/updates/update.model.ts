import type { DesktopBridge, UpdateInfo } from '../../desktop/contract.generated';
import { bridge, call, DesktopProblem } from '../../desktop/transport';

export const channels = ['dev', 'alpha', 'beta', 'rc', 'stable'] as const;
export type Channel = typeof channels[number];
export function checkedUpdate(value: UpdateInfo): UpdateInfo {
  if (!value || typeof value.current_version !== 'string' || !value.current_version ||
    typeof value.latest_version !== 'string' || !channels.includes(value.channel as Channel) ||
    typeof value.source !== 'string' || typeof value.available !== 'boolean' ||
    !['idle', 'checked', 'downloaded', 'installing', 'applied'].includes(value.phase) ||
    typeof value.download_path !== 'string' || !['', 'nsis', 'deb', 'runtime', 'binary'].includes(value.install_kind) ||
    (value.available && (!value.latest_version || !value.install_kind)) ||
    (['downloaded', 'installing'].includes(value.phase) && (!value.available || !value.download_path))) {
    throw new DesktopProblem('read_error');
  }
  return { current_version: value.current_version, latest_version: value.latest_version,
    channel: value.channel, source: value.source, available: value.available,
    phase: value.phase, download_path: value.download_path, install_kind: value.install_kind };
}
export class UpdateModel {
  constructor(private readonly getBridge: () => DesktopBridge = bridge) {}
  async status() { return checkedUpdate(await call(() => this.getBridge().UpdateStatus())); }
  async check(channel: Channel) { return checkedUpdate(await call(() => this.getBridge().CheckUpdate(channel))); }
  async download(version: string) { return checkedUpdate(await call(() => this.getBridge().DownloadUpdate(version), true)); }
  async remoteCheck(instance: string) { return checkedUpdate(await call(() => this.getBridge().CheckRemoteUpdate(instance))); }
  async remoteInstall(instance: string, version: string) { return checkedUpdate(await call(() => this.getBridge().InstallRemoteUpdate({ instance_id: instance, version, approved: true }), true)); }
  async install(version: string) { return checkedUpdate(await call(() => this.getBridge().InstallUpdate({ version, approved: true }), true)); }
}
export const updates = new UpdateModel();
