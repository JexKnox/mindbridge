// 与后端 internal/kb/kb.go、internal/rag/service.go 对齐的类型。

export interface KB {
  id: string
  name: string
  collection: string // 形如 kb_{id}
  embedding_model: string
  embedding_dim: number
  created_at: string
}

export interface CreateKBRequest {
  name: string
  embedding_model: string
  embedding_base_url?: string
  embedding_api_key?: string
}

export interface UploadResult {
  kb_id: string
  file: string
  chunks: number
  collection: string
}

// rag.Citation
export interface Citation {
  source_file: string
  chunk_index: number
  score: number
}

// agent 工具调用事件(tool_call/tool_result)。
export interface ToolCall {
  name: string
  args?: string
  result?: string
  error?: string
  status: 'running' | 'done' | 'error'
}

// mcp.Server
export interface MCPServer {
  id: string
  name: string
  transport: 'sse' | 'http'
  endpoint: string
  created_at: string
}

export interface CreateMCPServerRequest {
  name: string
  transport: 'sse' | 'http'
  endpoint: string
}
