"use client";

import * as React from "react";
import { FileTextIcon, RefreshCwIcon } from "lucide-react";
import { api } from "@/lib/api";
import type { TaskNetwork, ProxyEvent } from "@/lib/proxy-pool-types";
import { Button } from "@/components/ui/button";
import { useCurrentUser } from "@/hooks/use-current-user";
import { toast } from "sonner";
import { CapturedTrafficViewer } from "@/components/traffic-evidence-viewer";

export function ProxyEventsTable({ events }: { events: ProxyEvent[] }) {
  const [trafficID, setTrafficID] = React.useState<string | null>(null);
  return (
    <div className="overflow-x-auto">
      <table className="w-full min-w-[640px] text-left text-sm">
        <thead className="border-b text-muted-foreground">
          <tr>
            <th className="p-3">时间</th>
            <th className="p-3">任务</th>
            <th className="p-3">节点</th>
            <th className="p-3">目标</th>
            <th className="p-3">结果</th>
            <th className="p-3">请求包</th>
          </tr>
        </thead>
        <tbody>
          {events.map((e) => (
            <tr key={e.id} className="border-b">
              <td className="whitespace-nowrap p-3">{new Date(e.created_at).toLocaleString()}</td>
              <td className="p-3">{e.task_id ?? "-"}</td>
              <td className="p-3">{e.node_id ?? "-"}</td>
              <td className="max-w-80 break-all p-3 font-mono text-xs">{e.target}</td>
              <td className="p-3">{e.result}</td>
              <td className="p-3">
                {e.traffic_id ? (
                  <Button variant="ghost" size="icon" title="查看请求与响应" aria-label="查看请求与响应"
                    onClick={() => setTrafficID(e.traffic_id!)}><FileTextIcon className="size-4" /></Button>
                ) : "-"}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {events.length === 0 && <p className="p-8 text-center text-muted-foreground text-sm">暂无出口记录</p>}
      <CapturedTrafficViewer id={trafficID} onClose={() => setTrafficID(null)} />
    </div>
  );
}

export function TaskNetworkPanel({ taskID, canReassign = false }: { taskID: string; canReassign?: boolean }) {
  const user = useCurrentUser();
  const [data, setData] = React.useState<TaskNetwork | null>(null);
  const [error, setError] = React.useState("");
  const [loading, setLoading] = React.useState(false);
  const load = React.useCallback(async () => {
    setLoading(true);
    try {
      setData(await api.taskNetwork(taskID));
      setError("");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }, [taskID]);
  React.useEffect(() => {
    void load();
  }, [load]);
  return (
    <section className="space-y-5">
      <div className="flex items-center justify-between gap-3">
        <h2 className="font-medium">任务出口</h2>
        <Button
          variant="outline"
          size="icon"
          aria-label="刷新任务出口"
          title="刷新任务出口"
          disabled={loading}
          onClick={load}
        >
          <RefreshCwIcon className="size-4" />
        </Button>
      </div>
      {error && (
        <p role="alert" className="text-destructive text-sm">
          {error}
        </p>
      )}
      {!data && loading && <p className="text-muted-foreground text-sm">加载中</p>}
      {data && !data.bound && <p className="text-muted-foreground text-sm">跟随全局出口配置，未绑定任务代理池。</p>}
      {data?.bound && (
        <>
          <dl className="grid gap-4 border-y py-4 text-sm sm:grid-cols-2 xl:grid-cols-4">
            <div>
              <dt className="text-muted-foreground">代理来源</dt>
              <dd className="mt-1 break-all">{data.pool_name}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">模式</dt>
              <dd className="mt-1">配置模式 · 未强制隔离</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">节点</dt>
              <dd className="mt-1 break-all">{data.node?.address ?? "-"}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">最近实测出口</dt>
              <dd className="mt-1">{data.node?.verified_ip || "未验证"}</dd>
            </div>
          </dl>
          {data.reason && (
            <p role="alert" className="text-destructive text-sm">
              {data.reason}
            </p>
          )}
          {canReassign && (user.role === "admin" || user.role === "operator") && (
            <Button
              variant="outline"
              disabled={loading}
              onClick={async () => {
                setLoading(true);
                try {
                  await api.reassignTaskProxyLease(taskID);
                  toast.success("代理租约已重新分配");
                  await load();
                } catch (e) {
                  toast.error((e as Error).message);
                } finally {
                  setLoading(false);
                }
              }}
            >
              重新分配租约
            </Button>
          )}
          <p className="text-amber-700 text-sm dark:text-amber-400">
            宿主脚本、交互终端、浏览器 / MCP、自定义网络工具及资产补全尚未完成强制出口验收。
          </p>
          <p className="text-muted-foreground text-sm">
            {data.capture === "http_and_webfetch_complete_bodies"
              ? "录流已开启：HTTP 转发与 WebFetch 可保存完整请求和响应，每个正文上限 2 MiB。外部 HTTPS 隧道仅记录连接。"
              : data.capture === "storage_unavailable"
                ? "流量存储不可用，请求包无法保存。"
                : "录流未开启，仅记录连接。"}
          </p>
          <ProxyEventsTable events={data.events ?? []} />
        </>
      )}
    </section>
  );
}
