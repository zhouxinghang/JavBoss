import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react'
import CleaningServicesRoundedIcon from '@mui/icons-material/CleaningServicesRounded'
import CloseRoundedIcon from '@mui/icons-material/CloseRounded'
import ContentCopyRoundedIcon from '@mui/icons-material/ContentCopyRounded'
import DeleteOutlineRoundedIcon from '@mui/icons-material/DeleteOutlineRounded'
import ImageOutlinedIcon from '@mui/icons-material/ImageOutlined'
import RefreshRoundedIcon from '@mui/icons-material/RefreshRounded'
import SearchRoundedIcon from '@mui/icons-material/SearchRounded'
import WarningAmberRoundedIcon from '@mui/icons-material/WarningAmberRounded'
import { CircularProgress, IconButton, Tooltip } from '@mui/material'

import {
  deleteDirectoryCleanupFiles,
  directoryCleanupPreviewUrl,
  scanDirectoryCleanup,
} from '@/features/directories/api'
import {
  ALL_JUNK_CATEGORY_IDS,
  JUNK_CATEGORIES,
  applyDeletedPathsToScanResult,
  duplicateFingerprintCounts,
  filterAndSortDuplicateGroups,
  filterJunkItems,
  findFullySelectedDuplicateGroups,
  formatCleanupBytes,
  formatCleanupDuration,
  getJunkCategoryMeta,
  selectDuplicatePathsKeepingBest,
  selectGroupDuplicatePathsKeepingBest,
} from '@/features/directories/cleanupModel'
import AppModal from '@/shared/ui/AppModal'
import { useStore } from '@/store'
import { getErrorMessage } from '@/utils/errors'
import { displayHostPath, hostPathsEnabled } from '@/utils/hostPath'
import { zh } from '@/utils/i18n'

const TAB_JUNK = 'junk'
const TAB_DUPLICATES = 'duplicates'

