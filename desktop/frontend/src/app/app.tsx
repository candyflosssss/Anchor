import { SkillsView } from '../features/skills/skills.view';
// Layout adapted from official shadcn sidebar-07 (MIT); see SHADCN_SOURCES.md.
import { useCallback, useMemo, useState } from 'react';
import { createEnvironment, prototype } from '../desktop/environment';
import { desktopMessages, useDesktopSession } from '../desktop/use-desktop-session';
import { Button } from '../components/ui/button';
import { ConfirmAction } from '../components/shared/confirm-action';
import { AppSidebar, type Page } from '../components/shared/app-sidebar';
import { Breadcrumb, BreadcrumbItem, BreadcrumbList, BreadcrumbPage, BreadcrumbSeparator } from '../components/ui/breadcrumb';
import { Separator } from '../components/ui/separator';
import { SidebarInset, SidebarProvider, SidebarTrigger } from '../components/ui/sidebar';
import { ConnectionsView } from '../features/connections/connections.view';
import { AgentsView } from '../features/agents/agents.view';
import { SettingsPage } from '../pages/settings-page';
import { LogsPreview } from '../features/logs/log-workspace.view';
import { ChatWorkspace } from '../features/chat/chat-workspace.view';
import { WorkbenchPage } from '../pages/workbench-page';
import { CoreStatus } from './core-status.view';
import { useAppearance } from './use-appearance';
import { useWindowController } from '../features/window/use-window-controller';
import { WindowControls } from '../features/window/window-controls.view';
import { cn } from '../lib/utils';

const pageTitle: Record<Page, string> = { connections: '设备连接', overview: '概览', providers: '供应商', models: '模型映射', logs: '日志', chat: '对话测试', agents: 'Agent 托管', settings: '设置', skills: 'Skills' };

export function App() {
  const { appearance, setAppearance } = useAppearance();
  const environment = useMemo(createEnvironment, []);
  const session = useDesktopSession();
  const instanceId = session.info?.instance_id ?? (prototype ? 'prototype' : '');
  const windowController = useWindowController(!prototype);
  const [page, setPage] = useState<Page>('providers');
  const [navSeq, setNavSeq] = useState(0);
  // Sidebar clicks bump navSeq so the workbench returns to the list; in-page jumps (overview → detail) only switch page.
  const navigate = useCallback((next: Page) => { setPage(next); setNavSeq(n => n + 1); }, []);
  const [busy, setBusy] = useState(false);
  const [confirmStop, setConfirmStop] = useState(false);
  return <SidebarProvider>
    <AppSidebar appearance={appearance} setAppearance={setAppearance} page={page} onNavigate={navigate} busy={busy} />
    <SidebarInset className="min-w-0">
      <header onDoubleClick={windowController.onDragDoubleClick} className={cn("app-titlebar sticky top-0 z-20 flex h-10 shrink-0 items-center justify-between gap-2 border-b bg-background px-4 md:rounded-t-xl", !prototype && "desktop-titlebar pr-0")}>
        <div className="flex min-w-0 items-center gap-2">
          <SidebarTrigger className="-ml-1 h-8 w-8" />
          <Separator orientation="vertical" className="mr-2 h-4" />
          <Breadcrumb><BreadcrumbList className="flex-nowrap">
            <BreadcrumbItem className="hidden sm:block">工作台</BreadcrumbItem>
            <BreadcrumbSeparator className="hidden sm:block" />
            <BreadcrumbItem><BreadcrumbPage className="truncate">{pageTitle[page]}</BreadcrumbPage></BreadcrumbItem>
          </BreadcrumbList></Breadcrumb>
        </div>
        <div className="flex h-full min-w-0 items-center gap-2">
          <CoreStatus session={session} busy={busy} onActivity={setBusy} onManage={() => navigate('connections')} onExit={windowController.exit} onStop={() => setConfirmStop(true)} />
          {!prototype && <WindowControls controller={windowController} />}
        </div>
      </header>
      {windowController.error && <p role="status" className="border-b px-4 py-2 text-xs text-destructive">{windowController.error}</p>}
      {page === 'connections' ? <ConnectionsView currentProfile={session.info?.profile_name} disabled={busy || session.stopping || session.phase === 'starting'} onConnected={session.select} onActivity={setBusy} /> : page === 'logs' ? <LogsPreview key={session.info?.instance_id ?? session.phase} /> : page === 'chat' && (prototype || session.phase === 'ready') ? <ChatWorkspace key={session.info?.instance_id ?? session.phase} environment={environment} /> : page === 'agents' && (prototype || session.phase === 'ready') ? <AgentsView key={session.info?.instance_id ?? session.phase} onActivity={setBusy} /> : page === 'skills' && (prototype || session.phase === 'ready') ? <SkillsView key={session.info?.instance_id ?? session.phase} onActivity={setBusy} /> : page === 'settings' ? <SettingsPage appearance={appearance} setAppearance={setAppearance} onActivity={setBusy} /> : session.phase === 'ready' ? <WorkbenchPage key={instanceId} environment={environment} page={page === 'agents' || page === 'chat' || page === 'skills' ? 'overview' : page} navSeq={navSeq} onNavigate={setPage} onActivity={setBusy} /> : <section className="m-6 rounded-lg border p-6" aria-live="polite">
        <h1 className="text-xl font-semibold">{session.phase === 'starting' ? '正在连接核心…' : session.phase === 'stopped' ? '停服请求已接受' : '当前设备暂不可用'}</h1>
        {(session.error || session.phase === 'stopped') && <p className="my-4 text-sm text-muted-foreground">{session.error ? desktopMessages[session.error] : '核心正在处理退出与配置恢复。'}</p>}
        {session.phase !== 'starting' && <Button onClick={() => void session.connect()}>重新连接当前设备</Button>}
      </section>}
      <ConfirmAction open={confirmStop} title={session.info?.transport === 'ssh' ? `停止远程设备 ${session.info.target} 的核心？` : '停止本机核心？'} busy={session.stopping} onCancel={() => setConfirmStop(false)} onConfirm={() => { void session.stop().finally(() => setConfirmStop(false)); }}>
        <p className="break-all">实例：{session.info?.instance_id}</p><p>连接来源：{session.info?.transport === 'ssh' ? `远程设备 ${session.info.target}` : session.info?.source === 'started' ? '本次由桌面宿主启动' : '核验后复用的已有核心'}。</p><p>这会停止代理服务并按既有规则恢复托管配置，可能影响使用该实例的其他程序。仅关闭窗口不需要停止核心。</p>
      </ConfirmAction>
    </SidebarInset>
  </SidebarProvider>;
}
