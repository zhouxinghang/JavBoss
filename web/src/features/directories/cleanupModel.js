import { zh } from '@/utils/i18n'

export const JUNK_CATEGORIES = [
  {
    id: 'system_junk',
    label: () => zh('系统垃圾', 'System Junk'),
    description: () =>
      zh('.DS_Store、Thumbs.db、desktop.ini、._* 等', '.DS_Store, Thumbs.db, desktop.ini, ._*'),
    badgeClass: 'bg-zinc-100 text-zinc-700',
  },
  {
    id: 'text_web',
    label: () => zh('文本和网页存档', 'Text & Web Archives'),
    description: () =>
      zh(
        '.txt、.html、.mhtml、.chm 等（不含 NFO/字幕）',
        '.txt, .html, .mhtml, .chm (excludes NFO/subtitles)'
      ),
    badgeClass: 'bg-sky-50 text-sky-700',
  },
  {
    id: 'torrent',
    label: () => zh('种子文件', 'Torrent Files'),
    description: () => zh('.torrent、.magnet、.metalink 等', '.torrent, .magnet, .metalink'),
    badgeClass: 'bg-emerald-50 text-emerald-700',
  },
  {
    id: 'windows_shortcut',
    label: () => zh('Windows 快捷方式', 'Windows Shortcuts'),
    description: () => zh('.lnk、.url、.website 等', '.lnk, .url, .website'),
    badgeClass: 'bg-indigo-50 text-indigo-700',
  },
  {
    id: 'suspected_ad_image',
    label: () => zh('疑似广告图片', 'Suspected Ad Images'),
    description: () =>
      zh(
        '广告词/域名图片、GIF、横幅图（已保护封面）',
        'Ad keywords/domains, GIFs, banners (covers protected)'
      ),
    badgeClass: 'bg-amber-50 text-amber-800',
  },
  {
    id: 'download_leftover',
    label: () => zh('下载残留', 'Download Leftovers'),
    description: () =>
      zh('.!qb、.part、.aria2、.xltd、padding 文件等', '.!qb, .part, .aria2, .xltd, padding files'),
    badgeClass: 'bg-rose-50 text-rose-700',
  },
]

export const ALL_JUNK_CATEGORY_IDS = JUNK_CATEGORIES.map((item) => item.id)

const JUNK_CATEGORY_MAP = new Map(JUNK_CATEGORIES.map((item) => [item.id, item]))

export function getJunkCategoryMeta(category) {
  return (
    JUNK_CATEGORY_MAP.get(category) || {
      id: category,
      label: () => category,
      description: () => '',
      badgeClass: 'bg-zinc-100 text-zinc-700',
    }
  )
}

export function filterJunkItems(items, { categoryFilter = 'all', search = '' } = {}) {
  const list = Array.isArray(items) ? items : []
  const query = String(search || '')
    .trim()
    .toLowerCase()
  return list.filter((item) => {
    if (categoryFilter && categoryFilter !== 'all' && item.category !== categoryFilter) {
      return false
    }
    if (!query) return true
    const haystack = [item.filename, item.relative_path, item.reason_zh, item.reason_en]
      .filter(Boolean)
      .join('\n')
      .toLowerCase()
    return haystack.includes(query)
  })
}

export function filterAndSortDuplicateGroups(
  groups,
  { search = '', hideMultiPart = false, sortBy = 'count' } = {}
) {
  const list = Array.isArray(groups) ? groups : []
  const query = String(search || '')
    .trim()
    .toLowerCase()
  const filtered = list.filter((group) => {
    if (hideMultiPart && group.has_multi_part) {
      return false
    }
    if (!query) return true
    if (
      String(group.code || '')
        .toLowerCase()
        .includes(query) ||
      String(group.title || '')
        .toLowerCase()
        .includes(query)
    ) {
      return true
    }
    return (group.files || []).some(
      (file) =>
        String(file.filename || '')
          .toLowerCase()
          .includes(query) ||
        String(file.relative_path || '')
          .toLowerCase()
          .includes(query)
    )
  })

  return [...filtered].sort((a, b) => {
    if (sortBy === 'reclaimable') {
      if ((b.reclaimable_size || 0) !== (a.reclaimable_size || 0)) {
        return (b.reclaimable_size || 0) - (a.reclaimable_size || 0)
      }
      if ((b.count || 0) !== (a.count || 0)) {
        return (b.count || 0) - (a.count || 0)
      }
      return String(a.code || '').localeCompare(String(b.code || ''))
    }
    if (sortBy === 'code') {
      return String(a.code || '').localeCompare(String(b.code || ''))
    }
    if ((b.count || 0) !== (a.count || 0)) {
      return (b.count || 0) - (a.count || 0)
    }
    if ((b.reclaimable_size || 0) !== (a.reclaimable_size || 0)) {
      return (b.reclaimable_size || 0) - (a.reclaimable_size || 0)
    }
    return String(a.code || '').localeCompare(String(b.code || ''))
  })
}

