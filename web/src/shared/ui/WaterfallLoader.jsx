import { useEffect, useRef } from 'react'
import { zh } from '@/utils/i18n'

const ROOT_MARGIN_PX = 480

function withinRange(node) {
  if (!node) return false
  const rect = node.getBoundingClientRect()
  return rect.top <= window.innerHeight + ROOT_MARGIN_PX && rect.bottom >= -ROOT_MARGIN_PX
}

export default function WaterfallLoader({ enabled, hasMore, loading, onLoadMore }) {
  const sentinelRef = useRef(null)
  const onLoadMoreRef = useRef(onLoadMore)
  const loadingRef = useRef(loading)
  const armedRef = useRef(true)
  onLoadMoreRef.current = onLoadMore
  loadingRef.current = loading

  // Observe the sentinel once per enabled/hasMore transition. Keeping `loading`
  // out of the deps avoids tearing down and immediately re-firing the observer
  // on every load, which used to queue several pages back to back.
  useEffect(() => {
    if (!enabled || !hasMore) return undefined
    const node = sentinelRef.current
    if (!node) return undefined

    const observer = new IntersectionObserver(
      (entries) => {
        const intersecting = entries.some((entry) => entry.isIntersecting)
        if (!intersecting) {
          armedRef.current = true
          return
        }
        if (!armedRef.current || loadingRef.current) return
        armedRef.current = false
        onLoadMoreRef.current?.()
      },
      { rootMargin: `${ROOT_MARGIN_PX}px 0px` }
    )
    observer.observe(node)
    return () => observer.disconnect()
  }, [enabled, hasMore])

  // Refill after a page settles, but defer to an idle callback so the appended
  // page can commit before another request starts.
  useEffect(() => {
    if (!enabled || !hasMore || loading) return undefined
    armedRef.current = true
    if (!withinRange(sentinelRef.current)) return undefined
    armedRef.current = false

    let cancelled = false
    const trigger = () => {
      if (cancelled || loadingRef.current) return
      if (withinRange(sentinelRef.current)) onLoadMoreRef.current?.()
    }

    if (typeof window.requestIdleCallback === 'function') {
      const handle = window.requestIdleCallback(trigger, { timeout: 500 })
      return () => {
        cancelled = true
        window.cancelIdleCallback?.(handle)
      }
    }
    const handle = window.setTimeout(trigger, 250)
    return () => {
      cancelled = true
      window.clearTimeout(handle)
    }
  }, [enabled, hasMore, loading])

  if (!enabled) return null

  return (
    <div ref={sentinelRef} className="flex min-h-16 items-center justify-center py-4 text-sm">
      {loading ? (
        <span className="text-gray-500">{zh('加载更多…', 'Loading more...')}</span>
      ) : hasMore ? (
        <button
          type="button"
          onClick={onLoadMore}
          className="rounded border border-gray-300 bg-white px-3 py-1 text-gray-600 shadow-sm hover:border-gray-400"
        >
          {zh('加载更多', 'Load more')}
        </button>
      ) : (
        <span className="text-gray-400">{zh('没有更多了', 'No more items')}</span>
      )}
    </div>
  )
}
