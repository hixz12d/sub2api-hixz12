// 界面风格：经典（上游原版）/ 新版（改版后）。
// 每个浏览器自己记住选择，默认新版；切换时整页刷新，不做运行中热切换。
export type UiStyle = 'classic' | 'modern'

export const UI_STYLE_STORAGE_KEY = 'ui-style'

export function getUiStyle(): UiStyle {
  try {
    return localStorage.getItem(UI_STYLE_STORAGE_KEY) === 'classic' ? 'classic' : 'modern'
  } catch {
    // 隐私模式等情况下读取 localStorage 会抛异常，按默认新版处理
    return 'modern'
  }
}

export function isClassicUi(): boolean {
  return getUiStyle() === 'classic'
}

// 在 app.mount 之前调用，给 <html> 打上 data-ui 标记，全局样式只认这个属性
export function applyUiStyleAttr(): void {
  document.documentElement.dataset.ui = getUiStyle()
}

export function setUiStyle(style: UiStyle): void {
  try {
    localStorage.setItem(UI_STYLE_STORAGE_KEY, style)
  } catch {
    // 写入失败时刷新后仍是原风格，不额外提示
  }
  window.location.reload()
}
