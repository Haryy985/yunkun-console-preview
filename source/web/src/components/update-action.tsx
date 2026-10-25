"use client";

import * as React from "react";

import { DownloadIcon, LoaderCircleIcon, RefreshCwIcon } from "lucide-react";
import { toast } from "sonner";

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
import type { UpdateCheck, UpdateProgress } from "@/lib/types";

const phaseNames: Record<string, string> = {
  downloading: "下载更新包",
  verifying: "校验文件",
  extracting: "检查安装包",
  staged: "等待服务重启",
  waiting: "等待服务停止",
  backing_up: "备份数据库和文件",
  installing: "安装新版",
  starting: "检查新版并重启",
  rolling_back: "恢复旧版及数据",
  completed: "更新完成",
  rolled_back: "更新失败，已恢复旧版和数据",
  failed: "更新未完成",
  recovery_required: "自动恢复未完成，请保留备份并检查日志",
};
const terminal = new Set(["idle", "completed", "rolled_back", "failed", "recovery_required"]);
const pendingKey = "yunkun-update-in-progress";

export function UpdateAction({ info }: { info: UpdateCheck | null }) {
  const [confirm, setConfirm] = React.useState(false);
  const [submitting, setSubmitting] = React.useState(false);
  const [progress, setProgress] = React.useState<UpdateProgress | null>(null);
  const [reconnecting, setReconnecting] = React.useState(false);
  const submitted = React.useRef(false);

  React.useEffect(() => {
    let alive = true;
    let timer: ReturnType<typeof setTimeout>;
    submitted.current = sessionStorage.getItem(pendingKey) !== null;
    const poll = async () => {
      try {
        const state = await api.updateStatus();
        if (!alive) return;
        setProgress(state);
        setReconnecting(false);
        if (terminal.has(state.phase) && state.phase !== "idle") {
          submitted.current = false;
          sessionStorage.removeItem(pendingKey);
        }
      } catch {
        if (alive && submitted.current) setReconnecting(true);
      } finally {
        if (alive) timer = setTimeout(poll, 2000);
      }
    };
    void poll();
    return () => {
      alive = false;
      clearTimeout(timer);
    };
  }, []);

  const busy = submitting || reconnecting || !!(progress && !terminal.has(progress.phase));
  const begin = async () => {
    if (!info?.latest) return;
    setSubmitting(true);
    try {
      await api.applyUpdate(info.latest);
      submitted.current = true;
      sessionStorage.setItem(pendingKey, info.latest);
      setProgress({ phase: "downloading", percent: 0, version: info.latest });
      setConfirm(false);
    } catch (error) {
      toast.error((error as Error).message);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="space-y-3">
      {info?.has_update && (
        <Button
          onClick={() => setConfirm(true)}
          disabled={busy || !info.auto_update_supported || info.asset_available === false}
        >
          {busy ? <LoaderCircleIcon className="size-4 animate-spin" /> : <DownloadIcon className="size-4" />}
          {busy ? "更新处理中" : "一键更新"}
        </Button>
      )}
      {info?.has_update && !info.auto_update_supported && info.auto_update_reason && (
        <p className="text-xs text-muted-foreground">{info.auto_update_reason}</p>
      )}
      {progress && progress.phase !== "idle" && (
        <div className="space-y-2 border-l-2 border-primary pl-3 text-sm" aria-live="polite" role="status">
          <p>{reconnecting ? "服务重启中，正在重新连接…" : phaseNames[progress.phase] || progress.message}</p>
          {progress.phase === "downloading" && typeof progress.percent === "number" && progress.percent >= 0 && (
            <div className="flex items-center gap-3">
              <progress className="h-2 min-w-0 flex-1" max={100} value={progress.percent} aria-label="下载进度" />
              <span className="w-10 text-right text-xs tabular-nums">{progress.percent}%</span>
            </div>
          )}
          {progress.error && <p className="break-words text-xs text-destructive">{progress.error}</p>}
          {progress.backup && <p className="break-all text-xs text-muted-foreground">备份位置：{progress.backup}</p>}
          {(progress.phase === "completed" || progress.phase === "rolled_back") && (
            <Button size="sm" variant="outline" onClick={() => window.location.reload()}>
              <RefreshCwIcon className="size-4" />
              重新载入控制台
            </Button>
          )}
        </div>
      )}
      {reconnecting && !progress && (
        <p role="status" className="text-sm">
          服务重启中，正在重新连接…
        </p>
      )}
      <AlertDialog open={confirm} onOpenChange={setConfirm}>
        <AlertDialogContent className="max-w-md">
          <AlertDialogHeader>
            <AlertDialogTitle>更新到 {info?.latest}</AlertDialogTitle>
            <AlertDialogDescription>
              服务将短暂中断。系统会先校验下载包，再备份数据库、配置、密钥和用户文件，随后安装并重启。
              启动检查失败时自动恢复更新前的程序和数据。运行中的任务和对话须先暂停或结束。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={submitting}>取消</AlertDialogCancel>
            <Button onClick={() => void begin()} disabled={submitting}>
              {submitting && <LoaderCircleIcon className="size-4 animate-spin" />}
              备份并更新
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
