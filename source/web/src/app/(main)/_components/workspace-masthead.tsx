"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { ArrowUpCircle, ExternalLink, RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { APP_CONFIG } from "@/config/app-config";
import { useCurrentUser } from "@/hooks/use-current-user";
import { api } from "@/lib/api";
import { findNavItem, navGroupHref, sidebarItems } from "@/navigation/sidebar/sidebar-items";

import { AccountSwitcher } from "./sidebar/account-switcher";
import { LayoutControls } from "./sidebar/layout-controls";
import { SearchDialog } from "./sidebar/search-dialog";
import { ThemeSwitcher } from "./sidebar/theme-switcher";

export function WorkspaceMasthead() {
  const currentUser = useCurrentUser();
  const selected = findNavItem(usePathname());
  const [version, setVersion] = useState("");
  const [update, setUpdate] = useState<Awaited<ReturnType<typeof api.checkUpdate>> | null>(null);
  const [checking, setChecking] = useState(false);
  const groups = currentUser.role === "admin" ? sidebarItems : sidebarItems.slice(0, 1);

  useEffect(() => {
    api
      .health()
      .then((h) => setVersion((h.version ?? "").replace(/^v(?=\d)/, "")))
      .catch(() => setVersion(""));
  }, []);

  useEffect(() => {
    if (currentUser.role !== "admin") return;
    let active = true;
    const check = () => {
      api.checkUpdate()
        .then((result) => { if (active) setUpdate(result); })
        .catch(() => { if (active) setUpdate({ status: "unavailable", current_version: version }); });
    };
    check();
    const interval = window.setInterval(check, 6 * 60 * 60 * 1000);
    return () => { active = false; window.clearInterval(interval); };
  }, [currentUser.role, version]);

  const refreshUpdate = async () => {
    setChecking(true);
    try {
      setUpdate(await api.checkUpdate(true));
    } catch {
      setUpdate({ status: "unavailable", current_version: version });
    } finally {
      setChecking(false);
    }
  };

  return (
    <header className="yunkun-masthead">
      <Link
        prefetch={false}
        href={currentUser.role === "admin" ? "/dashboard" : "/function/tasks"}
        className="yunkun-masthead-brand"
      >
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img src="/yunkun-logo.png" alt="云鲲" width={34} height={34} className="size-8 shrink-0 object-contain" />
        <span className="min-w-0">
          <strong className="block truncate text-sm font-semibold">{APP_CONFIG.shortName}</strong>
          <span className="block text-[10px] text-muted-foreground">{APP_CONFIG.team}</span>
        </span>
      </Link>
      <nav aria-label="模块分组" className="yunkun-group-navigation">
        {groups.map((group) => (
          <Link
            key={group.id}
            prefetch={false}
            href={currentUser.role === "admin" ? navGroupHref(group) : "/function/tasks"}
            data-active={(selected?.group.id ?? 1) === group.id}
            aria-current={(selected?.group.id ?? 1) === group.id ? "true" : undefined}
          >
            {group.label}
          </Link>
        ))}
      </nav>
      <div className="yunkun-masthead-controls">
        {currentUser.role === "admin" && (
          <Popover>
            <PopoverTrigger asChild>
              <Button
                size="icon" variant="ghost"
                aria-label={update?.status === "available" ? "发现新版本" : "检查更新"}
                title={update?.status === "available" ? "发现新版本" : "检查更新"}
                className="relative"
              >
                <ArrowUpCircle />
                {update?.status === "available" && <span className="absolute right-0.5 top-0.5 size-2 rounded-full bg-emerald-500" />}
              </Button>
            </PopoverTrigger>
            <PopoverContent align="end" className="w-80 space-y-3 text-sm">
              <div className="flex items-center justify-between">
                <strong>版本更新</strong>
                <Button size="icon-xs" variant="ghost" title="重新检查" aria-label="重新检查" disabled={checking} onClick={refreshUpdate}>
                  <RefreshCw className={checking ? "animate-spin" : ""} />
                </Button>
              </div>
              <p className="text-muted-foreground">当前版本：{update?.current_version || version || "未知"}</p>
              {update?.status === "available" ? (
                <>
                  <p>发现 {update.latest_version}。更新前请先暂停任务、停止旧服务并备份本机数据。</p>
                  <p className="text-xs text-muted-foreground">新包必须解压到另一目录；旧目录和 .yunkun-local 不会被自动删除。</p>
                  <p className="break-all font-mono text-[10px] text-muted-foreground" title="下载 ZIP 后校验 SHA-256">
                    SHA-256: {update.package_sha256}
                  </p>
                  <div className="flex flex-wrap gap-2">
                    <Button asChild size="sm"><a href={update.release_url} target="_blank" rel="noopener noreferrer"><ExternalLink />查看版本</a></Button>
                    <Button asChild size="sm" variant="outline"><a href="https://github.com/Haryy985/yunkun-console-preview/blob/main/docs/OFFLINE_UPDATE.md" target="_blank" rel="noopener noreferrer">升级手册</a></Button>
                  </div>
                </>
              ) : (
                <p>{update?.status === "current" ? "已是最新离线版本。" : update?.status === "unsupported" ? "当前为开发版或非离线版本，无法自动比较。" : "暂时无法检查更新；请确认网络连接，或手动查看发布页。"}</p>
              )}
            </PopoverContent>
          </Popover>
        )}
        <span className="hidden max-w-36 truncate text-xs text-muted-foreground xl:block" title={version}>
          {version}
        </span>
        <SearchDialog />
        <LayoutControls />
        <ThemeSwitcher />
        <AccountSwitcher users={[currentUser]} />
      </div>
    </header>
  );
}
