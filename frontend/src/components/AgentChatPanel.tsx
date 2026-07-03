import { useEffect, useRef, useState } from 'react'
import { Input, Button, Typography, Space, App as AntdApp, Empty, Tag, Collapse } from 'antd'
import { SendOutlined } from '@ant-design/icons'
import { agentChat } from '../api'
import type { ToolCall } from '../types'

const { Text } = Typography

interface Props {
  kbId: string | null
  kbName: string | null
}

interface Msg {
  role: 'user' | 'assistant'
  content: string
  toolCalls: ToolCall[]
  error?: boolean
}

export default function AgentChatPanel({ kbId, kbName }: Props) {
  const { message } = AntdApp.useApp()
  const [msgs, setMsgs] = useState<Msg[]>([])
  const [input, setInput] = useState('')
  const [streaming, setStreaming] = useState(false)
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [msgs])

  // 切换 KB 时清空历史(避免跨库串话)。
  useEffect(() => {
    setMsgs([])
  }, [kbId])

  const send = async () => {
    const q = input.trim()
    if (!q || streaming) return
    setInput('')

    const asstIdx = msgs.length + 1
    setMsgs((prev) => [
      ...prev,
      { role: 'user', content: q, toolCalls: [] },
      { role: 'assistant', content: '', toolCalls: [] },
    ])
    setStreaming(true)

    try {
      await agentChat(q, kbId, {
        onToken: (t) => {
          setMsgs((prev) => {
            const next = [...prev]
            next[asstIdx] = { ...next[asstIdx], content: next[asstIdx].content + t }
            return next
          })
        },
        onToolCall: (tc) => {
          setMsgs((prev) => {
            const next = [...prev]
            next[asstIdx] = {
              ...next[asstIdx],
              toolCalls: [...next[asstIdx].toolCalls, { name: tc.name, args: tc.args, status: 'running' }],
            }
            return next
          })
        },
        onToolResult: (tc) => {
          setMsgs((prev) => {
            const next = [...prev]
            const calls = [...next[asstIdx].toolCalls]
            // 倒序找最近一个同名 running 调用,填入结果。
            for (let i = calls.length - 1; i >= 0; i--) {
              if (calls[i].name === tc.name && calls[i].status === 'running') {
                calls[i] = {
                  ...calls[i],
                  result: tc.result,
                  error: tc.error,
                  status: tc.error ? 'error' : 'done',
                }
                break
              }
            }
            next[asstIdx] = { ...next[asstIdx], toolCalls: calls }
            return next
          })
        },
        onError: (msg) => {
          setMsgs((prev) => {
            const next = [...prev]
            next[asstIdx] = { ...next[asstIdx], error: true }
            return next
          })
          message.error(`生成出错:${msg}`)
        },
      })
    } catch (e) {
      setMsgs((prev) => {
        const next = [...prev]
        next[asstIdx] = { ...next[asstIdx], content: (e as Error).message, error: true }
        return next
      })
    } finally {
      setStreaming(false)
    }
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <div style={{ flex: 1, overflow: 'auto', padding: 16 }}>
        {msgs.length === 0 ? (
          <Empty
            description={
              <span>
                Agent 模式:自主决定是否检索
                {kbName ? `「${kbName}」` : '知识库'},并调用 calculator / web_fetch / MCP 等工具
              </span>
            }
            style={{ marginTop: 64 }}
          />
        ) : (
          <>
            {msgs.map((m, i) => (
              <div
                key={i}
                style={{
                  display: 'flex',
                  justifyContent: m.role === 'user' ? 'flex-end' : 'flex-start',
                  marginBottom: 12,
                }}
              >
                <div
                  style={{
                    maxWidth: '78%',
                    padding: '10px 14px',
                    borderRadius: 10,
                    background: m.role === 'user' ? '#1677ff' : '#f5f5f5',
                    color: m.role === 'user' ? '#fff' : m.error ? '#cf1322' : 'inherit',
                    whiteSpace: 'pre-wrap',
                    wordBreak: 'break-word',
                  }}
                >
                  {m.toolCalls.length > 0 && (
                    <div style={{ marginBottom: 8 }}>
                      <Collapse
                        size="small"
                        items={m.toolCalls.map((tc, ci) => ({
                          key: ci,
                          label: (
                            <Space size={4}>
                              <Tag color={tc.status === 'running' ? 'processing' : tc.status === 'error' ? 'red' : 'green'} style={{ margin: 0 }}>
                                {tc.status === 'running' ? '调用中' : tc.status === 'error' ? '失败' : '完成'}
                              </Tag>
                              <Text strong>{tc.name}</Text>
                            </Space>
                          ),
                          children: (
                            <div style={{ fontSize: 12 }}>
                              {tc.args && (
                                <div>
                                  <Text type="secondary">参数:</Text>
                                  <pre style={{ margin: '4px 0', whiteSpace: 'pre-wrap' }}>{tc.args}</pre>
                                </div>
                              )}
                              {tc.result && (
                                <div>
                                  <Text type="secondary">结果:</Text>
                                  <pre style={{ margin: '4px 0', whiteSpace: 'pre-wrap' }}>{tc.result}</pre>
                                </div>
                              )}
                              {tc.error && (
                                <div>
                                  <Text type="danger">错误:{tc.error}</Text>
                                </div>
                              )}
                            </div>
                          ),
                        }))}
                      />
                    </div>
                  )}
                  {m.content || (streaming && i === msgs.length - 1 ? '思考中…' : '')}
                </div>
              </div>
            ))}
            <div ref={bottomRef} />
          </>
        )}
      </div>
      <div style={{ padding: 12, borderTop: '1px solid #f0f0f0' }}>
        <Space.Compact style={{ width: '100%' }}>
          <Input
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onPressEnter={send}
            placeholder={streaming ? '生成中…' : '输入问题,回车发送'}
            disabled={streaming}
          />
          <Button
            type="primary"
            icon={<SendOutlined />}
            onClick={send}
            loading={streaming}
            disabled={!input.trim()}
          >
            发送
          </Button>
        </Space.Compact>
      </div>
    </div>
  )
}
