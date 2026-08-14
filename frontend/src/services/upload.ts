// 文件分块上传（WebSocket 协议：upload_start → N 个 Binary Frame → upload_end）
import { ws } from './ws'

const CHUNK_SIZE = 256 * 1024 // 256KB/块

export interface UploadResult {
  path: string // 后端相对路径，如 /uploads/files/xxx.png
  file_name: string
  size: number
}

/** 分块上传文件，返回后端相对路径 */
export function uploadFile(
  file: File,
  onProgress?: (pct: number) => void,
): Promise<UploadResult> {
  return new Promise((resolve, reject) => {
    const totalChunks = Math.max(1, Math.ceil(file.size / CHUNK_SIZE))
    const uploadID = Math.random().toString(36).slice(2)

    function onAck(p: any) {
      cleanup()
      if (p && p.ok) {
        resolve({ path: p.path, file_name: p.file_name, size: p.size })
      } else {
        reject(new Error(p?.message ?? '上传失败'))
      }
    }
    function onError(p: any) {
      cleanup()
      reject(new Error(p?.message ?? '上传出错'))
    }
    function cleanup() {
      ws.off('upload_complete_ack', onAck)
      ws.off('error', onError)
    }

    ws.on('upload_complete_ack', onAck)
    ws.on('error', onError)

    // 连接未就绪则失败（调用方应保证已连接）
    if (!ws.connected) {
      cleanup()
      reject(new Error('未连接后端'))
      return
    }

    ws.send('upload_start', {
      upload_id: uploadID,
      file_name: file.name,
      total_chunks: totalChunks,
    })

    let idx = 0
    async function next(): Promise<void> {
      if (idx >= totalChunks) {
        ws.send('upload_end', { upload_id: uploadID })
        return
      }
      const buf = await file
        .slice(idx * CHUNK_SIZE, (idx + 1) * CHUNK_SIZE)
        .arrayBuffer()
      ws.sendBinary(buf)
      idx++
      onProgress?.(Math.round((idx / totalChunks) * 100))
      // WebSocket 保序，顺序发送即可；给主线程让出，避免大文件阻塞 UI
      await new Promise((r) => setTimeout(r, 0))
      await next()
    }
    void next()
  })
}
