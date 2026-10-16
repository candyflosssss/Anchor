// Adapted from official sidebar-07 (MIT); see SHADCN_SOURCES.md.
import { Anchor, ArrowLeftRight, Bot, FileText, Library, LayoutDashboard, MessageSquare, PlugZap, Settings2, Server } from 'lucide-react';
import { Sidebar, SidebarContent, SidebarFooter, SidebarGroup, SidebarGroupContent, SidebarGroupLabel, SidebarHeader, SidebarMenu, SidebarMenuButton, SidebarMenuItem, SidebarRail, useSidebar } from '../ui/sidebar';
import { prototype } from '../../desktop/environment';
import { AppearanceControl } from './appearance-control';
import type { Appearance } from '../../app/use-appearance';

export type Page = 'overview' | 'providers' | 'models' | 'logs' | 'agents' | 'settings' | 'connections' | 'chat' | 'skills';

export function AppSidebar({ appearance, setAppearance, page, onNavigate, busy }: { page: Page; onNavigate: (page: Page) => void; busy: boolean; appearance: Appearance; setAppearance: (value: Appearance) => void }) {
  const { isMobile, setOpenMobile } = useSidebar();
  return <Sidebar collapsible="icon" variant="inset" aria-label="工作台导航">
    <SidebarHeader><SidebarMenu><SidebarMenuItem>
      <SidebarMenuButton asChild size="lg" className="pointer-events-none"><div aria-label="Anchor 模型中转工作台">
        <span className="flex aspect-square size-8 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground"><Anchor className="size-4" /></span>
        <span className="grid flex-1 text-left text-sm leading-tight"><span className="font-semibold">Anchor</span><span className="text-xs text-muted-foreground">模型中转工作台</span></span>
      </div></SidebarMenuButton>
    </SidebarMenuItem></SidebarMenu></SidebarHeader>
    <SidebarContent><SidebarGroup><SidebarGroupLabel>工作空间</SidebarGroupLabel><SidebarGroupContent><SidebarMenu>
      {([
        [Server, '设备连接', 'connections'], [LayoutDashboard, '概览', 'overview'], [PlugZap, '供应商', 'providers'], [ArrowLeftRight, '模型映射', 'models'], [MessageSquare, '对话测试', 'chat'], [Bot, 'Agent 托管', 'agents'], [Library, 'Skills', 'skills'], [FileText, '日志', 'logs'],
      ] as const).map(([Icon, title, target]) => <SidebarMenuItem key={title}>
        <SidebarMenuButton tooltip={target ? title : `${title}：不在本版范围`} disabled={!target || busy} isActive={!!target && page === target} aria-current={target && page === target ? 'page' : undefined} aria-label={title} onClick={() => { if (!target) return; onNavigate(target); if (isMobile) setOpenMobile(false); }}>
          <Icon /><span>{title}</span>{!target && <span className="ml-auto text-xs text-muted-foreground">后续</span>}
        </SidebarMenuButton>
      </SidebarMenuItem>)}
    </SidebarMenu></SidebarGroupContent></SidebarGroup></SidebarContent>
    <SidebarFooter>
      <SidebarMenu><SidebarMenuItem><SidebarMenuButton tooltip="设置 / 在线更新" aria-label="设置 / 在线更新" disabled={busy} isActive={page === 'settings'} aria-current={page === 'settings' ? 'page' : undefined} onClick={() => { onNavigate('settings'); if (isMobile) setOpenMobile(false); }}><Settings2 /><span>设置</span></SidebarMenuButton></SidebarMenuItem></SidebarMenu>
      <AppearanceControl value={appearance} onChange={setAppearance} />
      {prototype && <div className="px-2 py-2 text-xs text-muted-foreground group-data-[collapsible=icon]:hidden">Mock 设计预览</div>}
    </SidebarFooter><SidebarRail />
  </Sidebar>;
}
