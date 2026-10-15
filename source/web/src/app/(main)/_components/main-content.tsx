"use client";

import type { ReactNode } from "react";

import { usePathname } from "next/navigation";

import { SidebarTrigger } from "@/components/ui/sidebar";
import { findNavItem } from "@/navigation/sidebar/sidebar-items";

// 任务详情页保持原样：它自带头部/Tabs 与内边距，这里不再叠加全局头部和 padding。
function isFullBleed(pathname: string) {
  const p = (() => {
    try {
      return decodeURIComponent(pathname);
    } catch {
      return pathname;
    }
  })();
  // 静态导出开了 trailingSlash，列表页自身的 pathname 是 "/function/tasks/"，
  // 先去掉尾斜杠再判前缀，否则列表页会被误判成详情页而丢掉全局头部。
  const normalized = p.replace(/\/+$/, "");
  return normalized.startsWith("/function/tasks/");
}

export function MainContent({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const selected = findNavItem(pathname);

  if (isFullBleed(pathname)) {
    return <>{children}</>;
  }

  return (
    <>
      <header className="flex h-12 shrink-0 items-center gap-3 border-b px-4 md:px-6">
        <SidebarTrigger className="-ml-1" />
        <div className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
          <span>{selected?.group.label ?? "工作台"}</span>
          <span aria-hidden="true">/</span>
          <span className="truncate text-foreground">{selected?.item.title ?? "云鲲控制台"}</span>
        </div>
      </header>
      <div className="min-h-0 min-w-0 flex-1 overflow-x-hidden p-4 has-data-[content-padding=false]:p-0 md:p-6 md:has-data-[content-padding=false]:p-0">
        {children}
      </div>
    </>
  );
}
