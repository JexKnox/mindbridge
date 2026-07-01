import { useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Layout, Tabs, Typography, Button, Spin, App as AntdApp, Tag } from 'antd'
import { ArrowLeftOutlined } from '@ant-design/icons'
import { listKB } from '../api'
import type { KB } from '../types'
import UploadPanel from '../components/UploadPanel'
import ChatPanel from '../components/ChatPanel'

const { Sider, Content } = Layout
const { Title, Text } = Typography

export default function KBDetail() {
  const { id } = useParams<{ id: string }>()
  const nav = useNavigate()
  const { message } = AntdApp.useApp()
  const [kb, setKb] = useState<KB | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (!id) return
    setLoading(true)
    listKB()
      .then((ks) => {
        const found = ks.find((k) => k.id === id)
        if (!found) {
          message.error('知识库不存在')
          nav('/kb')
          return
        }
        setKb(found)
      })
      .catch((e) => message.error((e as Error).message))
      .finally(() => setLoading(false))
  }, [id, nav, message])

  if (loading || !kb) {
    return (
      <div style={{ textAlign: 'center', padding: 64 }}>
        <Spin />
      </div>
    )
  }

  return (
    <Layout style={{ height: '100%' }}>
      <Sider width={300} theme="light" style={{ padding: 16, borderRight: '1px solid #f0f0f0' }}>
        <Button
          icon={<ArrowLeftOutlined />}
          type="text"
          onClick={() => nav('/kb')}
          style={{ marginBottom: 12 }}
        >
          返回列表
        </Button>
        <Title level={5} style={{ marginTop: 0 }}>
          {kb.name}
        </Title>
        <div style={{ marginBottom: 8 }}>
          <Tag color="blue">{kb.embedding_model}</Tag>
          <Tag>维度 {kb.embedding_dim}</Tag>
        </div>
        <Text type="secondary" style={{ fontSize: 12 }}>
          collection: {kb.collection}
        </Text>
      </Sider>
      <Content style={{ background: '#fff' }}>
        <Tabs
          defaultActiveKey="chat"
          style={{ height: '100%' }}
          items={[
            {
              key: 'chat',
              label: '对话',
              children: (
                <div style={{ height: 'calc(100vh - 56px - 55px)' }}>
                  <ChatPanel kb={kb} />
                </div>
              ),
            },
            {
              key: 'docs',
              label: '文档',
              children: (
                <div style={{ padding: 16 }}>
                  <UploadPanel kb={kb} />
                </div>
              ),
            },
          ]}
        />
      </Content>
    </Layout>
  )
}
