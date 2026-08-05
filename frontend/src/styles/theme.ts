// Material You 动态主题引擎（Monet 取色）
// 从 seed color 或壁纸图片生成 M3 tonal palette，输出 CSS 变量 --md-sys-color-*
import {
  themeFromSourceColor,
  themeFromImage,
  hexFromArgb,
  argbFromHex,
  type Scheme,
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
  applyScheme(theme.schemes[mode])
}

// 从壁纸图片取色生成主题（Monet）
export async function applyThemeFromImage(
  image: string | HTMLImageElement,
  mode: 'dark' | 'light'
): Promise<string> {
  const theme = await themeFromImage(image)
  const seed = hexFromArgb(theme.source)
  applyScheme(theme.schemes[mode])
  return seed
}

// 生成 M3 scheme → CSS 变量
function applyScheme(scheme: Scheme) {
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
    ['surface-container-lowest', scheme.surfaceContainerLowest],
    ['surface-container-low', scheme.surfaceContainerLow],
    ['surface-container', scheme.surfaceContainer],
    ['surface-container-high', scheme.surfaceContainerHigh],
    ['surface-container-highest', scheme.surfaceContainerHighest],
    ['surface-tint', scheme.surfaceTint],
    ['inverse-surface', scheme.inverseSurface],
    ['inverse-primary', scheme.inversePrimary],
    ['shadow', scheme.shadow],
    ['scrim', scheme.scrim],
  ]
  for (const [k, v] of tokens) {
    root.setProperty(`--md-sys-color-${k}`, hexFromArgb(v))
  }
  // 同步 zinc 调色板（现有组件用 bg-zinc-* / text-zinc-* 自动获得 M3 配色）
  mapZincToScheme(scheme)
}

// 把 Tailwind 的 zinc 调色板映射到 M3 token，让现有组件无需改动即获得 M3 配色
function mapZincToScheme(scheme: Scheme) {
  const root = document.documentElement.style
  const hex = (v: number) => hexFromArgb(v)
  const map: Record<string, number> = {
    // 背景/表面
    '--color-zinc-950': scheme.surface,
    '--color-zinc-900': scheme.surfaceContainerHigh,
    '--color-zinc-800': scheme.surfaceContainerHighest,
    '--color-zinc-700': scheme.outlineVariant,
    // 边框/次要
    '--color-zinc-600': scheme.outline,
    '--color-zinc-500': scheme.onSurfaceVariant,
    '--color-zinc-400': scheme.onSurfaceVariant,
    '--color-zinc-300': scheme.outline,
    '--color-zinc-200': scheme.onSurfaceVariant,
    // 文字
    '--color-zinc-100': scheme.onSurface,
    '--color-zinc-50': scheme.onSurface,
    // 强调色 → primary / tertiary
    '--color-purple-500': scheme.primary,
    '--color-purple-600': scheme.primary,
    '--color-pink-500': scheme.tertiary,
    '--color-emerald-400': scheme.primary,
    '--color-emerald-500': scheme.primary,
    '--color-red-500': scheme.error,
  }
  for (const [k, v] of Object.entries(map)) {
    root.setProperty(k, hex(v))
  }
}
