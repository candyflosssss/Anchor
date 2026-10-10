// Layout adapted from official shadcn sidebar-07 (MIT); see SHADCN_SOURCES.md.
import { useMemo, useState } from 'react';
import { createEnvironment, prototype } from '../desktop/environment';
import { desktopMessages, useDesktopSession } from '../desktop/use-desktop-session';
import { Button } from '../components/ui/button';
import { ConfirmAction } from '../components/shared/confirm-action';
import { AppSidebar } from '../components/shared/app-sidebar';
import { Breadcrumb, BreadcrumbItem, BreadcrumbList, BreadcrumbPage, BreadcrumbSeparator } from '../components/ui/breadcrumb';
import { Separator } from '../components/ui/separator';
import { SidebarInset, SidebarProvider, SidebarTrigger } from '../components/ui/sidebar';
import { ProvidersPage } from '../pages/providers-page';
import { useAppearance } from './use-appearance';
import { useWindowController } from '../features/window/use-window-controller';
import { WindowControls } from '../features/window/window-controls.view';
import { cn } from '../lib/utils';

export function App() {
  const { appearance, setAppearance } = useAppearance();
  const environment = useMemo(createEnvironment, []);
  const session = useDesktopSession();
  const windowController = useWindowController(!prototype);
  const [busy, setBusy] = useState(false);
  const [confirmStop, setConfirmStop] = useState(false);
  return <SidebarProvider>
    <AppSidebar appearance={appearance} setAppearance={setAppearance} />
    <SidebarInset className="min-w-0">
      <header onDoubleClick={windowController.onDragDoubleClick} className={cn("flex h-16 shrink-0 items-center justify-between gap-2 border-b px-4 transition-[width,height] ease-linear group-has-[[data-collapsible=icon]]/sidebar-wrapper:h-12", !prototype && "desktop-titlebar sticky top-0 z-20 bg-background pr-0 md:rounded-t-xl")}>
        <div className="flex min-w-0 items-center gap-2">
          <SidebarTrigger className="-ml-1 h-8 w-8" />
          <Separator orientation="vertical" className="mr-2 h-4" />
          <Breadcrumb><BreadcrumbList>
            <BreadcrumbItem className="hidden sm:block">工作台</BreadcrumbItem>
            <BreadcrumbSeparator className="hidden sm:block" />
            <BreadcrumbItem><BreadcrumbPage>供应商</BreadcrumbPage></BreadcrumbItem>
          </BreadcrumbList></Breadcrumb>
        </div>
        <div className="flex h-full min-w-0 items-center gap-2">
          <span className="connection-label" role={prototype ? undefined : 'status'}><span className="mini-dot" />{prototype ? '本机实例 · 模拟，未连接' : session.phase === 'ready' ? '本机实例 · 已连接' : session.phase === 'starting' ? '本机实例 · 启动中' : '本机实例 · 未连接'}</span>
          {!prototype && <WindowControls controller={windowController} />}
        </div>
      </header>
      {windowController.error && <p role="status" className="border-b px-4 py-2 text-xs text-destructive">{windowController.error}</p>}
      {!prototype && <div className="flex flex-wrap items-center justify-between gap-3 border-b px-6 py-3 text-xs text-muted-foreground">
        <span>{session.info ? `${session.info.source === 'started' ? '本次启动' : '复用已有核心'} · ` : ''}关闭窗口不会停止核心 · 不自动设置开机自启动</span>
        {session.info?.can_stop && <Button variant="outline" size="sm" disabled={busy || session.stopping} onClick={() => setConfirmStop(true)}>停止核心</Button>}
      </div>}
      {session.phase === 'ready' ? <ProvidersPage environment={environment} onActivity={setBusy} /> : <section className="m-6 rounded-lg border p-6" aria-live="polite">
        <h1 className="text-xl font-semibold">{session.phase === 'starting' ? '正在连接本机核心…' : session.phase === 'stopped' ? '停服请求已接受' : '本机核心暂不可用'}</h1>
        <p className="my-4 text-sm text-muted-foreground">{session.error ? desktopMessages[session.error] : session.phase === 'stopped' ? '核心正在处理退出与配置恢复；请求接受不代表恢复已完成。' : '自动初始化或核验已有实例，不会切换到模拟数据。'}</p>
        {session.phase !== 'starting' && <Button onClick={() => void session.connect()}>重新连接本机核心</Button>}
      </section>}
      <ConfirmAction open={confirmStop} title="停止本机核心？" busy={session.stopping} onCancel={() => setConfirmStop(false)} onConfirm={() => { void session.stop().finally(() => setConfirmStop(false)); }}>
        <p className="break-all">实例：{session.info?.instance_id}</p><p>连接来源：{session.info?.source === 'started' ? '本次由桌面宿主启动' : '核验后复用的已有核心'}。</p><p>这会停止代理服务并按既有规则恢复托管配置，可能影响使用该实例的其他程序。仅关闭窗口不需要停止核心。</p>
      </ConfirmAction>
    </SidebarInset>
  </SidebarProvider>;
}
