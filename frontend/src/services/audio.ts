// 语音能力：录音（MediaRecorder）→ STT 上传
import { httpBase } from './backend'

/** 录音器：start 开始，stop 返回音频 Blob，cancel 放弃 */
export class VoiceRecorder {
  private mr: MediaRecorder | null = null
  private chunks: Blob[] = []
  private timer: number | null = null
  private startAt = 0
  /** 录音进行中回调（秒） */
  onTick: ((sec: number) => void) | null = null

  /** 浏览器是否支持录音 */
  static get supported(): boolean {
    return (
      typeof navigator !== 'undefined' &&
      !!navigator.mediaDevices?.getUserMedia &&
      typeof MediaRecorder !== 'undefined'
    )
  }

  get recording(): boolean {
    return this.mr !== null
  }

  async start(): Promise<void> {
    if (this.mr) return
    const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
    const mime = MediaRecorder.isTypeSupported('audio/webm;codecs=opus')
      ? 'audio/webm;codecs=opus'
      : MediaRecorder.isTypeSupported('audio/mp4')
        ? 'audio/mp4'
        : ''
    this.mr = new MediaRecorder(stream, mime ? { mimeType: mime } : undefined)
    this.chunks = []
    this.mr.ondataavailable = (e) => {
      if (e.data.size > 0) this.chunks.push(e.data)
    }
    this.mr.start(250)
    this.startAt = Date.now()
    this.timer = window.setInterval(() => {
      this.onTick?.((Date.now() - this.startAt) / 1000)
    }, 200)
  }

  /** 停止录音，返回音频 Blob（mime 由 MediaRecorder 决定） */
  stop(): Promise<Blob> {
    return new Promise((resolve, reject) => {
      const mr = this.mr
      if (!mr) {
        reject(new Error('未在录音'))
        return
      }
      mr.onstop = () => {
        const type = mr.mimeType || 'audio/webm'
        this.cleanup()
        resolve(new Blob(this.chunks, { type }))
      }
      mr.stop()
      mr.stream?.getTracks().forEach((t) => t.stop())
    })
  }

  /** 放弃录音 */
  cancel(): void {
    const mr = this.mr
    if (!mr) return
    mr.onstop = null
    mr.stop()
    mr.stream?.getTracks().forEach((t) => t.stop())
    this.cleanup()
  }

  private cleanup(): void {
    if (this.timer) {
      clearInterval(this.timer)
      this.timer = null
    }
    this.mr = null
    this.chunks = []
  }
}

/** 上传录音 → 识别文字（POST /api/v1/audio/stt） */
export async function sttUpload(blob: Blob): Promise<string> {
  const ext = blob.type.includes('mp4') ? 'm4a' : 'webm'
  const form = new FormData()
  form.append('file', blob, `voice.${ext}`)
  const resp = await fetch(`${httpBase()}/api/v1/audio/stt`, {
    method: 'POST',
    body: form,
  })
  if (!resp.ok) {
    const data = await resp.json().catch(() => null)
    throw new Error(data?.message ?? `语音识别失败: ${resp.status}`)
  }
  const data = await resp.json()
  return data.text ?? ''
}
