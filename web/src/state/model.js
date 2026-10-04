export const VIDEO_PAGE_SIZE = 25

export const JAV_PAGE_SIZE = 24

export const JAV_STUDIO_PAGE_SIZE = 25

export const JAV_SERIES_PAGE_SIZE = 25

export const JAV_GRID_COLUMNS_AUTO = 0

export const JAV_TITLE_MAX_ROWS_DEFAULT = 2

export const JAV_IDOL_TAG_MAX_ROWS_DEFAULT = 2

export const JAV_TAG_MAX_ROWS_DEFAULT = 2

export const RANDOM_SEED_MAX = 2147483646

export const WATERFALL_KEYS = ['video', 'jav', 'idol', 'studio', 'series']

// Maps each waterfall view to the config field that stores its default. The
// toolbar toggle and the matching display setting share one value, mirroring
// how "jav_compact_default" backs the compact-mode toggle.
export const WATERFALL_CONFIG_KEYS = {
  video: 'video_waterfall_default',
  jav: 'jav_waterfall_default',
  idol: 'idol_waterfall_default',
  studio: 'studio_waterfall_default',
  series: 'series_waterfall_default',
}

export const emptyWaterfallModes = () =>
  Object.fromEntries(WATERFALL_KEYS.map((key) => [key, false]))

export const directoryScopeResetState = () => ({
  page: 1,
  javPage: 1,
  idolPage: 1,
  studioPage: 1,
  seriesPage: 1,
  videoTempSort: '',
  javTempSort: '',
  idolTempSort: '',
  randomMode: false,
  randomSeed: null,
  javRandomMode: false,
  javRandomSeed: null,
})

export const normalizeSeed = (seed) => {
  const num = Math.floor(Number(seed))
  if (!Number.isFinite(num) || num <= 0) return null
  return Math.min(num, RANDOM_SEED_MAX)
}

export const generateSeed = () => Math.floor(Math.random() * RANDOM_SEED_MAX) + 1

export const videoSelectionKey = (video) => {
  if (video?.location_id) return `loc:${video.location_id}`
  if (video?.id) return `vid:${video.id}`
  return ''
}

export const selectedVideoContentIds = (state) => {
  const ids = new Set()
  for (const key of state.selectedVideoIds || []) {
    const meta = state.selectedVideoMeta?.[key]
    const raw = meta && typeof meta === 'object' ? meta.video_id : key
    const parsed = Number(raw)
    if (Number.isFinite(parsed) && parsed > 0) ids.add(parsed)
  }
  return Array.from(ids)
}
