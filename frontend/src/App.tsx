import { Routes, Route, Navigate, useNavigate, useLocation } from 'react-router-dom'
import { Layout, Typography, Space, Button } from 'antd'
import KBList from './pages/KBList'
import KBDetail from './pages/KBDetail'
import AgentPage from './pages/AgentPage'

const { Header, Content } = Layout
const { Title } = Typography

export default function App() {
  const nav = useNavigate()
  const loc = useLocation()
  const onAgent = loc.pathname.startsWith('/agent')

  return (
    <Layout style={{ height: '100vh' }}>
      <Header
        style={{
          display: 'flex',
          alignItems: 'center',
          color: '#fff',
          background: '#001529',
        }}
      >
        <Title level={4} style={{ color: '#fff', margin: 0, cursor: 'pointer' }} onClick={() => nav('/kb')}>
          🌉 MindBridge
        </Title>
        <span style={{ marginLeft: 12, color: 'rgba(255,255,255,0.65)', fontSize: 13 }}>
          通用 RAG 知识库
        </span>
        <Space style={{ marginLeft: 'auto' }}>
          <Button type={onAgent ? 'primary' : 'default'} onClick={() => nav('/kb')}>
            知识库
          </Button>
          <Button type={onAgent ? 'primary' : 'default'} onClick={() => nav('/agent')}>
            Agent
          </Button>
        </Space>
      </Header>
      <Content style={{ padding: 0, overflow: 'auto' }}>
        <Routes>
          <Route path="/" element={<Navigate to="/kb" replace />} />
          <Route path="/kb" element={<KBList />} />
          <Route path="/kb/:id" element={<KBDetail />} />
          <Route path="/agent" element={<AgentPage />} />
        </Routes>
      </Content>
    </Layout>
  )
}
