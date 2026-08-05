// Material You 动态主题引擎（Monet 取色 + 多种取色算法）
import {
  SchemeTonalSpot,
  SchemeExpressive,
  SchemeVibrant,
  SchemeMonochrome,
  SchemeNeutral,
  SchemeFidelity,
  SchemeRainbow,
  SchemeFruitSalad,
  SchemeContent,
  Hct,
  hexFromArgb,
  argbFromHex,
  sourceColorFromImage,
  type DynamicScheme,
} from '@material/material-color-utilities'

export type ThemeStyle =
  | 'tonal-spot'
  | 'expressive'
  | 'vibrant'
  | 'monochrome'
  | 'neutral'
  | 'fidelity'
  | 'rainbow'
  | 'fruit-salad'
  | 'content'

export const THEME_STYLES: { id: ThemeStyle; name: string; desc: string }[] = [
  { id: 'tonal-spot', name: '经典', desc: 'M3 默认，色相取种子' },
  { id: 'expressive', name: '表现', desc: '多色相，更浓烈' },
  { id: 'vibrant', name: '活力', desc: '高饱和，鲜艳' },
  { id: 'monochrome', name: '单色', desc: '完全中性灰' },
  { id: 'neutral', name: '中性', desc: '柔和中性，少量色相' },
  { id: 'fidelity', name: '忠实', desc: '最贴近壁纸原色' },
  { id: 'rainbow', name: '彩虹', desc: '多色相渐变' },
  { id: 'fruit-salad', name: '果昔', desc: '多色相，柔和' },
  { id: 'content', name: '内容', desc: '最贴近内容与壁纸色调' },
]

export interface ThemeState {
  seed: string // 种子色（hex）
  mode: 'dark' | 'light'
  style: ThemeStyle // 取色算法
}

const LS_KEY = 'alice.theme.m3'
const DEFAULT_SEED = '#7c5cff'

export function getSavedTheme(): ThemeState {
  try {
    const raw = localStorage.getItem(LS_KEY)
    if (raw) {
      const t = JSON.parse(raw)
      if (t.seed && (t.mode === 'dark' || t.mode === 'light') && t.style) return t
    }
  } catch {}
  return { seed: DEFAULT_SEED, mode: 'dark', style: 'tonal-spot' }
}

export function saveTheme(state: ThemeState) {
  localStorage.setItem(LS_KEY, JSON.stringify(state))
}

// 按取色算法构建 M3 DynamicScheme（自带完整 surface 色阶）
// 注意：material-color-utilities 0.4 用构造函数（hct, isDark, contrastLevel）
function buildScheme(seedArgb: number, style: ThemeStyle, dark: boolean): DynamicScheme {
  const hct = Hct.fromInt(seedArgb)
  const S = {
    'tonal-spot': SchemeTonalSpot,
    expressive: SchemeExpressive,
    vibrant: SchemeVibrant,
    monochrome: SchemeMonochrome,
    neutral: SchemeNeutral,
    fidelity: SchemeFidelity,
    rainbow: SchemeRainbow,
    'fruit-salad': SchemeFruitSalad,
    content: SchemeContent,
  }[style]
  return new S(hct, dark, 0)
}

// 应用主题：seed + 取色算法 → 写入 CSS 变量
export function applyThemeFromSeed(seed: string, style: ThemeStyle, mode: 'dark' | 'light') {
  applyTheme(buildScheme(argbFromHex(seed), style, mode === 'dark'), mode)
}

// 从壁纸图片取色生成主题，返回提取的种子色
export async function applyThemeFromImage(
  image: string | HTMLImageElement,
  style: ThemeStyle,
  mode: 'dark' | 'light'
): Promise<string> {
  const seed = sourceColorFromImage(image)
  applyTheme(buildScheme(seed, style, mode === 'dark'), mode)
  return hexFromArgb(seed)
}

// DynamicScheme → CSS 变量
function applyTheme(scheme: DynamicScheme, mode: 'dark' | 'light') {
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
    // surface 5 层色阶（DynamicScheme 内置）
    ['surface-container-lowest', scheme.surfaceContainerLowest],
    ['surface-container-low', scheme.surfaceContainerLow],
    ['surface-container', scheme.surfaceContainer],
    ['surface-container-high', scheme.surfaceContainerHigh],
    ['surface-container-highest', scheme.surfaceContainerHighest],
    ['surface-tint', scheme.surfaceTint],
  ]
  for (const [k, v] of tokens) {
    root.setProperty(`--md-sys-color-${k}`, hexFromArgb(v))
  }
  mapZincToScheme(scheme, mode)
}

// 把 Tailwind 的 zinc 调色板映射到 M3 token，让现有组件无需改动即获得 M3 配色。
function mapZincToScheme(scheme: DynamicScheme, mode: 'dark' | 'light') {
  const root = document.documentElement.style
  const hex = (v: number) => hexFromArgb(v)
  const map: Record<string, number> = {
    // 背景/表面：5 层色阶（从深到浅）
    '--color-zinc-950': scheme.surfaceContainerLowest,
    '--color-zinc-900': scheme.surfaceContainerLow,
    '--color-zinc-800': scheme.surfaceContainer,
    '--color-zinc-700': scheme.surfaceContainerHigh,
    '--color-zinc-600': scheme.surfaceContainerHighest,
    // 边框 / 分割线
    '--color-zinc-500': scheme.outline,
    '--color-zinc-300': scheme.outlineVariant,
    // 文字（主/次两级）
    '--color-zinc-400': scheme.onSurfaceVariant,
    '--color-zinc-200': scheme.onSurfaceVariant,
    '--color-zinc-100': scheme.onSurface,
    '--color-zinc-50': scheme.onSurface,
    // 强调色
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
  void mode
}
