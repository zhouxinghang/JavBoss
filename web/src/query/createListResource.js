// One owner for a list's requests and its prefetched pages. The state adapter
// keeps this independent of React/Zustand.
export function createListResource({
  get,
  set,
  fields,
  query,
  scope = () => '',
  fetcher,
  random = () => false,
  errorMessage = (error) => error.message,
  hasNextField,
  randomTotal = false,
  cacheLimit = 6,
}) {
  let generation = 0
  let successfulKey = null
  let pending = null
  let pendingMore = null
  let exhaustedKey = null

  // Pages fetched ahead of time never enter the live list state; they wait here
  // so a page switch or the next waterfall slice can render without a network
  // round trip. Keys mirror the request identity (scope + params, offset
  // included) and an entry is consumed by its first read, so a prefetched page
  // can never outlive a single use.
  const pageCache = new Map()
  const pendingPrefetch = new Map()

  const stateKey = (state, params = query(state)) => JSON.stringify([scope(state), params])

  const cachePage = (cacheKey, page) => {
    pageCache.delete(cacheKey)
    pageCache.set(cacheKey, page)
    while (pageCache.size > cacheLimit) {
      pageCache.delete(pageCache.keys().next().value)
    }
  }

  const takePage = (cacheKey) => {
    if (!pageCache.has(cacheKey)) return null
    const page = pageCache.get(cacheKey)
    pageCache.delete(cacheKey)
    return page
  }

  const dropPrefetch = () => {
    for (const entry of pendingPrefetch.values()) entry.controller.abort()
    pendingPrefetch.clear()
    pageCache.clear()
  }

  const isCurrent = (request) =>
    request.generation === generation && request.key === stateKey(get())

  // Replaces the in-flight requests without touching prefetched pages: a page
  // switch changes the request key but should still reuse what was prefetched.
  const reset = () => {
    generation += 1
    pending?.controller?.abort()
    pendingMore?.controller?.abort()
    pending = null
    pendingMore = null
    successfulKey = null
    exhaustedKey = null
    set({ [fields.loading]: false, [fields.loadingMore]: false })
  }

  const invalidate = () => {
    reset()
    dropPrefetch()
  }

  const applyFirstPage = (state, params, page, requestKey) => {
    const total = page.total ?? state[fields.total] ?? 0
    const patch = { [fields.items]: page.items || [], [fields.total]: total, [fields.error]: null }
    if (hasNextField) patch[hasNextField] = !random(state) && params.offset + params.limit < total
    successfulKey = requestKey
    set(patch)
  }

  const appendPage = (state, params, offset, page, requestKey) => {
    const items = page.items || []
    const total = page.total ?? state[fields.total] ?? 0
    if (!items.length) exhaustedKey = requestKey
    const patch = {
      [fields.items]: [...(get()[fields.items] || []), ...items],
      [fields.total]: total,
    }
    if (hasNextField)
      patch[hasNextField] =
        items.length > 0 &&
        (total > 0 ? offset + items.length < total : items.length >= params.limit)
    set(patch)
  }

  const load = (options = {}) => {
    const state = get()
    const requestKey = stateKey(state)
    if (!options.force && pending?.key === requestKey) return pending.promise
    if (!options.force && successfulKey === requestKey) return Promise.resolve()
    const params = query(state)
    if (options.force) {
      dropPrefetch()
    } else {
      const cached = takePage(requestKey)
      if (cached) {
        applyFirstPage(state, params, cached, requestKey)
        return Promise.resolve()
      }
      // Join an in-flight prefetch instead of firing a duplicate request.
      const entry = pendingPrefetch.get(requestKey)
      if (entry) {
        const request = { generation, key: requestKey, controller: null }
        pending = request
        request.promise = entry.promise.then(() => {
          if (pending !== request) return undefined
          pending = null
          const current = get()
          if (stateKey(current) !== requestKey) return load(options)
          const page = takePage(requestKey)
          if (!page) return load(options)
          applyFirstPage(current, query(current), page, requestKey)
          return undefined
        })
        return request.promise
      }
    }
    reset()
    const request = { generation, key: requestKey, controller: new AbortController() }
    pending = request
    set({ [fields.loading]: true, [fields.error]: null })
    request.promise = (async () => {
      try {
        const response = await fetcher({ ...params, signal: request.controller.signal })
        if (!isCurrent(request)) return
        const items = response.items || []
        const total = random(state) && randomTotal ? items.length : (response.total ?? 0)
        const patch = { [fields.items]: items, [fields.total]: total }
        if (hasNextField)
          patch[hasNextField] = !random(state) && params.offset + params.limit < total
        successfulKey = requestKey
        set(patch)
      } catch (error) {
        if (isCurrent(request) && !request.controller.signal.aborted) {
          set({ [fields.error]: errorMessage(error) })
        }
      } finally {
        if (pending === request) {
          pending = null
          set({ [fields.loading]: false })
        }
      }
    })()
    return request.promise
  }

  const loadMore = () => {
    const state = get()
    const requestKey = stateKey(state)
    if (pendingMore?.key === requestKey) return pendingMore.promise
    if (
      state[fields.loading] ||
      random(state) ||
      requestKey !== successfulKey ||
      exhaustedKey === requestKey
    )
      return Promise.resolve()
    const params = query(state)
    const loaded = state[fields.items]?.length || 0
    const total = state[fields.total] || 0
    const offset = params.offset + loaded
    if (total > 0 && offset >= total) return Promise.resolve()
    const cacheKey = stateKey(state, { ...params, offset })

    const cached = takePage(cacheKey)
    if (cached) {
      appendPage(state, params, offset, cached, requestKey)
      return Promise.resolve()
    }
    const entry = pendingPrefetch.get(cacheKey)
    if (entry) {
      const request = { generation, key: requestKey, controller: null }
      pendingMore = request
      request.promise = entry.promise.then(() => {
        if (pendingMore !== request) return undefined
        pendingMore = null
        const current = get()
        if (stateKey(current) !== requestKey) return loadMore()
        const page = takePage(cacheKey)
        if (!page) return loadMore()
        const currentParams = query(current)
        appendPage(
          current,
          currentParams,
          currentParams.offset + (current[fields.items]?.length || 0),
          page,
          requestKey
        )
        return undefined
      })
      return request.promise
    }

    const request = { generation, key: requestKey, controller: new AbortController() }
    pendingMore = request
    set({ [fields.loadingMore]: true, [fields.error]: null })
    request.promise = (async () => {
      try {
        const response = await fetcher({ ...params, offset, signal: request.controller.signal })
        if (!isCurrent(request)) return
        const items = response.items || []
        const nextTotal = response.total ?? total
        if (!items.length) exhaustedKey = requestKey
        const patch = {
          [fields.items]: [...(get()[fields.items] || []), ...items],
          [fields.total]: nextTotal,
        }
        if (hasNextField)
          patch[hasNextField] =
            items.length > 0 &&
            (nextTotal > 0 ? offset + items.length < nextTotal : items.length >= params.limit)
        set(patch)
      } catch (error) {
        if (isCurrent(request) && !request.controller.signal.aborted) {
          set({ [fields.error]: errorMessage(error) })
        }
      } finally {
        if (pendingMore === request) {
          pendingMore = null
          set({ [fields.loadingMore]: false })
        }
      }
    })()
    return request.promise
  }

  // Fetches one page into the cache without ever touching loading state, so the
  // UI keeps showing the current page while the next one warms up.
  const prefetch = (offset) => {
    const state = get()
    if (!Number.isFinite(offset) || offset < 0) return Promise.resolve()
    // Never race an append: loadMore for the same slice either joins this
    // prefetch or fetches it itself.
    if (state[fields.loading] || state[fields.loadingMore] || random(state))
      return Promise.resolve()
    const params = { ...query(state), offset }
    const cacheKey = stateKey(state, params)
    if (pageCache.has(cacheKey)) return Promise.resolve()
    if (successfulKey === cacheKey || pending?.key === cacheKey || pendingMore?.key === cacheKey)
      return Promise.resolve()
    const existing = pendingPrefetch.get(cacheKey)
    if (existing) return existing.promise
    const entry = { controller: new AbortController(), generation, promise: null }
    entry.promise = (async () => {
      try {
        const response = await fetcher({ ...params, signal: entry.controller.signal })
        if (entry.generation !== generation) return
        cachePage(cacheKey, {
          items: response.items || [],
          total: response.total ?? get()[fields.total] ?? 0,
        })
      } catch {
        // Prefetches are opportunistic: a failure just means the page is
        // fetched on demand later.
      } finally {
        if (pendingPrefetch.get(cacheKey) === entry) pendingPrefetch.delete(cacheKey)
      }
    })()
    pendingPrefetch.set(cacheKey, entry)
    return entry.promise
  }

  // Prefetches the slice that a scroll or "next page" click would request next.
  // `items.length` is the loaded span, which covers both the single-page grid
  // layout and the appended waterfall layout.
  const prefetchNext = () => {
    const state = get()
    if (state[fields.loading] || random(state) || exhaustedKey === stateKey(state))
      return Promise.resolve()
    const params = query(state)
    const loaded = state[fields.items]?.length || 0
    if (!loaded) return Promise.resolve()
    const offset = params.offset + loaded
    if (offset <= params.offset) return Promise.resolve()
    const total = state[fields.total] || 0
    if (total > 0 && offset >= total) return Promise.resolve()
    return prefetch(offset)
  }

  const prefetchPrev = () => {
    const state = get()
    if (state[fields.loading] || random(state)) return Promise.resolve()
    const params = query(state)
    const offset = params.offset - params.limit
    if (offset < 0) return Promise.resolve()
    return prefetch(offset)
  }

  return { load, loadMore, prefetch, prefetchNext, prefetchPrev, invalidate }
}