function compareFilesForKeep(a, b, strategy) {
  if (strategy === 'newest') {
    if ((b.modified_at_unix_ms || 0) !== (a.modified_at_unix_ms || 0)) {
      return (b.modified_at_unix_ms || 0) - (a.modified_at_unix_ms || 0)
    }
    if ((b.size || 0) !== (a.size || 0)) {
      return (b.size || 0) - (a.size || 0)
    }
  } else {
    if ((b.size || 0) !== (a.size || 0)) {
      return (b.size || 0) - (a.size || 0)
    }
    if ((b.modified_at_unix_ms || 0) !== (a.modified_at_unix_ms || 0)) {
      return (b.modified_at_unix_ms || 0) - (a.modified_at_unix_ms || 0)
    }
  }
  return String(a.relative_path || '').localeCompare(String(b.relative_path || ''))
}

export function selectGroupDuplicatePathsKeepingBest(group, strategy = 'largest') {
  const files = Array.isArray(group?.files) ? group.files : []
  if (files.length < 2) return []
  const sorted = [...files].sort((a, b) => compareFilesForKeep(a, b, strategy))
  return sorted.slice(1).map((file) => file.relative_path)
}

export function selectDuplicatePathsKeepingBest(groups, strategy = 'largest') {
  const list = Array.isArray(groups) ? groups : []
  const paths = []
  let skippedMultiPartCount = 0
  for (const group of list) {
    if (group.has_multi_part) {
      skippedMultiPartCount += 1
      continue
    }
    paths.push(...selectGroupDuplicatePathsKeepingBest(group, strategy))
  }
  return { paths, skippedMultiPartCount }
}

export function findFullySelectedDuplicateGroups(groups, selectedPaths) {
  const selected = selectedPaths instanceof Set ? selectedPaths : new Set(selectedPaths || [])
  const codes = []
  for (const group of groups || []) {
    const files = group?.files || []
    if (files.length > 0 && files.every((file) => selected.has(file.relative_path))) {
      codes.push(group.code)
    }
  }
  return codes
}

export function duplicateFingerprintCounts(group) {
  const counts = new Map()
  for (const file of group?.files || []) {
    const fp = String(file?.fingerprint || '').trim()
    if (!fp) continue
    counts.set(fp, (counts.get(fp) || 0) + 1)
  }
  return counts
}

export function applyDeletedPathsToScanResult(scanResult, deletedPaths) {
  if (!scanResult) return null
  const deleted = new Set(deletedPaths || [])
  if (deleted.size === 0) return scanResult

  const remainingJunk = (scanResult.junk_items || []).filter(
    (item) => !deleted.has(item.relative_path)
  )
  const summaryMap = new Map()
  const selectedCategories =
    Array.isArray(scanResult.selected_categories) && scanResult.selected_categories.length > 0
      ? scanResult.selected_categories
      : (scanResult.category_summaries || []).map((entry) => entry.category)
  const categorySummaries = selectedCategories.map((category) => {
    const summary = { category, count: 0, total_size: 0 }
    summaryMap.set(category, summary)
    return summary
  })
  let junkTotalSize = 0
  for (const item of remainingJunk) {
    const size = Number(item.size) || 0
    junkTotalSize += size
    const summary = summaryMap.get(item.category)
    if (summary) {
      summary.count += 1
      summary.total_size += size
    }
  }

  const remainingGroups = []
  let duplicateFileCount = 0
  let duplicateTotalSize = 0
  let duplicateReclaimableSize = 0
  for (const group of scanResult.duplicate_groups || []) {
    const files = (group.files || []).filter((file) => !deleted.has(file.relative_path))
    if (files.length < 2) continue
    let totalSize = 0
    let maxSize = 0
    for (const file of files) {
      const size = Number(file.size) || 0
      totalSize += size
      if (size > maxSize) maxSize = size
    }
    const reclaimableSize = Math.max(0, totalSize - maxSize)
    remainingGroups.push({
      ...group,
      count: files.length,
      total_size: totalSize,
      reclaimable_size: reclaimableSize,
      files,
    })
    duplicateFileCount += files.length
    duplicateTotalSize += totalSize
    duplicateReclaimableSize += reclaimableSize
  }

  return {
    ...scanResult,
    junk_total_count: remainingJunk.length,
    junk_total_size: junkTotalSize,
    category_summaries: categorySummaries,
    junk_items: remainingJunk,
    duplicate_group_count: remainingGroups.length,
    duplicate_file_count: duplicateFileCount,
    duplicate_total_size: duplicateTotalSize,
    duplicate_reclaimable_size: duplicateReclaimableSize,
    duplicate_groups: remainingGroups,
  }
}

export function formatCleanupBytes(bytes) {
  const size = Number(bytes)
  if (!Number.isFinite(size) || size <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let val = size
  let idx = 0
  while (val >= 1024 && idx < units.length - 1) {
    val /= 1024
    idx += 1
  }
  const rounded = val >= 10 || idx === 0 ? Math.round(val) : Math.round(val * 10) / 10
  return `${rounded} ${units[idx]}`
}

export function formatCleanupDuration(durationSec) {
  const sec = Number(durationSec)
  if (!Number.isFinite(sec) || sec <= 0) return ''
  const totalMinutes = Math.round(sec / 60)
  if (totalMinutes < 1) return zh(`${Math.round(sec)} 秒`, `${Math.round(sec)}s`)
  return zh(`${totalMinutes} 分钟`, `${totalMinutes} min`)
}
