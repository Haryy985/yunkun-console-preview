import type { RAGDocument, RAGDocumentDetail } from "@/lib/types";
import { auth } from "@/lib/auth";

function token() {
  return auth.getToken();
}

async function request<T>(path: string, method = "GET"): Promise<T> {
  const auth = token();
  const response = await fetch(`/api${path}`, {
    method,
    headers: {
      ...(auth ? { Authorization: `Bearer ${auth}` } : {}),
      ...(method === "POST" ? { "Content-Type": "application/json" } : {}),
    },
    ...(method === "POST" ? { body: "{}" } : {}),
  });
  const body = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(body?.error || `请求失败(${response.status})`);
  return body as T;
}

export const ragDocumentApi = {
  detail: (id: number) => request<RAGDocumentDetail>(`/rag/documents/${id}`),
  restore: (id: number) => request<RAGDocument>(`/rag/documents/${id}/restore`, "POST"),
  reparse: (id: number) =>
    request<{ document_id: number; job_id: number; status: string }>(`/rag/documents/${id}/reparse`, "POST"),
  retry: (id: number) =>
    request<{ document_id: number; job_id: number; status: string }>(`/rag/documents/${id}/retry`, "POST"),
  rebuildVector: (id: number) =>
    request<{ document_id: number; job_id: number; status: string }>(`/rag/documents/${id}/rebuild-vector`, "POST"),
};
