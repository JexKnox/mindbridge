import type {
  KB,
  CreateKBRequest,
  UploadResult,
  Citation,
  MCPServer,
  CreateMCPServerRequest,
  FeedbackCategory,
  StatsOverview,
} from './types'

// 开发态走 Vite 代理(同源),生产态由 Go 直接托管,均为同源,故用相对路径。
const API_BASE = ''

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...(init?.headers || {}) },
  })
  if (!resp.ok) {
    let msg = `HTTP ${resp.status}`
    try {
      const body = await resp.json()
      msg = body.error || msg
    } catch {
      /* ignore */
    }
    throw new Error(msg)
  }
  return resp.json() as Promise<T>
}

export function listKB(): Promise<KB[]> {
  return request<KB[]>('/api/kb')
}

export function createKB(req: CreateKBRequest): Promise<KB> {
  return request<KB>('/api/kb', { method: 'POST', body: JSON.stringify(req) })
}

export function uploadDocument(kbId: string, file: File): Promise<UploadResult> {
  const form = new FormData()
  form.append('file', file)
  return fetch(`${API_BASE}/api/kb/${kbId}/documents`, { method: 'POST', body: form }).then(
    async (resp) => {
      if (!resp.ok) {
        let msg = `HTTP ${resp.status}`
        try {
          msg = (await resp.json()).error || msg
        } catch {
          /* ignore */
        }
        throw new Error(msg)
      }
      return resp.json() as Promise<UploadResult>
    },
  )
}

export interface ChatHandlers {
  onToken: (token: string) => void
  onCitations: (citations: Citation[]) => void
  onError: (msg: string) => void
  // 服务端在回答结束后推 trace_id(问答留痕),用于提交反馈。
  onTrace?: (traceId: string) => void
}

// chat 走 SSE:POST + 流式读取(EventSource 不支持 POST,故用 fetch + ReadableStream)。
// 服务端格式:逐 token 的 `data: ...` 事件、`event: citations`、`event: error`。
// 这里按空行(\n\n)切分事件,以正确处理 token 内含换行的情况。
export async function chat(kbId: string, query: string, h: ChatHandlers): Promise<void> {
  const resp = await fetch(`${API_BASE}/api/kb/${kbId}/chat`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ query }),
  })
  if (!resp.ok || !resp.body) {
    let msg = `HTTP ${resp.status}`
    try {
      msg = (await resp.json()).error || msg
    } catch {
      /* ignore */
    }
    throw new Error(msg)
  }

  const reader = resp.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  const dispatch = (raw: string) => {
    let event = ''
    const dataLines: string[] = []
    for (const line of raw.split('\n')) {
      if (line.startsWith('event:')) {
        event = line.slice(6).trim()
      } else if (line.startsWith('data:')) {
        // data: 后可能有一个前导空格(SSE 规范),去掉
        dataLines.push(line.slice(5).replace(/^ /, ''))
      }
    }
    if (dataLines.length === 0) return
    const data = dataLines.join('\n')
    if (event === 'citations') {
      try {
        h.onCitations(JSON.parse(data))
      } catch {
        /* ignore malformed */
      }
    } else if (event === 'trace') {
      try {
        h.onTrace?.(JSON.parse(data).trace_id)
      } catch {
        /* ignore malformed */
      }
    } else if (event === 'error') {
      h.onError(data)
    } else {
      h.onToken(data)
    }
  }

  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    let idx: number
    while ((idx = buffer.indexOf('\n\n')) >= 0) {
      const raw = buffer.slice(0, idx)
      buffer = buffer.slice(idx + 2)
      if (raw.trim()) dispatch(raw)
    }
  }
  if (buffer.trim()) dispatch(buffer)
}

// ===== agent 链路 =====

export interface AgentChatHandlers {
  onToken: (token: string) => void
  onToolCall: (tc: { name: string; args?: string }) => void
  onToolResult: (tc: { name: string; result?: string; error?: string }) => void
  onError: (msg: string) => void
}

// agentChat 走 SSE:与 chat() 同样的解析,但多出 tool_call/tool_result/done 事件。
// 服务端格式见 internal/server/agent_handler.go。
export async function agentChat(
  query: string,
  kbId: string | null,
  h: AgentChatHandlers,
): Promise<void> {
  const resp = await fetch(`${API_BASE}/api/agent/chat`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(kbId ? { query, kb_id: kbId } : { query }),
  })
  if (!resp.ok || !resp.body) {
    let msg = `HTTP ${resp.status}`
    try {
      msg = (await resp.json()).error || msg
    } catch {
      /* ignore */
    }
    throw new Error(msg)
  }

  const reader = resp.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  const dispatch = (raw: string) => {
    let event = ''
    const dataLines: string[] = []
    for (const line of raw.split('\n')) {
      if (line.startsWith('event:')) {
        event = line.slice(6).trim()
      } else if (line.startsWith('data:')) {
        dataLines.push(line.slice(5).replace(/^ /, ''))
      }
    }
    if (dataLines.length === 0) return
    const data = dataLines.join('\n')
    if (event === 'tool_call') {
      try {
        h.onToolCall(JSON.parse(data))
      } catch {
        /* ignore */
      }
    } else if (event === 'tool_result') {
      try {
        h.onToolResult(JSON.parse(data))
      } catch {
        /* ignore */
      }
    } else if (event === 'error') {
      h.onError(data)
    } else if (event === 'done') {
      /* 结束 */
    } else {
      h.onToken(data)
    }
  }

  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    let idx: number
    while ((idx = buffer.indexOf('\n\n')) >= 0) {
      const raw = buffer.slice(0, idx)
      buffer = buffer.slice(idx + 2)
      if (raw.trim()) dispatch(raw)
    }
  }
  if (buffer.trim()) dispatch(buffer)
}

// ===== 质量闭环:反馈 + 统计 =====

export function sendFeedback(
  traceId: string,
  category: FeedbackCategory,
  comment = '',
): Promise<void> {
  return request<void>('/api/feedback', {
    method: 'POST',
    body: JSON.stringify({ trace_id: traceId, category, comment }),
  })
}

export function fetchStats(days = 30, kbId = ''): Promise<StatsOverview> {
  const params = new URLSearchParams({ days: String(days) })
  if (kbId) params.set('kb_id', kbId)
  return request<StatsOverview>(`/api/stats/overview?${params}`)
}

// ===== MCP server CRUD =====

export function listMCPServers(): Promise<MCPServer[]> {
  return request<MCPServer[]>('/api/mcp/servers')
}

export function createMCPServer(req: CreateMCPServerRequest): Promise<MCPServer> {
  return request<MCPServer>('/api/mcp/servers', { method: 'POST', body: JSON.stringify(req) })
}

export function deleteMCPServer(id: string): Promise<void> {
  return request<void>(`/api/mcp/servers/${id}`, { method: 'DELETE' })
}
