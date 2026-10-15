import {
  Activity,
  Ban,
  BellRing,
  BookOpen,
  Bot,
  Brain,
  Bug,
  ClipboardList,
  FolderOpen,
  FolderSync,
  LayoutDashboard,
  type LucideIcon,
  MessageSquare,
  Network,
  Plug,
  Radio,
  ScrollText,
  Settings2,
  ShieldAlert,
  Sparkles,
  Target,
  Terminal,
  Wrench,
} from "lucide-react";

export type NavBadge = "new" | "soon";

export interface NavSubItem {
  id: string;
  title: string;
  url: string;
  icon?: LucideIcon;
  badge?: NavBadge;
  disabled?: boolean;
  newTab?: boolean;
}

interface NavItemBase {
  id: string;
  title: string;
  icon?: LucideIcon;
  badge?: NavBadge;
  disabled?: boolean;
  newTab?: boolean;
}

export interface NavMainLinkItem extends NavItemBase {
  url: string;
  subItems?: never;
}

export interface NavMainParentItem extends NavItemBase {
  subItems: NavSubItem[];
}

export type NavMainItem = NavMainLinkItem | NavMainParentItem;

export interface NavGroup {
  id: number;
  label?: string;
  items: NavMainItem[];
}

export const sidebarItems: NavGroup[] = [
  {
    id: 1,
    label: "工作台",
    items: [
      { id: "dashboard", title: "仪表盘", url: "/dashboard", icon: LayoutDashboard },
      { id: "chat", title: "对话", url: "/chat", icon: MessageSquare },
      { id: "tasks", title: "任务", url: "/function/tasks", icon: Target },
      { id: "findings", title: "漏洞发现", url: "/function/findings", icon: Bug },
    ],
  },
  {
    id: 2,
    label: "执行记录",
    items: [
      { id: "traffic", title: "流量", url: "/function/traffic", icon: Activity },
      { id: "commands", title: "工具执行", url: "/function/commands", icon: Terminal },
      { id: "llm-records", title: "LLM 录制", url: "/function/llm-records", icon: Radio },
    ],
  },
  {
    id: 3,
    label: "资产空间",
    items: [
      { id: "assets", title: "资产", url: "/function/assets", icon: Network },
      { id: "sync", title: "资产同步", url: "/function/sync", icon: FolderSync },
      { id: "workspace", title: "工作空间", url: "/function/workspace", icon: FolderOpen },
    ],
  },
  {
    id: 4,
    label: "资源配置",
    items: [
      { id: "llm", title: "LLM", url: "/system/llm", icon: Brain },
      { id: "agents", title: "Agent", url: "/system/agents", icon: Bot },
      { id: "rag", title: "RAG 知识库", url: "/system/rag", icon: BookOpen },
      { id: "mcp", title: "MCP", url: "/system/mcp", icon: Plug },
      { id: "proxy-pools", title: "代理池", url: "/system/proxy-pools", icon: Network },
      { id: "skills", title: "Skill", url: "/system/skills", icon: Sparkles },
      { id: "tools", title: "工具", url: "/system/tools", icon: Wrench },
    ],
  },
  {
    id: 5,
    label: "系统管理",
    items: [
      { id: "notify", title: "通知推送", url: "/system/notify", icon: BellRing },
      { id: "intercept", title: "拦截规则", url: "/system/intercept", icon: ShieldAlert },
      { id: "asset-intercept", title: "资产拦截", url: "/system/intercept/assets", icon: Ban },
      { id: "approvals", title: "审批记录", url: "/system/intercept/approvals", icon: ClipboardList },
      { id: "logs", title: "日志", url: "/system/logs", icon: ScrollText },
      { id: "settings", title: "系统配置", url: "/system/settings", icon: Settings2 },
    ],
  },
];

export function normalizeNavPath(pathname: string): string {
  let path = pathname;
  try {
    path = decodeURIComponent(path);
  } catch {
    // Keep the original path when a malformed escape cannot be decoded.
  }
  return path.replace(/\/+$/, "") || "/";
}

export function navGroupHref(group: NavGroup): string {
  const first = group.items[0];
  if (!first) return "/dashboard";
  return "url" in first ? first.url : (first.subItems[0]?.url ?? "/dashboard");
}

// The longest route wins, so an asset approval page cannot also select its parent.
export function findNavItem(pathname: string) {
  const path = normalizeNavPath(pathname);
  return sidebarItems
    .flatMap((group) =>
      group.items.flatMap((item) =>
        item.subItems ? item.subItems.map((sub) => ({ group, item: sub })) : [{ group, item }],
      ),
    )
    .filter(({ item }) => path === item.url || path.startsWith(`${item.url}/`))
    .sort((a, b) => b.item.url.length - a.item.url.length)[0];
}
