import { Routes, Route, Navigate } from 'react-router-dom'
import { Layout, Typography } from 'antd'
import KBList from './pages/KBList'
import KBDetail from './pages/KBDetail'

const { Header, Content } = Layout
const { Title } = Typography

export default function App() {
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
        <Title level={4} style={{ color: '#fff', margin: 0 }}>
          🌉 MindBridge
        </Title>
        <span style={{ marginLeft: 12, color: 'rgba(255,255,255,0.65)', fontSize: 13 }}>
          通用 RAG 知识库
        </span>
      </Header>
      <Content style={{ padding: 0, overflow: 'auto' }}>
        <Routes>
          <Route path="/" element={<Navigate to="/kb" replace />} />
          <Route path="/kb" element={<KBList />} />
          <Route path="/kb/:id" element={<KBDetail />} />
        </Routes>
      </Content>
    </Layout>
  )
}
