import { useState } from 'react'
import { Upload, Button, App as AntdApp, Typography, Tag } from 'antd'
import type { UploadProps } from 'antd'
import { UploadOutlined } from '@ant-design/icons'
import { uploadDocument } from '../api'
import type { KB, UploadResult } from '../types'

const { Text } = Typography

interface Props {
  kb: KB
}

interface DoneItem {
  file: string
  chunks: number
}

export default function UploadPanel({ kb }: Props) {
  const { message } = AntdApp.useApp()
  const [uploading, setUploading] = useState(false)
  const [done, setDone] = useState<DoneItem[]>([])

  const customRequest: UploadProps['customRequest'] = async (opt) => {
    const file = opt.file as File
    setUploading(true)
    try {
      const res: UploadResult = await uploadDocument(kb.id, file)
      message.success(`${res.file} 已摄取 ${res.chunks} 个 chunk`)
      setDone((prev) => [{ file: res.file, chunks: res.chunks }, ...prev])
      opt.onSuccess?.(res)
    } catch (e) {
      message.error(`${file.name}: ${(e as Error).message}`)
      opt.onError?.(e as Error)
    } finally {
      setUploading(false)
    }
  }

  return (
    <div>
      <Text type="secondary">
        支持 .md / .docx / .txt。重新上传同名文件会先清除旧 chunk 再写入。
      </Text>
      <div style={{ margin: '12px 0' }}>
        <Upload
          customRequest={customRequest}
          showUploadList={false}
          multiple={false}
          accept=".md,.docx,.txt,.markdown"
        >
          <Button icon={<UploadOutlined />} loading={uploading} type="primary">
            上传文件
          </Button>
        </Upload>
      </div>
      {done.length > 0 && (
        <div>
          {done.map((d, i) => (
            <div key={i} style={{ marginBottom: 4 }}>
              <Tag color="green">{d.chunks} chunks</Tag>
              <Text>{d.file}</Text>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