const formatModifiedDate = (unixMS) => {
  const value = Number(unixMS)
  if (!Number.isFinite(value) || value <= 0) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return new Intl.DateTimeFormat(zh('zh-CN', 'en-US'), {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(date)
}

const sameStringSet = (a, b) => {
  if (a.length !== b.length) return false
  const setA = new Set(a)
  return b.every((item) => setA.has(item))
}

export default function DirectoryCleanupModal({
  directory,
  serverOS = '',
  onClose,
  onRefresh,
  onToast,
}) {
  const useHostPaths = useStore((state) => hostPathsEnabled(state.config))
  const titleId = useId()
  const confirmTitleId = useId()
  const previewTitleId = useId()

  const [activeTab, setActiveTab] = useState(TAB_JUNK)
  const [selectedCategories, setSelectedCategories] = useState(() => [...ALL_JUNK_CATEGORY_IDS])
  const [scanResult, setScanResult] = useState(null)
  const [scanning, setScanning] = useState(false)
  const [scanError, setScanError] = useState('')

  const [categoryFilter, setCategoryFilter] = useState('all')
  const [junkSearch, setJunkSearch] = useState('')
  const [selectedJunkPaths, setSelectedJunkPaths] = useState(() => new Set())

  const [duplicateSearch, setDuplicateSearch] = useState('')
  const [hideMultiPart, setHideMultiPart] = useState(false)
  const [duplicateSort, setDuplicateSort] = useState('count')
  const [selectedDuplicatePaths, setSelectedDuplicatePaths] = useState(() => new Set())
  const [quickSelectNote, setQuickSelectNote] = useState('')

  const [removeEmptyDirs, setRemoveEmptyDirs] = useState(false)
  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [deleteError, setDeleteError] = useState('')
  const [deleteFeedback, setDeleteFeedback] = useState(null)
  const [previewItem, setPreviewItem] = useState(null)

  const abortRef = useRef(null)

  const runScan = useCallback(
    async ({ categories = ALL_JUNK_CATEGORY_IDS, includeDuplicates = true } = {}) => {
      if (!directory?.id) return
      if (abortRef.current) {
        abortRef.current.abort()
      }
      const controller = new AbortController()
      abortRef.current = controller

      setScanning(true)
      setScanError('')
      setDeleteError('')
      setQuickSelectNote('')
      try {
        const result = await scanDirectoryCleanup(directory.id, {
          categories,
          includeDuplicates,
          signal: controller.signal,
        })
        if (controller.signal.aborted) return
        setScanResult(result)
        setSelectedJunkPaths(new Set())
        setSelectedDuplicatePaths(new Set())
        setCategoryFilter((prev) =>
          prev === 'all' || (result.selected_categories || []).includes(prev) ? prev : 'all'
        )
      } catch (err) {
        if (controller.signal.aborted) return
        setScanError(getErrorMessage(err))
      } finally {
        if (abortRef.current === controller) {
          abortRef.current = null
        }
        if (!controller.signal.aborted) {
          setScanning(false)
        }
      }
    },
    [directory?.id]
  )

  useEffect(() => {
    runScan({ categories: [...ALL_JUNK_CATEGORY_IDS], includeDuplicates: true })
    return () => {
      if (abortRef.current) {
        abortRef.current.abort()
        abortRef.current = null
      }
    }
  }, [runScan])

  const scannedCategories = useMemo(
    () => scanResult?.selected_categories || [],
    [scanResult?.selected_categories]
  )
  const categoriesDirty = useMemo(
    () => Boolean(scanResult) && !sameStringSet(selectedCategories, scannedCategories),
    [scanResult, scannedCategories, selectedCategories]
  )

  const categorySummaryMap = useMemo(() => {
    const map = new Map()
    for (const entry of scanResult?.category_summaries || []) {
      map.set(entry.category, entry)
    }
    return map
  }, [scanResult?.category_summaries])

  const junkItems = useMemo(() => scanResult?.junk_items || [], [scanResult?.junk_items])
  const visibleJunkItems = useMemo(
    () => filterJunkItems(junkItems, { categoryFilter, search: junkSearch }),
    [categoryFilter, junkItems, junkSearch]
  )

  const duplicateGroups = useMemo(
    () => scanResult?.duplicate_groups || [],
    [scanResult?.duplicate_groups]
  )
  const multiPartGroupCount = useMemo(
    () => duplicateGroups.filter((group) => group.has_multi_part).length,
    [duplicateGroups]
  )
  const visibleDuplicateGroups = useMemo(
    () =>
      filterAndSortDuplicateGroups(duplicateGroups, {
        search: duplicateSearch,
        hideMultiPart,
        sortBy: duplicateSort,
      }),
    [duplicateGroups, duplicateSearch, duplicateSort, hideMultiPart]
  )

  const junkSizeByPath = useMemo(() => {
    const map = new Map()
    for (const item of junkItems) {
      map.set(item.relative_path, Number(item.size) || 0)
    }
    return map
  }, [junkItems])

  const duplicateSizeByPath = useMemo(() => {
    const map = new Map()
    for (const group of duplicateGroups) {
      for (const file of group.files || []) {
        map.set(file.relative_path, Number(file.size) || 0)
      }
    }
    return map
  }, [duplicateGroups])

  const activeSelectedPaths = activeTab === TAB_JUNK ? selectedJunkPaths : selectedDuplicatePaths
  const otherTabSelectedCount =
    activeTab === TAB_JUNK ? selectedDuplicatePaths.size : selectedJunkPaths.size

  const activeSelectedSize = useMemo(() => {
    const sizeMap = activeTab === TAB_JUNK ? junkSizeByPath : duplicateSizeByPath
    let total = 0
    for (const path of activeSelectedPaths) {
      total += sizeMap.get(path) || 0
    }
    return total
  }, [activeSelectedPaths, activeTab, duplicateSizeByPath, junkSizeByPath])

  const fullySelectedDuplicateCodes = useMemo(
    () =>
      activeTab === TAB_DUPLICATES
        ? findFullySelectedDuplicateGroups(duplicateGroups, selectedDuplicatePaths)
        : [],
    [activeTab, duplicateGroups, selectedDuplicatePaths]
  )

  const allVisibleJunkSelected =
    visibleJunkItems.length > 0 &&
    visibleJunkItems.every((item) => selectedJunkPaths.has(item.relative_path))

  const toggleCategory = (categoryId) => {
    setSelectedCategories((prev) =>
      prev.includes(categoryId) ? prev.filter((id) => id !== categoryId) : [...prev, categoryId]
    )
  }

  const toggleJunkPath = (relativePath) => {
    setSelectedJunkPaths((prev) => {
      const next = new Set(prev)
      if (next.has(relativePath)) next.delete(relativePath)
      else next.add(relativePath)
      return next
    })
  }

  const toggleSelectAllVisibleJunk = () => {
    setSelectedJunkPaths((prev) => {
      const next = new Set(prev)
      if (allVisibleJunkSelected) {
        for (const item of visibleJunkItems) {
          next.delete(item.relative_path)
        }
      } else {
        for (const item of visibleJunkItems) {
          next.add(item.relative_path)
        }
      }
      return next
    })
  }

  const toggleDuplicatePath = (relativePath) => {
    setQuickSelectNote('')
    setSelectedDuplicatePaths((prev) => {
      const next = new Set(prev)
      if (next.has(relativePath)) next.delete(relativePath)
      else next.add(relativePath)
      return next
    })
  }

  const applyQuickSelectDuplicates = (strategy) => {
    const { paths, skippedMultiPartCount } = selectDuplicatePathsKeepingBest(
      visibleDuplicateGroups,
      strategy
    )
    setSelectedDuplicatePaths(new Set(paths))
    if (skippedMultiPartCount > 0) {
      setQuickSelectNote(
        zh(
          `已自动跳过 ${skippedMultiPartCount} 组疑似分段文件（CD1/CD2 等），请按需手动勾选。`,
          `Skipped ${skippedMultiPartCount} suspected multi-part group(s) (CD1/CD2, etc.); review them manually.`
        )
      )
    } else {
      setQuickSelectNote('')
    }
  }

  const applyGroupKeepLargest = (group) => {
    setQuickSelectNote('')
    const toDelete = selectGroupDuplicatePathsKeepingBest(group, 'largest')
    setSelectedDuplicatePaths((prev) => {
      const next = new Set(prev)
      for (const file of group.files || []) {
        next.delete(file.relative_path)
      }
      for (const path of toDelete) {
        next.add(path)
      }
      return next
    })
  }

  const clearGroupSelection = (group) => {
    setSelectedDuplicatePaths((prev) => {
      const next = new Set(prev)
      for (const file of group.files || []) {
        next.delete(file.relative_path)
      }
      return next
    })
  }

  const handleRescan = () => {
    if (activeTab === TAB_JUNK && selectedCategories.length === 0) {
      setScanError(zh('请至少选择一个垃圾文件类别', 'Select at least one junk category to scan'))
      return
    }
    setDeleteFeedback(null)
    runScan({
      categories: selectedCategories,
      includeDuplicates: true,
    })
  }

  const handleConfirmDelete = async () => {
    if (!directory?.id || activeSelectedPaths.size === 0) return
    const targetPaths = Array.from(activeSelectedPaths)
    setDeleting(true)
    setDeleteError('')
    try {
      const res = await deleteDirectoryCleanupFiles(directory.id, {
        paths: targetPaths,
        removeEmptyDirs,
      })
      const deletedSet = new Set(res.deleted_paths || [])
      setScanResult((prev) => applyDeletedPathsToScanResult(prev, res.deleted_paths || []))
      setSelectedJunkPaths((prev) => {
        const next = new Set(prev)
        for (const p of deletedSet) next.delete(p)
        return next
      })
      setSelectedDuplicatePaths((prev) => {
        const next = new Set(prev)
        for (const p of deletedSet) next.delete(p)
        return next
      })
      if (previewItem && deletedSet.has(previewItem.relative_path)) {
        setPreviewItem(null)
      }
      setConfirmDeleteOpen(false)
      setDeleteFeedback(res)
      if (res.deleted_count > 0) {
        onToast?.(
          zh(
            `已清理 ${res.deleted_count} 个文件，释放 ${formatCleanupBytes(res.deleted_size)}`,
            `Cleaned ${res.deleted_count} file(s), freed ${formatCleanupBytes(res.deleted_size)}`
          ),
          4000
        )
        await onRefresh?.()
      }
    } catch (err) {
      setDeleteError(getErrorMessage(err))
    } finally {
      setDeleting(false)
    }
  }

  const displayDirPath = displayHostPath(directory?.path, useHostPaths)
  const isWindows = serverOS === 'windows'

  return (
    <>
      <AppModal
        ariaLabelledby={titleId}
        className="p-3 sm:p-5"
        closeDisabled={deleting || confirmDeleteOpen || Boolean(previewItem)}
        contentClassName="flex h-[84dvh] max-h-[840px] w-full max-w-5xl flex-col overflow-hidden rounded-2xl bg-white shadow-xl"
        onClose={onClose}
        zIndex={1420}
      >
        {/* Header */}
        <div className="flex shrink-0 items-start justify-between gap-3 border-b border-zinc-200 px-5 py-4">
          <div className="min-w-0">
            <h2
              id={titleId}
              className="flex items-center gap-2 text-base font-semibold text-zinc-900"
            >
              <CleaningServicesRoundedIcon className="text-blue-600" fontSize="small" />
              <span>{zh('垃圾文件清理', 'Junk File Cleanup')}</span>
            </h2>
            <p className="mt-1 truncate text-xs text-zinc-500" title={displayDirPath}>
              {displayDirPath}
            </p>
          </div>
          <Tooltip title={zh('关闭', 'Close')} arrow>
            <span>
              <IconButton
                type="button"
                size="small"
                aria-label={zh('关闭', 'Close')}
                disabled={deleting}
                onClick={onClose}
              >
                <CloseRoundedIcon fontSize="small" />
              </IconButton>
            </span>
          </Tooltip>
        </div>

        {/* Mode Tabs */}
        <div className="flex shrink-0 flex-wrap items-center justify-between gap-3 border-b border-zinc-200 bg-zinc-50/70 px-5 py-2.5">
          <div className="inline-flex rounded-xl bg-zinc-200/70 p-1" role="tablist">
            <button
              type="button"
              role="tab"
              aria-selected={activeTab === TAB_JUNK}
              onClick={() => {
                setActiveTab(TAB_JUNK)
                setDeleteError('')
              }}
              className={`flex items-center gap-2 rounded-lg px-3.5 py-1.5 text-sm font-medium transition ${
                activeTab === TAB_JUNK
                  ? 'bg-white text-zinc-900 shadow-sm'
                  : 'text-zinc-600 hover:text-zinc-900'
              }`}
            >
              <span>{zh('垃圾文件扫描', 'Junk Files')}</span>
              {scanResult && (
                <span className="rounded-full bg-zinc-100 px-2 py-0.5 text-xs tabular-nums text-zinc-700">
                  {scanResult.junk_total_count}
                </span>
              )}
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={activeTab === TAB_DUPLICATES}
              onClick={() => {
                setActiveTab(TAB_DUPLICATES)
                setDeleteError('')
              }}
              className={`flex items-center gap-2 rounded-lg px-3.5 py-1.5 text-sm font-medium transition ${
                activeTab === TAB_DUPLICATES
                  ? 'bg-white text-zinc-900 shadow-sm'
                  : 'text-zinc-600 hover:text-zinc-900'
              }`}
            >
              <ContentCopyRoundedIcon sx={{ fontSize: 15 }} />
              <span>{zh('重复文件（按番号）', 'Duplicate Files (by Code)')}</span>
              {scanResult && (
                <span className="rounded-full bg-zinc-100 px-2 py-0.5 text-xs tabular-nums text-zinc-700">
                  {scanResult.duplicate_group_count}
                </span>
              )}
            </button>
          </div>

          <div className="flex items-center gap-2">
            {scanResult && !scanning && (
              <span className="text-xs tabular-nums text-zinc-500">
                {zh(
                  `已遍历 ${scanResult.scanned_files} 个文件`,
                  `Scanned ${scanResult.scanned_files} files`
                )}
              </span>
            )}
            <button
              type="button"
              onClick={handleRescan}
              disabled={scanning || deleting}
              className={`inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium transition disabled:opacity-60 ${
                categoriesDirty && activeTab === TAB_JUNK
                  ? 'bg-blue-600 text-white hover:bg-blue-700'
                  : 'border border-zinc-300 bg-white text-zinc-700 hover:bg-zinc-100'
              }`}
            >
              {scanning ? (
                <CircularProgress size={14} color="inherit" />
              ) : (
                <RefreshRoundedIcon sx={{ fontSize: 16 }} />
              )}
              <span>
                {scanning
                  ? zh('扫描中…', 'Scanning...')
                  : categoriesDirty && activeTab === TAB_JUNK
                    ? zh('按所选类别扫描', 'Scan Selected Categories')
                    : zh('重新扫描', 'Rescan')}
              </span>
            </button>
          </div>
        </div>

        {/* Feedback / Error banners */}
        {(scanError || deleteError || deleteFeedback) && (
          <div className="shrink-0 space-y-2 border-b border-zinc-200 bg-white px-5 py-2.5">
            {scanError && (
              <div role="alert" className="rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700">
                {scanError}
              </div>
            )}
            {deleteError && (
              <div role="alert" className="rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700">
                {deleteError}
              </div>
            )}
            {deleteFeedback && (
              <div className="rounded-lg bg-emerald-50 px-3 py-2 text-xs text-emerald-800">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span>
                    {zh(
                      `清理完成：已删除 ${deleteFeedback.deleted_count} 个文件（释放 ${formatCleanupBytes(deleteFeedback.deleted_size)}）`,
                      `Cleanup complete: removed ${deleteFeedback.deleted_count} file(s) (freed ${formatCleanupBytes(deleteFeedback.deleted_size)})`
                    )}
                    {deleteFeedback.empty_directories_removed > 0 &&
                      zh(
                        `，移除 ${deleteFeedback.empty_directories_removed} 个空目录`,
                        `, removed ${deleteFeedback.empty_directories_removed} empty folder(s)`
                      )}
                    {deleteFeedback.failed?.length > 0 &&
                      zh(
                        `，${deleteFeedback.failed.length} 个失败`,
                        `, ${deleteFeedback.failed.length} failed`
                      )}
                    {zh('。', '.')}
                  </span>
                  <button
                    type="button"
                    onClick={() => setDeleteFeedback(null)}
                    className="text-emerald-700 underline hover:text-emerald-900"
                  >
                    {zh('知道了', 'Dismiss')}
                  </button>
                </div>
                {deleteFeedback.failed?.length > 0 && (
                  <ul className="mt-1.5 list-disc space-y-0.5 pl-4 text-red-700">
                    {deleteFeedback.failed.slice(0, 5).map((item) => (
                      <li key={item.relative_path}>
                        <span className="font-mono">{item.relative_path}</span>:{' '}
                        {zh(item.reason_zh, item.reason_en)}
                      </li>
                    ))}
                    {deleteFeedback.failed.length > 5 && (
                      <li>
                        {zh(
                          `另有 ${deleteFeedback.failed.length - 5} 项失败`,
                          `and ${deleteFeedback.failed.length - 5} more failed`
                        )}
                      </li>
                    )}
                  </ul>
                )}
              </div>
            )}
          </div>
        )}

        {/* Body */}
        {activeTab === TAB_JUNK ? (
          <div className="flex min-h-0 flex-1 flex-col">
            {/* Category picker */}
            <div className="shrink-0 border-b border-zinc-200 px-5 py-3">
              <div className="mb-2 flex items-center justify-between gap-2">
                <span className="text-xs font-medium text-zinc-700">
                  {zh('选择扫描类别', 'Scan categories')}
                </span>
                <div className="flex items-center gap-2 text-xs">
                  <button
                    type="button"
                    onClick={() => setSelectedCategories([...ALL_JUNK_CATEGORY_IDS])}
                    disabled={scanning || deleting}
                    className="text-blue-600 hover:underline disabled:opacity-50"
                  >
                    {zh('全选', 'Select all')}
                  </button>
                  <span className="text-zinc-300">|</span>
                  <button
                    type="button"
                    onClick={() => setSelectedCategories([])}
                    disabled={scanning || deleting}
                    className="text-zinc-500 hover:text-zinc-800 hover:underline disabled:opacity-50"
                  >
                    {zh('清空', 'Clear')}
                  </button>
                </div>
              </div>
              <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
                {JUNK_CATEGORIES.map((category) => {
                  const checked = selectedCategories.includes(category.id)
                  const summary = categorySummaryMap.get(category.id)
                  return (
                    <label
                      key={category.id}
                      aria-label={category.label()}
                      className={`flex cursor-pointer items-start gap-2.5 rounded-xl border px-3 py-2 transition ${
                        checked
                          ? 'border-blue-300 bg-blue-50/50'
                          : 'border-zinc-200 bg-white hover:bg-zinc-50'
                      }`}
                    >
                      <input
                        type="checkbox"
                        checked={checked}
                        disabled={scanning || deleting}
                        onChange={() => toggleCategory(category.id)}
                        className="mt-0.5 h-4 w-4 rounded"
                      />
                      <span className="min-w-0 flex-1">
                        <span className="flex items-center justify-between gap-2">
                          <span className="truncate text-xs font-medium text-zinc-900">
                            {category.label()}
                          </span>
                          {summary && (
                            <span className="shrink-0 text-[11px] tabular-nums text-zinc-500">
                              {summary.count} · {formatCleanupBytes(summary.total_size)}
                            </span>
                          )}
                        </span>
                        <span className="mt-0.5 block truncate text-[11px] text-zinc-500">
                          {category.description()}
                        </span>
                      </span>
                    </label>
                  )
                })}
              </div>
            </div>

            {/* Junk filter & selection toolbar */}
            <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b border-zinc-200 bg-zinc-50/60 px-5 py-2.5">
              <div className="flex flex-wrap items-center gap-1.5">
                <button
                  type="button"
                  onClick={() => setCategoryFilter('all')}
                  className={`rounded-full px-2.5 py-1 text-xs font-medium transition ${
                    categoryFilter === 'all'
                      ? 'bg-zinc-900 text-white'
                      : 'bg-white text-zinc-600 ring-1 ring-zinc-200 hover:bg-zinc-100'
                  }`}
                >
                  {zh('全部', 'All')}{' '}
                  <span className="tabular-nums">({scanResult?.junk_total_count || 0})</span>
                </button>
                {scannedCategories.map((catId) => {
                  const meta = getJunkCategoryMeta(catId)
                  const count = categorySummaryMap.get(catId)?.count || 0
                  const active = categoryFilter === catId
                  return (
                    <button
                      key={catId}
                      type="button"
                      onClick={() => setCategoryFilter(catId)}
                      className={`rounded-full px-2.5 py-1 text-xs font-medium transition ${
                        active
                          ? 'bg-blue-600 text-white'
                          : 'bg-white text-zinc-600 ring-1 ring-zinc-200 hover:bg-zinc-100'
                      }`}
                    >
                      {meta.label()} <span className="tabular-nums">({count})</span>
                    </button>
                  )
                })}
              </div>
              <div className="flex items-center gap-2">
                <div className="relative">
                  <SearchRoundedIcon
                    className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-zinc-400"
                    sx={{ fontSize: 16 }}
                  />
                  <input
                    type="search"
                    value={junkSearch}
                    onChange={(event) => setJunkSearch(event.target.value)}
                    placeholder={zh('筛选文件名或路径…', 'Filter filename or path...')}
                    className="w-48 rounded-lg border border-zinc-300 bg-white py-1 pl-8 pr-2.5 text-xs outline-none focus:border-blue-500 sm:w-56"
                  />
                </div>
                <button
                  type="button"
                  onClick={toggleSelectAllVisibleJunk}
                  disabled={visibleJunkItems.length === 0 || deleting}
                  className="rounded-lg border border-zinc-300 bg-white px-2.5 py-1 text-xs font-medium text-zinc-700 hover:bg-zinc-100 disabled:opacity-50"
                >
                  {allVisibleJunkSelected
                    ? zh('取消全选当前列表', 'Deselect Visible')
                    : zh(
                        `全选当前列表 (${visibleJunkItems.length})`,
                        `Select Visible (${visibleJunkItems.length})`
                      )}
                </button>
              </div>
            </div>

            {/* Junk list */}
            <div className="min-h-0 flex-1 overflow-y-auto px-5 py-3">
              {scanning && !scanResult ? (
                <div className="flex h-full flex-col items-center justify-center gap-3 text-sm text-zinc-500">
                  <CircularProgress size={24} />
                  <span>{zh('正在扫描目录垃圾文件…', 'Scanning directory for junk files...')}</span>
                </div>
              ) : visibleJunkItems.length === 0 ? (
                <div className="flex h-full flex-col items-center justify-center gap-2 text-center text-sm text-zinc-500">
                  <CleaningServicesRoundedIcon className="text-zinc-300" sx={{ fontSize: 36 }} />
                  <p>
                    {junkItems.length === 0
                      ? zh(
                          '当前所选类别下未发现垃圾文件。',
                          'No junk files found for the selected categories.'
                        )
                      : zh('没有符合当前筛选条件的文件。', 'No files match the current filter.')}
                  </p>
                </div>
              ) : (
                <ul className="divide-y divide-zinc-100 rounded-xl border border-zinc-200">
                  {visibleJunkItems.map((item) => {
                    const checked = selectedJunkPaths.has(item.relative_path)
                    const catMeta = getJunkCategoryMeta(item.category)
                    const previewUrl = item.previewable
                      ? directoryCleanupPreviewUrl(directory.id, item.relative_path)
                      : ''
                    return (
                      <li
                        key={item.relative_path}
                        className={`flex items-center gap-3 px-3 py-2.5 transition ${
                          checked ? 'bg-red-50/40' : 'hover:bg-zinc-50'
                        }`}
                      >
                        <input
                          type="checkbox"
                          checked={checked}
                          disabled={deleting}
                          aria-label={zh(
                            `选择 ${item.relative_path}`,
                            `Select ${item.relative_path}`
                          )}
                          onChange={() => toggleJunkPath(item.relative_path)}
                          className="h-4 w-4 shrink-0 rounded"
                        />
                        {item.previewable && (
                          <button
                            type="button"
                            onClick={() => setPreviewItem(item)}
                            title={zh('点击查看大图', 'Click to preview image')}
                            className="group relative flex h-11 w-14 shrink-0 items-center justify-center overflow-hidden rounded-lg border border-zinc-200 bg-zinc-100 hover:border-blue-400"
                          >
                            <img
                              src={previewUrl}
                              alt={item.filename}
                              loading="lazy"
                              className="h-full w-full object-contain"
                            />
                          </button>
                        )}
                        <div className="min-w-0 flex-1">
                          <div className="flex flex-wrap items-center gap-1.5">
                            <span className="truncate text-sm font-medium text-zinc-900">
                              {item.filename}
                            </span>
                            <span
                              className={`inline-flex rounded-full px-2 py-0.5 text-[11px] font-medium ${catMeta.badgeClass}`}
                            >
                              {catMeta.label()}
                            </span>
                            <span className="text-xs text-zinc-500">
                              {zh(item.reason_zh, item.reason_en)}
                            </span>
                          </div>
                          <div
                            className="mt-0.5 truncate font-mono text-xs text-zinc-500"
                            title={item.relative_path}
                          >
                            {item.relative_path}
                          </div>
                        </div>
                        <div className="shrink-0 text-right">
                          <div className="text-xs font-medium tabular-nums text-zinc-800">
                            {formatCleanupBytes(item.size)}
                          </div>
                          <div className="mt-0.5 text-[11px] tabular-nums text-zinc-400">
                            {formatModifiedDate(item.modified_at_unix_ms)}
                          </div>
                        </div>
                      </li>
                    )
                  })}
                </ul>
              )}
            </div>
          </div>
        ) : (
          <div className="flex min-h-0 flex-1 flex-col">
            {/* Duplicate toolbar */}
            <div className="shrink-0 space-y-2.5 border-b border-zinc-200 bg-zinc-50/60 px-5 py-3">
              <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-zinc-600">
                <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
                  <span>
                    {zh('重复番号：', 'Duplicate codes: ')}{' '}
                    <strong className="font-semibold tabular-nums text-zinc-900">
                      {scanResult?.duplicate_group_count || 0}
                    </strong>
                  </span>
                  <span>
                    {zh('涉及视频：', 'Videos involved: ')}{' '}
                    <strong className="font-semibold tabular-nums text-zinc-900">
                      {scanResult?.duplicate_file_count || 0}
                    </strong>
                  </span>
                  <span>
                    {zh('预计可释放：', 'Est. reclaimable: ')}{' '}
                    <strong className="font-semibold tabular-nums text-blue-700">
                      {formatCleanupBytes(scanResult?.duplicate_reclaimable_size || 0)}
                    </strong>
                  </span>
                </div>
                <div className="flex flex-wrap items-center gap-1.5">
                  <button
                    type="button"
                    onClick={() => applyQuickSelectDuplicates('largest')}
                    disabled={visibleDuplicateGroups.length === 0 || deleting}
                    className="rounded-lg border border-zinc-300 bg-white px-2.5 py-1 text-xs font-medium text-zinc-700 hover:bg-zinc-100 disabled:opacity-50"
                  >
                    {zh('每组保留最大文件', 'Keep Largest per Group')}
                  </button>
                  <button
                    type="button"
                    onClick={() => applyQuickSelectDuplicates('newest')}
                    disabled={visibleDuplicateGroups.length === 0 || deleting}
                    className="rounded-lg border border-zinc-300 bg-white px-2.5 py-1 text-xs font-medium text-zinc-700 hover:bg-zinc-100 disabled:opacity-50"
                  >
                    {zh('每组保留最新文件', 'Keep Newest per Group')}
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setSelectedDuplicatePaths(new Set())
                      setQuickSelectNote('')
                    }}
                    disabled={selectedDuplicatePaths.size === 0 || deleting}
                    className="rounded-lg border border-zinc-300 bg-white px-2.5 py-1 text-xs font-medium text-zinc-600 hover:bg-zinc-100 disabled:opacity-50"
                  >
                    {zh('清空选择', 'Clear Selection')}
                  </button>
                </div>
              </div>

              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="flex flex-wrap items-center gap-2">
                  <div className="relative">
                    <SearchRoundedIcon
                      className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-zinc-400"
                      sx={{ fontSize: 16 }}
                    />
                    <input
                      type="search"
                      value={duplicateSearch}
                      onChange={(event) => setDuplicateSearch(event.target.value)}
                      placeholder={zh(
                        '按番号、标题或文件名筛选…',
                        'Filter by code, title, or filename...'
                      )}
                      className="w-56 rounded-lg border border-zinc-300 bg-white py-1 pl-8 pr-2.5 text-xs outline-none focus:border-blue-500 sm:w-64"
                    />
                  </div>
                  <select
                    value={duplicateSort}
                    aria-label={zh('重复文件排序方式', 'Duplicate sort order')}
                    onChange={(event) => setDuplicateSort(event.target.value)}
                    className="rounded-lg border border-zinc-300 bg-white px-2.5 py-1 text-xs text-zinc-700 outline-none focus:border-blue-500"
                  >
                    <option value="count">{zh('按重复数量排序', 'Sort by duplicate count')}</option>
                    <option value="reclaimable">
                      {zh('按可释放空间排序', 'Sort by reclaimable size')}
                    </option>
                    <option value="code">{zh('按番号名称排序', 'Sort by code name')}</option>
                  </select>
                </div>

                {multiPartGroupCount > 0 && (
                  <label className="flex cursor-pointer items-center gap-1.5 text-xs text-zinc-700">
                    <input
                      type="checkbox"
                      checked={hideMultiPart}
                      onChange={(event) => setHideMultiPart(event.target.checked)}
                      className="h-3.5 w-3.5 rounded"
                    />
                    <span>
                      {zh(
                        `隐藏疑似分段 (${multiPartGroupCount})`,
                        `Hide suspected multi-part (${multiPartGroupCount})`
                      )}
                    </span>
                  </label>
                )}
              </div>

              {quickSelectNote && (
                <div className="flex items-center gap-1.5 rounded-lg bg-amber-50 px-3 py-1.5 text-xs text-amber-800">
                  <WarningAmberRoundedIcon sx={{ fontSize: 16 }} />
                  <span>{quickSelectNote}</span>
                </div>
              )}
            </div>

            {/* Duplicate groups list */}
            <div className="min-h-0 flex-1 overflow-y-auto px-5 py-3">
              {scanning && !scanResult ? (
                <div className="flex h-full flex-col items-center justify-center gap-3 text-sm text-zinc-500">
                  <CircularProgress size={24} />
                  <span>
                    {zh('正在按番号统计重复文件…', 'Scanning for duplicate files by code...')}
                  </span>
                </div>
              ) : visibleDuplicateGroups.length === 0 ? (
                <div className="flex h-full flex-col items-center justify-center gap-2 text-center text-sm text-zinc-500">
                  <ContentCopyRoundedIcon className="text-zinc-300" sx={{ fontSize: 36 }} />
                  <p>
                    {duplicateGroups.length === 0
                      ? zh(
                          '当前目录下未发现同番号重复视频文件。',
                          'No duplicate video files by JAV code were found in this directory.'
                        )
                      : zh(
                          '没有符合当前筛选条件的重复番号。',
                          'No duplicate groups match the current filter.'
                        )}
                  </p>
                </div>
              ) : (
                <div className="space-y-3">
                  {visibleDuplicateGroups.map((group) => {
                    const files = group.files || []
                    const fpCounts = duplicateFingerprintCounts(group)
                    const maxSize = files.reduce((max, f) => Math.max(max, Number(f.size) || 0), 0)
                    const selectedInGroup = files.filter((f) =>
                      selectedDuplicatePaths.has(f.relative_path)
                    ).length
                    const allInGroupSelected = files.length > 0 && selectedInGroup === files.length
                    return (
                      <section
                        key={group.code}
                        className={`overflow-hidden rounded-xl border ${
                          allInGroupSelected
                            ? 'border-amber-400 bg-amber-50/20'
                            : 'border-zinc-200 bg-white'
                        }`}
                      >
                        <div className="flex flex-wrap items-center justify-between gap-2 border-b border-zinc-100 bg-zinc-50/80 px-3.5 py-2.5">
                          <div className="flex min-w-0 flex-wrap items-center gap-2">
                            <span className="rounded-md bg-zinc-900 px-2 py-0.5 font-mono text-xs font-semibold text-white">
                              {group.code}
                            </span>
                            {group.title && (
                              <span
                                className="max-w-md truncate text-xs text-zinc-600"
                                title={group.title}
                              >
                                {group.title}
                              </span>
                            )}
                            <span className="rounded-full bg-blue-50 px-2 py-0.5 text-[11px] font-medium tabular-nums text-blue-700">
                              {zh(`${group.count} 个文件`, `${group.count} files`)}
                            </span>
                            <span className="text-xs tabular-nums text-zinc-500">
                              {zh(
                                `共 ${formatCleanupBytes(group.total_size)} · 可释放 ${formatCleanupBytes(group.reclaimable_size)}`,
                                `Total ${formatCleanupBytes(group.total_size)} · Reclaimable ${formatCleanupBytes(group.reclaimable_size)}`
                              )}
                            </span>
                            {group.has_multi_part && (
                              <span className="inline-flex items-center gap-1 rounded-full bg-amber-100 px-2 py-0.5 text-[11px] font-medium text-amber-800">
                                <WarningAmberRoundedIcon sx={{ fontSize: 13 }} />
                                {zh('疑似分段', 'Suspected multi-part')}
                              </span>
                            )}
                            {allInGroupSelected && (
                              <span className="rounded-full bg-red-100 px-2 py-0.5 text-[11px] font-medium text-red-700">
                                {zh('已选中全部文件', 'All copies selected')}
                              </span>
                            )}
                          </div>
                          <div className="flex items-center gap-2 text-xs">
                            <button
                              type="button"
                              onClick={() => applyGroupKeepLargest(group)}
                              disabled={deleting}
                              className="text-blue-600 hover:underline disabled:opacity-50"
                            >
                              {zh('仅保留最大', 'Keep largest')}
                            </button>
                            {selectedInGroup > 0 && (
                              <>
                                <span className="text-zinc-300">|</span>
                                <button
                                  type="button"
                                  onClick={() => clearGroupSelection(group)}
                                  disabled={deleting}
                                  className="text-zinc-500 hover:text-zinc-800 hover:underline disabled:opacity-50"
                                >
                                  {zh('取消本组', 'Clear')}
                                </button>
                              </>
                            )}
                          </div>
                        </div>

                        <ul className="divide-y divide-zinc-100">
                          {files.map((file, idx) => {
                            const checked = selectedDuplicatePaths.has(file.relative_path)
                            const isLargest =
                              idx === 0 && Number(file.size) === maxSize && maxSize > 0
                            const sameFingerprint =
                              Boolean(file.fingerprint) &&
                              (fpCounts.get(file.fingerprint) || 0) >= 2
                            const durationLabel = formatCleanupDuration(file.duration_sec)
                            return (
                              <li
                                key={file.relative_path}
                                className={`flex items-center gap-3 px-3.5 py-2.5 transition ${
                                  checked ? 'bg-red-50/40' : 'hover:bg-zinc-50/80'
                                }`}
                              >
                                <input
                                  type="checkbox"
                                  checked={checked}
                                  disabled={deleting}
                                  aria-label={zh(
                                    `选择 ${file.relative_path}`,
                                    `Select ${file.relative_path}`
                                  )}
                                  onChange={() => toggleDuplicatePath(file.relative_path)}
                                  className="h-4 w-4 shrink-0 rounded"
                                />
                                <div className="min-w-0 flex-1">
                                  <div className="flex flex-wrap items-center gap-1.5">
                                    <span className="truncate text-sm font-medium text-zinc-900">
                                      {file.filename}
                                    </span>
                                    {isLargest && (
                                      <span className="rounded-full bg-emerald-50 px-2 py-0.5 text-[11px] font-medium text-emerald-700">
                                        {zh('最大', 'Largest')}
                                      </span>
                                    )}
                                    {file.code_source === 'jav' ? (
                                      <span className="rounded-full bg-blue-50 px-2 py-0.5 text-[11px] font-medium text-blue-700">
                                        {zh('已刮削', 'Scraped')}
                                      </span>
                                    ) : file.code_source === 'override' ? (
                                      <span className="rounded-full bg-indigo-50 px-2 py-0.5 text-[11px] font-medium text-indigo-700">
                                        {zh('手动指定', 'Manual')}
                                      </span>
                                    ) : (
                                      <span className="rounded-full bg-zinc-100 px-2 py-0.5 text-[11px] font-medium text-zinc-600">
                                        {zh('文件名识别', 'Filename')}
                                      </span>
                                    )}
                                    {file.part_label && (
                                      <span className="rounded-full bg-amber-50 px-2 py-0.5 text-[11px] font-medium text-amber-800">
                                        {zh(`分段 ${file.part_label}`, `Part ${file.part_label}`)}
                                      </span>
                                    )}
                                    {sameFingerprint && (
                                      <span className="rounded-full bg-violet-50 px-2 py-0.5 text-[11px] font-medium text-violet-700">
                                        {zh('内容相同', 'Same content')}
                                      </span>
                                    )}
                                  </div>
                                  <div
                                    className="mt-0.5 truncate font-mono text-xs text-zinc-500"
                                    title={file.relative_path}
                                  >
                                    {file.relative_path}
                                  </div>
                                </div>
                                <div className="shrink-0 text-right">
                                  <div className="text-xs font-semibold tabular-nums text-zinc-900">
                                    {formatCleanupBytes(file.size)}
                                  </div>
                                  <div className="mt-0.5 flex items-center justify-end gap-1.5 text-[11px] tabular-nums text-zinc-400">
                                    {file.resolution && <span>{file.resolution}</span>}
                                    {durationLabel && <span>{durationLabel}</span>}
                                    <span>{formatModifiedDate(file.modified_at_unix_ms)}</span>
                                  </div>
                                </div>
                              </li>
                            )
                          })}
                        </ul>
                      </section>
                    )
                  })}
                </div>
              )}
            </div>
          </div>
        )}

        {/* Footer */}
        <div className="flex shrink-0 flex-wrap items-center justify-between gap-3 border-t border-zinc-200 bg-zinc-50 px-5 py-3.5">
          <div className="flex flex-wrap items-center gap-x-4 gap-y-1.5 text-xs text-zinc-600">
            <span>
              {zh(
                `当前已选 ${activeSelectedPaths.size} 个文件（${formatCleanupBytes(activeSelectedSize)}）`,
                `Selected ${activeSelectedPaths.size} file(s) (${formatCleanupBytes(activeSelectedSize)})`
              )}
            </span>
            {otherTabSelectedCount > 0 && (
              <span className="rounded-full bg-zinc-200/80 px-2 py-0.5 text-[11px] text-zinc-600">
                {activeTab === TAB_JUNK
                  ? zh(
                      `重复文件页另选 ${otherTabSelectedCount} 项`,
                      `${otherTabSelectedCount} also selected in Duplicates`
                    )
                  : zh(
                      `垃圾文件页另选 ${otherTabSelectedCount} 项`,
                      `${otherTabSelectedCount} also selected in Junk`
                    )}
              </span>
            )}
            <label className="flex cursor-pointer items-center gap-1.5 text-zinc-700">
              <input
                type="checkbox"
                checked={removeEmptyDirs}
                disabled={deleting}
                onChange={(event) => setRemoveEmptyDirs(event.target.checked)}
                className="h-3.5 w-3.5 rounded"
              />
              <span>{zh('同时清理删后变空的子目录', 'Remove empty subdirectories afterward')}</span>
            </label>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={onClose}
              disabled={deleting}
              className="rounded-lg border border-zinc-300 bg-white px-3.5 py-1.5 text-sm text-zinc-700 hover:bg-zinc-100 disabled:opacity-50"
            >
              {zh('关闭', 'Close')}
            </button>
            <button
              type="button"
              onClick={() => {
                setDeleteError('')
                setConfirmDeleteOpen(true)
              }}
              disabled={activeSelectedPaths.size === 0 || deleting || scanning}
              className="inline-flex items-center gap-1.5 rounded-lg bg-red-600 px-3.5 py-1.5 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
            >
              <DeleteOutlineRoundedIcon sx={{ fontSize: 17 }} />
              <span>
                {activeTab === TAB_JUNK
                  ? zh(
                      `清理所选 (${activeSelectedPaths.size})`,
                      `Clean Selected (${activeSelectedPaths.size})`
                    )
                  : zh(
                      `删除所选 (${activeSelectedPaths.size})`,
                      `Delete Selected (${activeSelectedPaths.size})`
                    )}
              </span>
            </button>
          </div>
        </div>
      </AppModal>

      {/* Delete confirmation modal */}
      {confirmDeleteOpen && (
        <AppModal
          ariaLabelledby={confirmTitleId}
          className="p-4"
          closeDisabled={deleting}
          contentClassName="w-full max-w-md rounded-2xl bg-white p-5 shadow-xl"
          onClose={() => setConfirmDeleteOpen(false)}
          zIndex={1440}
        >
          <div
            id={confirmTitleId}
            className="flex items-center gap-2 text-base font-semibold text-zinc-900"
          >
            <WarningAmberRoundedIcon className="text-red-600" fontSize="small" />
            <span>{zh('确认清理所选文件', 'Confirm File Deletion')}</span>
          </div>
          <p className="mt-3 text-sm leading-6 text-zinc-600">
            {zh(
              `即将删除 ${activeSelectedPaths.size} 个文件，预计释放 ${formatCleanupBytes(activeSelectedSize)}。`,
              `About to remove ${activeSelectedPaths.size} file(s), freeing ${formatCleanupBytes(activeSelectedSize)}.`
            )}
          </p>
          <p className="mt-1.5 text-xs leading-5 text-zinc-500">
            {isWindows
              ? zh('文件将移至系统回收站。', 'Selected files will be moved to the Recycle Bin.')
              : zh(
                  '文件将从磁盘直接删除，此操作无法撤销，请确认后再继续。',
                  'Selected files will be permanently deleted from disk and cannot be restored.'
                )}
          </p>
          {removeEmptyDirs && (
            <p className="mt-1.5 text-xs text-zinc-500">
              {zh(
                '删除完成后会自动清理变空的子目录（不会删除根目录）。',
                'Empty subdirectories left behind will also be removed (the root directory is kept).'
              )}
            </p>
          )}
          {fullySelectedDuplicateCodes.length > 0 && (
            <div className="mt-3 rounded-xl border border-amber-300 bg-amber-50 p-3 text-xs leading-5 text-amber-900">
              <div className="font-semibold">
                {zh(
                  '注意：以下番号的所有文件均已被选中，删除后该目录将不再保留对应视频：',
                  'Warning: all copies of the following JAV codes are selected; no video will remain in this directory:'
                )}
              </div>
              <div className="mt-1 font-mono">{fullySelectedDuplicateCodes.join(', ')}</div>
            </div>
          )}
          {deleteError && <div className="mt-3 text-xs text-red-600">{deleteError}</div>}
          <div className="mt-5 flex justify-end gap-2">
            <button
              type="button"
              onClick={() => setConfirmDeleteOpen(false)}
              disabled={deleting}
              className="rounded-lg border border-zinc-300 px-3.5 py-1.5 text-sm text-zinc-700 hover:bg-zinc-50 disabled:opacity-50"
            >
              {zh('取消', 'Cancel')}
            </button>
            <button
              type="button"
              onClick={handleConfirmDelete}
              disabled={deleting}
              className="inline-flex items-center gap-1.5 rounded-lg bg-red-600 px-3.5 py-1.5 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-60"
            >
              {deleting && <CircularProgress size={14} color="inherit" />}
              <span>
                {deleting ? zh('删除中…', 'Deleting...') : zh('确认删除', 'Confirm Delete')}
              </span>
            </button>
          </div>
        </AppModal>
      )}

      {/* Suspected ad image lightbox preview */}
      {previewItem && (
        <AppModal
          ariaLabelledby={previewTitleId}
          className="p-4"
          contentClassName="flex max-h-[85dvh] w-full max-w-2xl flex-col overflow-hidden rounded-2xl bg-white shadow-xl"
          onClose={() => setPreviewItem(null)}
          zIndex={1450}
        >
          <div className="flex shrink-0 items-center justify-between gap-3 border-b border-zinc-200 px-4 py-3">
            <div className="min-w-0">
              <div
                id={previewTitleId}
                className="flex items-center gap-1.5 truncate text-sm font-semibold text-zinc-900"
              >
                <ImageOutlinedIcon fontSize="small" className="text-amber-600" />
                <span className="truncate">{previewItem.filename}</span>
              </div>
              <div className="mt-0.5 truncate font-mono text-xs text-zinc-500">
                {previewItem.relative_path}
              </div>
            </div>
            <IconButton
              type="button"
              size="small"
              aria-label={zh('关闭预览', 'Close preview')}
              onClick={() => setPreviewItem(null)}
            >
              <CloseRoundedIcon fontSize="small" />
            </IconButton>
          </div>
          <div className="flex min-h-0 flex-1 items-center justify-center overflow-auto bg-zinc-950/90 p-4">
            <img
              src={directoryCleanupPreviewUrl(directory.id, previewItem.relative_path)}
              alt={previewItem.filename}
              className="max-h-[60dvh] max-w-full rounded object-contain"
            />
          </div>
          <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-t border-zinc-200 bg-zinc-50 px-4 py-3 text-xs text-zinc-600">
            <div className="flex flex-wrap items-center gap-3">
              <span>{zh(previewItem.reason_zh, previewItem.reason_en)}</span>
              {previewItem.width > 0 && previewItem.height > 0 && (
                <span className="tabular-nums">
                  {previewItem.width} × {previewItem.height}
                </span>
              )}
              <span className="tabular-nums">{formatCleanupBytes(previewItem.size)}</span>
            </div>
            <button
              type="button"
              onClick={() => toggleJunkPath(previewItem.relative_path)}
              className={`rounded-lg px-3 py-1.5 text-xs font-medium transition ${
                selectedJunkPaths.has(previewItem.relative_path)
                  ? 'border border-zinc-300 bg-white text-zinc-700 hover:bg-zinc-100'
                  : 'bg-red-600 text-white hover:bg-red-700'
              }`}
            >
              {selectedJunkPaths.has(previewItem.relative_path)
                ? zh('取消删除勾选', 'Uncheck for Deletion')
                : zh('勾选为待删除', 'Check for Deletion')}
            </button>
          </div>
        </AppModal>
      )}
    </>
  )
}
