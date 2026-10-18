"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";

import { APP_CONFIG } from "@/config/app-config";
import { useCurrentUser } from "@/hooks/use-current-user";
import { api } from "@/lib/api";
import { findNavItem, navGroupHref, sidebarItems } from "@/navigation/sidebar/sidebar-items";

import { AccountSwitcher } from "./sidebar/account-switcher";
import { LayoutControls } from "./sidebar/layout-controls";
import { SearchDialog } from "./sidebar/search-dialog";
import { ThemeSwitcher } from "./sidebar/theme-switcher";
import { UpdateBadge } from "./update-badge";

export function WorkspaceMasthead() {
  const currentUser = useCurrentUser();
  const selected = findNavItem(usePathname());
  const [version, setVersion] = useState("");
  const groups = currentUser.role === "admin" ? sidebarItems : sidebarItems.slice(0, 1);

  useEffect(() => {
    api
      .health()
      .then((h) => setVersion((h.version ?? "").replace(/^v(?=\d)/, "")))
      .catch(() => setVersion(""));
  }, []);

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
        {currentUser.role === "admin" ? <UpdateBadge /> : null}
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
