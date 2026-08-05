// Material You 动态主题引擎（Monet 取色）
// 从 seed color 或壁纸图片生成 M3 tonal palette，输出 CSS 变量 --md-sys-color-*
import {
  themeFromSourceColor,
  themeFromImage,
  hexFromArgb,
  argbFromHex,
  type Scheme,
  type Theme,
} from '@material/material-color-utilities'

export interface ThemeState {
  seed: string // 种子色（hex），壁纸取色的主色
  mode: 'dark' | 'light'
}

const LS_KEY = 'alice.theme.m3'
const DEFAULT_SEED = '#7c5cff' // Material You 默认紫

export function getSavedTheme(): ThemeState {
  try {
    const raw = localStorage.getItem(LS_KEY)
    if (raw) {
      const t = JSON.parse(raw)
      if (t.seed && (t.mode === 'dark' || t.mode === 'light')) return t
    }
  } catch {}
  return { seed: DEFAULT_SEED, mode: 'dark' }
}

export function saveTheme(state: ThemeState) {
  localStorage.setItem(LS_KEY, JSON.stringify(state))
}

// 应用主题：根据 seed color 生成 M3 scheme 并写入 CSS 变量
export function applyThemeFromSeed(seed: string, mode: 'dark' | 'light') {
  const theme = themeFromSourceColor(argbFromHex(seed))
  applyTheme(theme, mode)
}

// 从壁纸图片取色生成主题（Monet）
export async function applyThemeFromImage(
  image: string | HTMLImageElement,
  mode: 'dark' | 'light'
): Promise<string> {
  const theme = await themeFromImage(image)
  applyTheme(theme, mode)
  return hexFromArgb(theme.source)
}

// 写入 M3 tokens + 由中性色阶生成 surface container 色阶
function applyTheme(theme: Theme, mode: 'dark' | 'light') {
  const scheme = theme.schemes[mode]
  const neutral = theme.palettes.neutral

  // M3 surface 色阶：按规范 tone 值从中性色板取值
  // dark: 4/10/12/17/22；light: 100/96/94/92/90
  const surfaceContainer = {
    'surface-container-lowest': neutral.tone(mode === 'dark' ? 4 : 100),
    'surface-container-low': neutral.tone(mode === 'dark' ? 10 : 96),
    'surface-container': neutral.tone(mode === 'dark' ? 12 : 94),
    'surface-container-high': neutral.tone(mode === 'dark' ? 17 : 92),
    'surface-container-highest': neutral.tone(mode === 'dark' ? 22 : 90),
  }

  const root = document.documentElement.style
  const tokens: [string, number][] = [
    ['primary', scheme.primary],
    ['on-primary', scheme.onPrimary],
    ['primary-container', scheme.primaryContainer],
    ['on-primary-container', scheme.onPrimaryContainer],
    ['secondary', scheme.secondary],
    ['on-secondary', scheme.onSecondary],
    ['secondary-container', scheme.secondaryContainer],
    ['on-secondary-container', scheme.onSecondaryContainer],
    ['tertiary', scheme.tertiary],
    ['on-tertiary', scheme.onTertiary],
    ['tertiary-container', scheme.tertiaryContainer],
    ['on-tertiary-container', scheme.onTertiaryContainer],
    ['error', scheme.error],
    ['on-error', scheme.onError],
    ['surface', scheme.surface],
    ['on-surface', scheme.onSurface],
    ['surface-variant', scheme.surfaceVariant],
    ['on-surface-variant', scheme.onSurfaceVariant],
    ['outline', scheme.outline],
    ['outline-variant', scheme.outlineVariant],
    ['background', scheme.background],
    ['on-background', scheme.onBackground],
    ['inverse-surface', scheme.inverseSurface],
    ['inverse-primary', scheme.inversePrimary],
    ['shadow', scheme.shadow],
    ['scrim', scheme.scrim],
  ]
  for (const [k, v] of tokens) {
    root.setProperty(`--md-sys-color-${k}`, hexFromArgb(v))
  }
  for (const [k, v] of Object.entries(surfaceContainer)) {
    root.setProperty(`--md-sys-color-${k}`, hexFromArgb(v))
  }
  // 同步 zinc 调色板（现有组件用 bg-zinc-* / text-zinc-* 自动获得 M3 配色）
  mapZincToScheme(scheme, surfaceContainer)
}

// 把 Tailwind 的 zinc 调色板映射到 M3 token，让现有组件无需改动即获得 M3 配色。
// 关键：surface 有 5 层色阶（lowest→highest 渐亮），文字分主/次两级，
// 边框用 outline 系——这样 UI 才有层次且对比度符合 M3 规范。
function mapZincToScheme(
  scheme: Scheme,
  sc: { [k: string]: number } /* surface container 色阶（argb） */
) {
  const root = document.documentElement.style
  const hex = (v: number) => hexFromArgb(v)
  const map: Record<string, number> = {
    // 背景/表面：5 层色阶（从深到浅）
    '--color-zinc-950': sc['surface-container-lowest'], // 页面最底
    '--color-zinc-900': sc['surface-container-low'], // 顶部栏 / 面板
    '--color-zinc-800': sc['surface-container'], // 卡片 / 气泡
    '--color-zinc-700': sc['surface-container-high'], // 输入框 / 浮动层
    '--color-zinc-600': sc['surface-container-highest'],
    // 边框 / 分割线
    '--color-zinc-500': scheme.outline,
    '--color-zinc-300': scheme.outlineVariant,
    // 文字（主/次两级，符合 M3 对比度规范）
    '--color-zinc-400': scheme.onSurfaceVariant,
    '--color-zinc-200': scheme.onSurfaceVariant,
    '--color-zinc-100': scheme.onSurface,
    '--color-zinc-50': scheme.onSurface,
    // 强调色 → primary / tertiary / error
    '--color-purple-500': scheme.primary,
    '--color-purple-600': scheme.primary,
    '--color-pink-500': scheme.tertiary,
    '--color-emerald-400': scheme.tertiary,
    '--color-emerald-500': scheme.tertiary,
    '--color-red-500': scheme.error,
  }
  for (const [k, v] of Object.entries(map)) {
    root.setProperty(k, hex(v))
  }
}
