import { useStore } from '@/store'
import { useShallow } from 'zustand/react/shallow'
import { useCallback } from 'react'

export default function useListDisplay({ configLoaded, hydrated }) {
  const {
    waterfallModes,
    setWaterfallMode: persistWaterfallMode,
    syncWaterfallMode: syncWaterfallModeState,
    loadVideos,
    loadJavIdols,
    loadJavFavoriteGroups,
    loadJavStudios,
    loadJavSeries,
    loadJavs,
  } = useStore(
    useShallow((state) => ({
      waterfallModes: state.waterfallModes,
      setWaterfallMode: state.setWaterfallMode,
      syncWaterfallMode: state.syncWaterfallMode,
      loadVideos: state.loadVideos,
      loadJavIdols: state.loadJavIdols,
      loadJavFavoriteGroups: state.loadJavFavoriteGroups,
      loadJavStudios: state.loadJavStudios,
      loadJavSeries: state.loadJavSeries,
      loadJavs: state.loadJavs,
    }))
  )

  const forceReloadVideos = useCallback(() => {
    if (!hydrated || !configLoaded) return
    loadVideos({ force: true })
  }, [configLoaded, hydrated, loadVideos])

  const forceReloadJavByTab = useCallback(
    (tab) => {
      if (!hydrated || !configLoaded) return
      if (tab === 'download') {
        return
      } else if (tab === 'idol') {
        loadJavIdols({ force: true })
        loadJavFavoriteGroups('idol', { force: true })
      } else if (tab === 'studio') {
        loadJavStudios({ force: true })
        loadJavFavoriteGroups('studio', { force: true })
      } else if (tab === 'series') {
        loadJavSeries({ force: true })
        loadJavFavoriteGroups('series', { force: true })
      } else {
        loadJavs({ force: true })
        loadJavFavoriteGroups('jav', { force: true })
      }
    },
    [
      configLoaded,
      hydrated,
      loadJavFavoriteGroups,
      loadJavIdols,
      loadJavSeries,
      loadJavStudios,
      loadJavs,
    ]
  )

  // Leaving waterfall mode changes how the list is paged, so the current view
  // must be reloaded for the switched layout.
  const reloadWaterfallList = useCallback(
    (key) => {
      if (!hydrated || !configLoaded) return
      if (key === 'video') {
        loadVideos({ force: true })
      } else if (key === 'jav') {
        loadJavs({ force: true })
      } else if (key === 'idol') {
        loadJavIdols({ force: true })
      } else if (key === 'studio') {
        loadJavStudios({ force: true })
      } else if (key === 'series') {
        loadJavSeries({ force: true })
      }
    },
    [configLoaded, hydrated, loadJavIdols, loadJavSeries, loadJavStudios, loadJavs, loadVideos]
  )

  const setWaterfallMode = useCallback(
    (key, enabled) => {
      const next = Boolean(enabled)
      const result = persistWaterfallMode(key, next)
      if (!next) reloadWaterfallList(key)
      return result
    },
    [persistWaterfallMode, reloadWaterfallList]
  )

  // Display settings already write the config; only mirror the saved value and
  // refresh the active list without a redundant config request.
  const syncWaterfallMode = useCallback(
    (key, enabled) => {
      const next = Boolean(enabled)
      syncWaterfallModeState(key, next)
      if (!next) reloadWaterfallList(key)
    },
    [reloadWaterfallList, syncWaterfallModeState]
  )

  return {
    waterfallModes,
    forceReloadVideos,
    forceReloadJavByTab,
    setWaterfallMode,
    syncWaterfallMode,
  }
}
