import { useEffect, useRef, useState } from 'react'
import { Input, Button, Tag, Typography, Space, App as AntdApp, Empty } from 'antd'
import { SendOutlined } from '@ant-design/icons'
import { chat } from '../api'
import type { KB, Citation } from '../types'

const { Text } = Typography

interface Props {
  kb: KB
}

interface Msg {
  role: 'user' | 'assistant'
  content: string
  citations?: Citation[]
  error?: boolean
}

export default function ChatPanel({ kb }: Props) {
  const { message } = AntdApp.useApp()
  const [msgs, setMsgs] = useState<Msg[]>([])
  const [input, setInput] = useState('')
  const [streaming, setStreaming] = useState(false)
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [msgs])

  const send = async () => {
    const q = input.trim()
    if (!q || streaming) return
    setInput('')

    const userMsg: Msg = { role: 'user', content: q }
    const asstIdx = msgs.length + 1
    const asstMsg: Msg = { role: 'assistant', content: '' }
    setMsgs((prev) => [...prev, userMsg, asstMsg])
    setStreaming(true)

    try {
      await chat(kb.id, q, {
        onToken: (t) => {
          setMsgs((prev) => {
            const next = [...prev]
            next[asstIdx] = { ...next[asstIdx], content: next[asstIdx].content + t }
            return next
          })
        },
        onCitations: (cits) => {
          setMsgs((prev) => {
            const next = [...prev]
            next[asstIdx] = { ...next[asstIdx], citations: cits }
            return next
          })
        },
        onError: (msg) => {
          setMsgs((prev) => {
            const next = [...prev]
            next[asstIdx] = {
              ...next[asstIdx],
              content: next[asstIdx].content || '',
              error: true,
            }
            return next
          })
          message.error(`生成出错:${msg}`)
        },
      })
    } catch (e) {
      setMsgs((prev) => {
        const next = [...prev]
        next[asstIdx] = {
          ...next[asstIdx],
          content: (e as Error).message,
          error: true,
        }
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
                向知识库「{kb.name}」提问,回答会带 [1][2] 引用标注
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
                  {m.content || (streaming && i === msgs.length - 1 ? '思考中…' : '')}
                  {m.citations && m.citations.length > 0 && (
                    <div style={{ marginTop: 8, paddingTop: 8, borderTop: '1px solid rgba(0,0,0,0.08)' }}>
                      <Text style={{ fontSize: 12, color: m.role === 'user' ? '#fff' : '#888' }}>
                        参考来源:
                      </Text>
                      <Space size={4} wrap style={{ marginTop: 4 }}>
                        {m.citations.map((c, ci) => (
                          <Tag
                            key={ci}
                            style={{ fontSize: 12 }}
                            color={m.role === 'user' ? 'white' : 'blue'}
                          >
                            [{ci + 1}] {c.source_file}#{c.chunk_index} · {c.score.toFixed(3)}
                          </Tag>
                        ))}
                      </Space>
                    </div>
                  )}
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
