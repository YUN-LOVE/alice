// Markdown 渲染（marked + DOMPurify 防 XSS）
import { marked } from 'marked'
import DOMPurify from 'dompurify'

marked.setOptions({
  gfm: true,
  breaks: true,
})

// 渲染 Markdown 为安全的 HTML
export function renderMarkdown(text: string): string {
  const html = marked.parse(text, { async: false }) as string
  return DOMPurify.sanitize(html, {
    // 允许图片、链接、代码块、表格等
    ADD_ATTR: ['target', 'rel'],
  })
}

// 检查是否包含 Markdown 语法（用于流式时决定是否需要延迟渲染）
export function containsMarkdown(text: string): boolean {
  return /[#*`\[\]>|_-]/.test(text)
}
