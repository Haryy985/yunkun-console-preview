"use client";

import * as React from "react";
import { api } from "@/lib/api";
import type { ProxyNode, ProxyPool } from "@/lib/proxy-pool-types";

export function TaskProxyPicker({
  value,
  nodeValue,
  onChange,
  onNodeChange,
}: {
  value?: number;
  nodeValue?: number;
  onChange: (value?: number) => void;
  onNodeChange: (value?: number) => void;
}) {
  const [pools, setPools] = React.useState<ProxyPool[]>([]);
  const [nodes, setNodes] = React.useState<ProxyNode[]>([]);
  const [mode, setMode] = React.useState<"auto" | "manual">("auto");
  const initialized = React.useRef(false);
  const [error, setError] = React.useState("");
  React.useEffect(() => {
    let active = true;
    Promise.all([api.proxyPools(), api.proxySettings()])
      .then(([poolResponse, settings]) => {
        if (!active) return;
        setPools(poolResponse.pools ?? []);
        if (!initialized.current) {
          initialized.current = true;
          if (settings.enabled && settings.default_pool_id) onChange(settings.default_pool_id);
        }
      })
      .catch((e: Error) => {
        if (active) setError(e.message);
      });
    return () => {
      active = false;
    };
  }, []);
  React.useEffect(() => {
    if (!value) {
      setNodes([]);
      onNodeChange(undefined);
      return;
    }
    api
      .proxyNodes(value)
      .then((r) => setNodes((r.nodes ?? []).filter((node) => node.enabled && !node.deleted_at)))
      .catch(() => setNodes([]));
  }, [value, onNodeChange]);
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <label htmlFor="task-proxy-pool" className="font-medium text-sm">任务代理池</label>
        <select
          className="h-8 rounded-md border bg-background px-2 text-xs"
          value={value ? "enabled" : "disabled"}
          onChange={(event) => {
            if (event.target.value === "disabled") {
              onChange(undefined);
              onNodeChange(undefined);
            }
          }}
        >
          <option value="enabled">使用代理</option>
          <option value="disabled">关闭代理</option>
        </select>
      </div>
      <select
        id="task-proxy-pool"
        className="h-9 w-full rounded-md border bg-background px-3 text-sm"
        value={value ?? ""}
        onChange={(e) => onChange(e.target.value ? Number(e.target.value) : undefined)}
      >
        <option value="">跟随全局出口配置</option>
        {pools
          .filter((p) => p.config.enabled)
          .map((p) => (
            <option key={p.id} value={p.id}>
              {p.config.name}
            </option>
          ))}
      </select>
      {value ? (
        <>
          <div className="flex items-center justify-between gap-3">
            <span className="text-muted-foreground text-xs">节点分配</span>
            <select className="h-8 rounded-md border bg-background px-2 text-xs" value={mode} onChange={(event) => {
              const next = event.target.value as "auto" | "manual";
              setMode(next);
              if (next === "auto") onNodeChange(undefined);
            }}>
              <option value="auto">自动分配</option>
              <option value="manual">手动选择</option>
            </select>
          </div>
          {mode === "manual" ? (
            <select
              className="h-9 w-full rounded-md border bg-background px-3 text-sm"
              value={nodeValue ?? ""}
              onChange={(event) => onNodeChange(event.target.value ? Number(event.target.value) : undefined)}
            >
              <option value="">选择可用节点</option>
              {nodes.map((node) => <option key={node.id} value={node.id}>{node.address}{node.verified_ip ? ` · ${node.verified_ip}` : ""}</option>)}
            </select>
          ) : null}
        </>
      ) : null}
      {error && (
        <p role="alert" className="text-destructive text-sm">
          {error}
        </p>
      )}
      {value && <p className="text-amber-700 text-xs dark:text-amber-400">固定节点 · 配置模式 · 网络隔离尚未验收</p>}
    </div>
  );
}
