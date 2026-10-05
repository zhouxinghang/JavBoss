import { useEffect, useRef } from 'react'

// Warms neighbouring list pages while the browser is idle. The list resource
// caches whatever is prefetched, so the following page switch or waterfall
// slice renders without waiting on the network.
export default function useListPrefetch({
  enabled,
  requestKey,
  itemCount,
  hasNext,
  hasPrev = false,
  prefetchNext,
  prefetchPrev,
}) {
  const actionsRef = useRef({ prefetchNext, prefetchPrev })
  actionsRef.current = { prefetchNext, prefetchPrev }

  useEffect(() => {
    if (!enabled || (!hasNext && !hasPrev)) return undefined
    let cancelled = false
    const run = () => {
      if (cancelled) return
      if (hasPrev) actionsRef.current.prefetchPrev?.()
      if (hasNext) actionsRef.current.prefetchNext?.()
    }
    if (typeof window.requestIdleCallback === 'function') {
      const handle = window.requestIdleCallback(run, { timeout: 1000 })
      return () => {
        cancelled = true
        window.cancelIdleCallback?.(handle)
      }
    }
    const handle = window.setTimeout(run, 300)
    return () => {
      cancelled = true
      window.clearTimeout(handle)
    }
  }, [enabled, requestKey, itemCount, hasNext, hasPrev])
}
