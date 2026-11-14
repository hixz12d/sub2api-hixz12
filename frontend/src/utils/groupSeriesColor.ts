/**
 * 分组识别色：和用户仪表盘（UserDashboardView）的分组色规则一致。
 * 在 orderedIds 里的分组按位置取色，不在的按 abs(id) 取色，共 5 种循环。
 */

const SERIES_CLASSES = ['bg-series-1', 'bg-series-2', 'bg-series-3', 'bg-series-4', 'bg-series-5'] as const

export function groupSeriesClass(groupId: number, orderedIds?: number[]): string {
  const idx = orderedIds ? orderedIds.indexOf(groupId) : -1
  return SERIES_CLASSES[(idx >= 0 ? idx : Math.abs(groupId)) % SERIES_CLASSES.length]
}
