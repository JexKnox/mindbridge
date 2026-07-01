import { useState } from 'react'
import { Modal, Form, Input, App as AntdApp } from 'antd'
import { createKB } from '../api'
import type { KB } from '../types'

interface Props {
  open: boolean
  onClose: () => void
  onCreated: (k: KB) => void
}

// 建库表单:embedding 模型名必填;base_url/api_key 可选(缺省则用后端全局配置)。
export default function CreateKBModal({ open, onClose, onCreated }: Props) {
  const [form] = Form.useForm()
  const { message } = AntdApp.useApp()
  const [loading, setLoading] = useState(false)

  const submit = async () => {
    const values = await form.validateFields()
    setLoading(true)
    try {
      const k = await createKB(values)
      onCreated(k)
      form.resetFields()
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <Modal
      title="新建知识库"
      open={open}
      onCancel={onClose}
      onOk={submit}
      confirmLoading={loading}
      okText="创建"
      cancelText="取消"
      destroyOnClose
    >
      <Form form={form} layout="vertical" initialValues={{}}>
        <Form.Item
          name="name"
          label="知识库名称"
          rules={[{ required: true, message: '请输入名称' }]}
        >
          <Input placeholder="如:产品手册" />
        </Form.Item>
        <Form.Item
          name="embedding_model"
          label="Embedding 模型"
          rules={[{ required: true, message: '请输入 embedding 模型名' }]}
          extra="同一知识库内不可变,维度据此探测。例:bge-m3、text-embedding-3-small"
        >
          <Input placeholder="bge-m3" />
        </Form.Item>
        <Form.Item name="embedding_base_url" label="Embedding Base URL(可选)">
          <Input placeholder="留空则用后端全局配置" />
        </Form.Item>
        <Form.Item name="embedding_api_key" label="Embedding API Key(可选)">
          <Input.Password placeholder="留空则用后端全局配置" />
        </Form.Item>
      </Form>
    </Modal>
  )
}
