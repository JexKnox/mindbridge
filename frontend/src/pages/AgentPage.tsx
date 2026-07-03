import { useEffect, useState } from 'react'
import { Layout, Select, Typography, Space, Divider, List, Button, Input, Form, Tag, App as AntdApp, Spin } from 'antd'
import { DeleteOutlined } from '@ant-design/icons'
import { listKB, listMCPServers, createMCPServer, deleteMCPServer } from '../api'
import AgentChatPanel from '../components/AgentChatPanel'
import type { KB, MCPServer } from '../types'

const { Sider, Content } = Layout
const { Title, Text } = Typography

export default function AgentPage() {
  const { message, modal } = AntdApp.useApp()
  const [kbs, setKbs] = useState<KB[]>([])
  const [kbId, setKbId] = useState<string | null>(null)
  const [servers, setServers] = useState<MCPServer[]>([])
  const [loading, setLoading] = useState(true)
  const [form] = Form.useForm()

  const kbName = kbs.find((k) => k.id === kbId)?.name ?? null

  const refresh = async () => {
    const [ks, srvs] = await Promise.all([listKB(), listMCPServers()])
    setKbs(ks)
    setServers(srvs)
  }

  useEffect(() => {
    refresh()
      .catch((e) => message.error((e as Error).message))
      .finally(() => setLoading(false))
  }, [message])

  const addServer = () => {
    form.validateFields().then(async (v) => {
      try {
        await createMCPServer(v)
        form.resetFields()
        await refresh()
        message.success('已添加 MCP server')
      } catch (e) {
        message.error((e as Error).message)
      }
    })
  }

  const removeServer = (s: MCPServer) => {
    modal.confirm({
      title: `删除 MCP server「${s.name}」?`,
      onOk: async () => {
        try {
          await deleteMCPServer(s.id)
          await refresh()
          message.success('已删除')
        } catch (e) {
          message.error((e as Error).message)
        }
      },
    })
  }

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: 64 }}>
        <Spin />
      </div>
    )
  }

  return (
    <Layout style={{ height: '100%' }}>
      <Sider width={320} theme="light" style={{ padding: 16, borderRight: '1px solid #f0f0f0', overflow: 'auto' }}>
        <Title level={5} style={{ marginTop: 0 }}>
          🤖 Agent
        </Title>
        <Text type="secondary" style={{ fontSize: 12 }}>
          检索 → 决策 → 调工具 → 生成
        </Text>

        <Divider style={{ margin: '12px 0' }} />

        <Text strong>知识库</Text>
        <Select
          allowClear
          placeholder="不绑定知识库"
          style={{ width: '100%', marginTop: 6 }}
          value={kbId ?? undefined}
          onChange={(v) => setKbId(v ?? null)}
          options={kbs.map((k) => ({ label: k.name, value: k.id }))}
        />
        <Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 4 }}>
          {kbName ? `将作为 kb_search 工具交给 agent` : '纯 MCP + skill 工具 agent'}
        </Text>

        <Divider style={{ margin: '12px 0' }} />

        <Text strong>MCP Server</Text>
        <List
          size="small"
          dataSource={servers}
          locale={{ emptyText: '未配置' }}
          renderItem={(s) => (
            <List.Item
              actions={[
                <Button size="small" type="text" danger icon={<DeleteOutlined />} onClick={() => removeServer(s)} />,
              ]}
            >
              <List.Item.Meta
                title={<Text style={{ fontSize: 13 }}>{s.name}</Text>}
                description={
                  <Space size={4} wrap>
                    <Tag color="purple" style={{ margin: 0 }}>{s.transport}</Tag>
                    <Text style={{ fontSize: 11 }} type="secondary">
                      {s.endpoint}
                    </Text>
                  </Space>
                }
              />
            </List.Item>
          )}
        />

        <Divider style={{ margin: '12px 0' }} />
        <Form form={form} layout="vertical" size="small">
          <Form.Item name="name" label="名称" rules={[{ required: true }]}>
            <Input placeholder="demo" />
          </Form.Item>
          <Form.Item name="transport" label="Transport" rules={[{ required: true }]} initialValue="sse">
            <Select
              options={[
                { label: 'sse', value: 'sse' },
                { label: 'http', value: 'http' },
              ]}
            />
          </Form.Item>
          <Form.Item name="endpoint" label="Endpoint" rules={[{ required: true }]}>
            <Input placeholder="http://localhost:12345/sse" />
          </Form.Item>
          <Button type="primary" block onClick={addServer}>
            保存
          </Button>
        </Form>
      </Sider>
      <Content style={{ background: '#fff' }}>
        <div style={{ height: 'calc(100vh - 56px)' }}>
          <AgentChatPanel kbId={kbId} kbName={kbName} />
        </div>
      </Content>
    </Layout>
  )
}
