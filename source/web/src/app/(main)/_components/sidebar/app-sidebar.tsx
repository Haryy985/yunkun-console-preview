"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

import { useShallow } from "zustand/react/shallow";

import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";
import { useCurrentUser } from "@/hooks/use-current-user";
import { findNavItem, navGroupHref, sidebarItems } from "@/navigation/sidebar/sidebar-items";
import { usePreferencesStore } from "@/stores/preferences/preferences-provider";

import { NavMain } from "./nav-main";
import { NavUser } from "./nav-user";

export function AppSidebar({ ...props }: React.ComponentProps<typeof Sidebar>) {
  const currentUser = useCurrentUser();
  const selectedGroup = findNavItem(usePathname())?.group ?? sidebarItems[0];
  const groups =
    currentUser.role === "admin"
      ? [selectedGroup]
      : sidebarItems
          .map((group) => ({
            ...group,
            items: group.items.filter((item) => item.id === "tasks"),
          }))
          .filter((group) => group.items.length > 0);
  const { sidebarVariant, sidebarCollapsible, isSynced } = usePreferencesStore(
    useShallow((s) => ({
      sidebarVariant: s.sidebarVariant,
      sidebarCollapsible: s.sidebarCollapsible,
      isSynced: s.isSynced,
    })),
  );

  const variant = isSynced ? sidebarVariant : props.variant;
  const collapsible = isSynced ? sidebarCollapsible : props.collapsible;

  return (
    <Sidebar {...props} variant={variant} collapsible={collapsible}>
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton asChild>
              <Link
                prefetch={false}
                href={currentUser.role === "admin" ? navGroupHref(selectedGroup) : "/function/tasks"}
                className="h-14! gap-2.5!"
              >
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img
                  src="/yunkun-logo.png"
                  alt="云鲲"
                  width={38}
                  height={38}
                  className="size-9 shrink-0 object-contain"
                />
                <span className="min-w-0 leading-tight group-data-[collapsible=icon]:hidden">
                  <strong className="block truncate text-sm font-semibold">{selectedGroup.label}</strong>
                  <small className="block truncate text-[10px] text-muted-foreground">云鲲控制台</small>
                </span>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent>
        <NavMain items={groups} />
        {/* <NavDocuments items={data.documents} /> */}
        {/* <NavSecondary items={data.navSecondary} className="mt-auto" /> */}
      </SidebarContent>
      <SidebarFooter>
        <NavUser user={currentUser} />
      </SidebarFooter>
    </Sidebar>
  );
}
