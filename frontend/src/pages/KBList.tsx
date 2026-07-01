import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Card, List, Tag, Typography, App as AntdApp, Empty, Spin } from 'antd'
import { PlusOutlined, MessageOutlined } from '@ant-design/icons'
import { listKB } from '../api'
import type { KB } from '../types'
import CreateKBModal from '../components/CreateKBModal'

const { Title, Text } = Typography

export default function KBList() {
  const nav = useNavigate()
  const { message } = AntdApp.useApp()
  const [kbs, setKbs] = useState<KB[]>([])
  const [loading, setLoading] = useState(true)
  const [modalOpen, setModalOpen] = useState(false)

  const refresh = async () => {
    setLoading(true)
    try {
      setKbs(await listKB())
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    refresh()
  }, [])

  return (
    <div style={{ maxWidth: 960, margin: '0 auto', padding: 24 }}>
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          marginBottom: 16,
        }}
      >
        <Title level={3} style={{ margin: 0 }}>
          知识库
        </Title>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setModalOpen(true)}>
          新建知识库
        </Button>
      </div>

      <Card>
        {loading ? (
          <div style={{ textAlign: 'center', padding: 48 }}>
            <Spin />
          </div>
        ) : kbs.length === 0 ? (
          <Empty description="还没有知识库,先建一个吧" />
        ) : (
          <List
            dataSource={kbs}
            renderItem={(k) => (
              <List.Item
                actions={[
                  <Button
                    key="chat"
                    type="link"
                    icon={<MessageOutlined />}
                    onClick={() => nav(`/kb/${k.id}`)}
                  >
                    进入对话
                  </Button>,
                ]}
              >
                <List.Item.Meta
                  title={
                    <span>
                      {k.name}{' '}
                      <Text type="secondary" style={{ fontSize: 12 }}>
                        {k.collection}
                      </Text>
                    </span>
                  }
                  description={
                    <span>
                      <Tag color="blue">{k.embedding_model}</Tag>
                      <Tag>维度 {k.embedding_dim}</Tag>
                      <Text type="secondary" style={{ fontSize: 12 }}>
                        创建于 {new Date(k.created_at).toLocaleString()}
                      </Text>
                    </span>
                  }
                />
              </List.Item>
            )}
          />
        )}
      </Card>

      <CreateKBModal
        open={modalOpen}
        onClose={() => setModalOpen(false)}
        onCreated={(k) => {
          setModalOpen(false)
          message.success(`知识库「${k.name}」已创建`)
          nav(`/kb/${k.id}`)
        }}
      />
    </div>
  )
}
