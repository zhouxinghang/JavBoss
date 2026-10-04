import { Fragment, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useWindowVirtualizer } from '@tanstack/react-virtual'

// Window-scrolled, row-virtualized CSS grid. Rows are absolutely positioned by
// the virtualizer and measured dynamically, so cards keep their natural height
// while offscreen rows are skipped entirely instead of mounting every card.
export default function VirtualizedGrid({
  items,
  renderItem,
  getItemKey,
  minColumnWidth = 240,
  fixedColumns = 0,
  gap = 16,
  rowGap,
  estimateRowHeight = 320,
  overscan = 3,
  className = '',
  style,
}) {
  const list = Array.isArray(items) ? items : []
  const containerRef = useRef(null)
  const [width, setWidth] = useState(0)
  const [scrollMargin, setScrollMargin] = useState(0)

  // Track the container width (for column count) and its document offset (for
  // the window virtualizer's scroll margin).
  useLayoutEffect(() => {
    const node = containerRef.current
    if (!node) return undefined
    const measure = () => {
      setWidth(node.clientWidth)
      const next = node.getBoundingClientRect().top + window.scrollY
      setScrollMargin((previous) => (Math.abs(previous - next) > 1 ? next : previous))
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(node)
    window.addEventListener('resize', measure)
    return () => {
      observer.disconnect()
      window.removeEventListener('resize', measure)
    }
  }, [])

  const columns = useMemo(() => {
    if (fixedColumns > 0) return fixedColumns
    if (width <= 0) return 1
    return Math.max(1, Math.floor((width + gap) / (minColumnWidth + gap)))
  }, [fixedColumns, gap, minColumnWidth, width])

  const rowCount = Math.ceil(list.length / columns)
  const virtualizer = useWindowVirtualizer({
    count: rowCount,
    estimateSize: () => estimateRowHeight,
    overscan,
    scrollMargin,
  })

  // Re-estimate rows when the column count (and therefore card width) changes.
  const virtualizerRef = useRef(virtualizer)
  virtualizerRef.current = virtualizer
  useLayoutEffect(() => {
    virtualizerRef.current.measure()
  }, [columns])

  const verticalGap = rowGap ?? gap

  return (
    <div
      ref={containerRef}
      className={className}
      style={{
        position: 'relative',
        width: '100%',
        height: virtualizer.getTotalSize(),
        ...style,
      }}
    >
      {virtualizer.getVirtualItems().map((virtualRow) => {
        const start = virtualRow.index * columns
        const rowItems = list.slice(start, start + columns)
        return (
          <div
            key={virtualRow.key}
            data-index={virtualRow.index}
            ref={virtualizer.measureElement}
            style={{
              position: 'absolute',
              top: 0,
              left: 0,
              width: '100%',
              paddingBottom: verticalGap,
              transform: `translateY(${virtualRow.start - scrollMargin}px)`,
            }}
          >
            <div
              className="grid"
              style={{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`, gap }}
            >
              {rowItems.map((item, offset) => {
                const index = start + offset
                return (
                  <Fragment key={getItemKey ? getItemKey(item, index) : (item?.id ?? index)}>
                    {renderItem(item, index)}
                  </Fragment>
                )
              })}
            </div>
          </div>
        )
      })}
    </div>
  )
}
