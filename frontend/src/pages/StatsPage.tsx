import { useCallback, useEffect, useState } from 'react'
import {
  Card,
  Col,
  Row,
  Statistic,
  Table,
  Segmented,
  Space,
  Button,
  Typography,
  App as AntdApp,
  Progress,
} from 'antd'
import { DownloadOutlined, ReloadOutlined } from '@ant-design/icons'
import { fetchStats } from '../api'
import { FEEDBACK_OPTIONS } from '../types'
import type { StatsOverview } from '../types'

const { Title, Text } = Typography

const categoryLabel = (v: string) =>
  FEEDBACK_OPTIONS.find((o) => o.value === v)?.label ?? v

export default function StatsPage() {
  const { message } = AntdApp.useApp()
  const [days, setDays] = useState(30)
  const [data, setData] = useState<StatsOverview | null>(null)
  const [loading, setLoading] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      setData(await fetchStats(days))
    } catch (e) {
      message.error(`加载统计失败:${(e as Error).message}`)
    } finally {
      setLoading(false)
    }
  }, [days, message])

  useEffect(() => {
    load()
  }, [load])

  const refuseRate = data && data.answers > 0 ? data.refusals / data.answers : 0
  const negative = data
    ? data.feedback.filter((f) => f.category !== 'helpful').reduce((s, f) => s + f.count, 0)
    : 0
  const helpful = data
    ? data.feedback.find((f) => f.category === 'helpful')?.count ?? 0
    : 0

  return (
    <div style={{ padding: 24, maxWidth: 1080, margin: '0 auto' }}>
      <Space style={{ width: '100%', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4} style={{ margin: 0 }}>
          质量统计
        </Title>
        <Space>
          <Segmented
            options={[
              { label: '7 天', value: 7 },
              { label: '30 天', value: 30 },
              { label: '90 天', value: 90 },
            ]}
            value={days}
            onChange={(v) => setDays(v as number)}
          />
          <Button icon={<ReloadOutlined />} onClick={load} loading={loading} />
          <Button
            icon={<DownloadOutlined />}
            href={`/api/stats/export?days=${days}`}
            target="_blank"
          >
            导出评测集
          </Button>
        </Space>
      </Space>

      <Row gutter={[16, 16]}>
        <Col xs={12} md={6}>
          <Card>
            <Statistic title="回答数" value={data?.answers ?? 0} />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card>
            <Statistic
              title="拒答率"
              value={refuseRate * 100}
              precision={1}
              suffix="%"
              valueStyle={refuseRate > 0.3 ? { color: '#cf1322' } : undefined}
            />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card>
            <Statistic
              title="top1 均分(余弦)"
              value={data?.avg_top_score ?? 0}
              precision={3}
            />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card>
            <Statistic
              title={`忠实度均分(抽样 ${data?.judged_count ?? 0} 条)`}
              value={data && data.judged_count > 0 ? data.avg_judge_score : 0}
              precision={2}
              valueStyle={
                data && data.judged_count > 0 && data.avg_judge_score < 0.7
                  ? { color: '#cf1322' }
                  : undefined
              }
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} md={12}>
          <Card
            title="用户反馈分布"
            extra={
              <Text type="secondary" style={{ fontSize: 12 }}>
                有帮助 {helpful} · 负反馈 {negative}
              </Text>
            }
          >
            {data && data.feedback.length > 0 ? (
              <Table
                size="small"
                rowKey="category"
                pagination={false}
                dataSource={data.feedback}
                columns={[
                  {
                    title: '分类',
                    dataIndex: 'category',
                    render: (v: string) => categoryLabel(v),
                  },
                  { title: '次数', dataIndex: 'count', width: 80 },
                  {
                    title: '占比',
                    width: 160,
                    render: (_, r) => {
                      const total = data.feedback.reduce((s, f) => s + f.count, 0)
                      return (
                        <Progress
                          percent={total > 0 ? Math.round((r.count / total) * 100) : 0}
                          size="small"
                        />
                      )
                    },
                  },
                ]}
              />
            ) : (
              <Text type="secondary">暂无反馈</Text>
            )}
          </Card>
        </Col>
        <Col xs={24} md={12}>
          <Card title="每日问答量">
            {data && data.daily.length > 0 ? (
              <Table
                size="small"
                rowKey="day"
                pagination={{ pageSize: 8, hideOnSinglePage: true }}
                dataSource={[...data.daily].reverse()}
                columns={[
                  { title: '日期', dataIndex: 'day' },
                  { title: '回答', dataIndex: 'answers', width: 80 },
                  {
                    title: '拒答',
                    dataIndex: 'refused',
                    width: 80,
                    render: (v: number) => (v > 0 ? <Text type="danger">{v}</Text> : v),
                  },
                ]}
              />
            ) : (
              <Text type="secondary">暂无数据</Text>
            )}
          </Card>
        </Col>
      </Row>
    </div>
  )
}
