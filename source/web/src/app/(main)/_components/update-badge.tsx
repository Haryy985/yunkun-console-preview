"use client";

import * as React from "react";

import { useRouter } from "next/navigation";

import { ArrowUpCircleIcon } from "lucide-react";

import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { api } from "@/lib/api";

const RELEASES_URL = "https://github.com/Haryy985/yunkun-console-preview/releases";

export function UpdateBadge({ showTrigger = true }: { showTrigger?: boolean }) {
  const router = useRouter();
  const [info, setInfo] = React.useState<Awaited<ReturnType<typeof api.checkUpdate>> | null>(null);
  const [open, setOpen] = React.useState(false);

  React.useEffect(() => {
    let alive = true;
    const check = () => {
      api
        .checkUpdate()
        .then((result) => {
          if (!alive || !result.has_update || !result.latest) return;
          setInfo(result);
          if (window.localStorage.getItem("yunkun-update-dismissed") !== result.latest) {
            setOpen(true);
          }
        })
        .catch(() => {
          // Offline installations remain usable when GitHub cannot be reached.
        });
    };
    check();
    const timer = window.setInterval(check, 6 * 60 * 60 * 1000);
    return () => {
      alive = false;
      window.clearInterval(timer);
    };
  }, []);

  if (!info?.latest) return null;

  const dismiss = () => {
    window.localStorage.setItem("yunkun-update-dismissed", info.latest);
    setOpen(false);
  };

  const openReleases = () => {
    window.open(RELEASES_URL, "_blank", "noopener,noreferrer");
    dismiss();
  };

  return (
    <>
      {showTrigger ? (
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="h-8 gap-1.5 border-primary/40 text-primary"
          title={`发现新版本 ${info.latest}`}
          onClick={() => setOpen(true)}
        >
          <ArrowUpCircleIcon className="size-4" />
          <span className="hidden sm:inline">新版本 {info.latest}</span>
        </Button>
      ) : null}
      <AlertDialog open={open} onOpenChange={(next) => (next ? setOpen(true) : dismiss())}>
        <AlertDialogContent size="sm" className="max-h-[80vh] max-w-lg overflow-y-auto">
          <AlertDialogHeader>
            <AlertDialogTitle>发现云鲲控制台新版本</AlertDialogTitle>
            <AlertDialogDescription>
              当前版本 {info.current}，最新版本 {info.latest}。
            </AlertDialogDescription>
          </AlertDialogHeader>
          {info.notes ? (
            <div className="max-h-56 overflow-y-auto whitespace-pre-wrap rounded-md border bg-muted/30 p-3 text-xs leading-5">
              {info.notes.slice(0, 6000)}
            </div>
          ) : null}
          <p className="text-muted-foreground text-xs">更新前保留数据备份；可在版本与更新页面查看安装进度。</p>
          <AlertDialogFooter>
            <AlertDialogCancel onClick={dismiss}>稍后</AlertDialogCancel>
            <Button variant="outline" onClick={openReleases}>
              发布说明
            </Button>
            <Button
              onClick={() => {
                dismiss();
                router.push("/system/settings/");
              }}
            >
              前往更新
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
