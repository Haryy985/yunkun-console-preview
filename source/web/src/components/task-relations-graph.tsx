"use client";
"use no memo";

import * as React from "react";
import {
  Background,
  BackgroundVariant,
  Controls,
  MarkerType,
  ReactFlow,
  type Edge,
  type Node,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";

import type { Task } from "@/lib/types";

export function TaskRelationsGraph({
  tasks, total, onTaskSelect,
}: {
  tasks: Task[];
  total: number;
  onTaskSelect: (task: Task) => void;
}) {
  const { nodes, edges } = React.useMemo(() => {
    const visible = new Set(tasks.map((task) => task.id));
    const nodes: Node<{ label: string }>[]= tasks.map((task, index) => ({
      id: task.id,
      position: { x: (index % 4) * 254, y: Math.floor(index / 4) * 92 },
      data: { label: `#${task.id} · ${task.name?.trim() ? task.name : task.description.slice(0, 36)} · ${task.status}` },
      style: {
        width: 218,
        minHeight: 58,
        borderRadius: 6,
        border: "1px solid var(--border)",
        background: "var(--background)",
        color: "var(--foreground)",
        fontSize: 12,
        overflowWrap: "anywhere",
      },
    }));
    const edges: Edge[] = tasks.flatMap((task) => (task.source_task_ids ?? [])
      .filter((source) => visible.has(source))
      .map((source) => ({
        id: `${source}:${task.id}`,
        source,
        target: task.id,
        label: "继承上下文",
        markerEnd: { type: MarkerType.ArrowClosed },
        style: { stroke: "#0f766e" },
        labelStyle: { fontSize: 10, fill: "#0f766e" },
      })));
    return { nodes, edges };
  }, [tasks]);
  const byID = React.useMemo(() => new Map(tasks.map((task) => [task.id, task])), [tasks]);
  return (
    <div className="px-4 lg:px-6">
      <p className="border-y py-2 text-muted-foreground text-xs">
        任务间只显示明确的上下文继承关系。点击节点查看该任务的证据关系。
        {total > tasks.length && ` 当前展示前 ${tasks.length} / ${total} 项，请缩小筛选范围。`}
      </p>
      <div className="hidden h-[min(72vh,780px)] min-h-96 md:block">
        <ReactFlow
          nodes={nodes}
          edges={edges}
          fitView
          nodesDraggable={false}
          nodesConnectable={false}
          onNodeClick={(_, node) => { const task = byID.get(node.id); if (task) onTaskSelect(task); }}
          proOptions={{ hideAttribution: true }}
        >
          <Background variant={BackgroundVariant.Dots} gap={24} size={1} />
          <Controls showInteractive={false} />
        </ReactFlow>
      </div>
      <div className="divide-y md:hidden">
        {tasks.map((task) => (
          <button key={task.id} type="button" onClick={() => onTaskSelect(task)}
            className="flex w-full flex-col gap-1 py-3 text-left text-sm">
            <span className="break-words font-medium">#{task.id} · {task.name?.trim() ? task.name : task.description}</span>
            <span className="break-words text-muted-foreground text-xs">
              {task.status}{task.source_task_ids?.length ? ` · 继承自 ${task.source_task_ids.map((id) => `#${id}`).join("、")}` : ""}
            </span>
          </button>
        ))}
      </div>
    </div>
  );
}
