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

// 用户反馈分类(与后端 internal/trace 对齐):分类后的反馈才能进入下一轮评测集。
export type FeedbackCategory =
  | 'helpful'
  | 'not_found'
  | 'outdated'
  | 'irrelevant'
  | 'not_actionable'
  | 'other'

export const FEEDBACK_OPTIONS: { value: FeedbackCategory; label: string }[] = [
  { value: 'helpful', label: '有帮助' },
  { value: 'not_found', label: '没找到资料' },
  { value: 'outdated', label: '资料过期' },
  { value: 'irrelevant', label: '答非所问' },
  { value: 'not_actionable', label: '步骤不可执行' },
  { value: 'other', label: '其他' },
]

// 统计总览(GET /api/stats/overview)。
export interface StatsOverview {
  days: number
  answers: number
  refusals: number
  avg_top_score: number
  judged_count: number
  avg_judge_score: number
  feedback: { category: string; count: number }[]
  daily: { day: string; answers: number; refused: number }[]
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
