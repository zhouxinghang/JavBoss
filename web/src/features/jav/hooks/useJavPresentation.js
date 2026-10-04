import { useStore } from '@/store'
import { configFlag } from '@/utils/config'
import { useMemo, useRef } from 'react'
import { withJavTagDisplayName } from '@/utils/javTag'

export default function useJavPresentation(items) {
  const preferChineseName = useStore((state) =>
    configFlag(state.config?.jav_idol_prefer_chinese_name)
  )
  const hideSeries = useStore((state) => configFlag(state.config?.jav_hide_series))
  const hideIdols = useStore((state) => configFlag(state.config?.jav_hide_idols))
  const hideTags = useStore((state) => configFlag(state.config?.jav_hide_tags))
  const hideActions = useStore((state) => configFlag(state.config?.jav_hide_actions))
  const showFullFavoriteRating = useStore((state) =>
    configFlag(state.config?.jav_favorite_rating_show_full, false)
  )
  const showSimplifiedTags = useStore((state) => configFlag(state.config?.jav_tag_show_simplified))
  // Keep simplified item objects referentially stable so memoized cards skip
  // re-rendering when another page of items is appended to the list.
  const simplifiedCacheRef = useRef(new WeakMap())
  const displayItems = useMemo(() => {
    if (!showSimplifiedTags) return items
    const cache = simplifiedCacheRef.current
    return (items || []).map((item) => {
      if (!item || typeof item !== 'object') return item
      const cached = cache.get(item)
      if (cached) return cached
      const next = {
        ...item,
        tags: Array.isArray(item?.tags)
          ? item.tags.map((tag) => withJavTagDisplayName(tag, true))
          : item?.tags,
      }
      cache.set(item, next)
      return next
    })
  }, [items, showSimplifiedTags])
  return {
    preferChineseName,
    hideSeries,
    hideIdols,
    hideTags,
    hideActions,
    showFullFavoriteRating,
    displayItems,
  }
}
