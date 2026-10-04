import VideoCard from '@/features/video/components/VideoCard'
import VirtualizedGrid from '@/shared/ui/VirtualizedGrid'
import { videoSelectionKey } from '@/store'

export default function VideoGrid({
  videos,
  selectedIds,
  onToggleSelect,
  showSelection = true,
  onPlay,
  onOpenFile,
  onRevealFile,
  openFileLabel,
  onOpenTagPicker,
  showTagEditor = true,
  onOpenScreenshots,
  onOpenScrapeSettings,
  onRenameVideo,
  onDeleteVideo,
  onTagClick,
}) {
  return (
    <VirtualizedGrid
      items={videos}
      minColumnWidth={240}
      gap={32}
      estimateRowHeight={300}
      getItemKey={(video) => videoSelectionKey(video)}
      renderItem={(v) => (
        <VideoCard
          video={v}
          checked={selectedIds.has(videoSelectionKey(v))}
          onToggleSelect={onToggleSelect}
          showSelection={showSelection}
          onPlay={onPlay}
          onOpenFile={onOpenFile}
          onRevealFile={onRevealFile}
          openFileLabel={openFileLabel}
          onOpenTagPicker={onOpenTagPicker}
          showTagEditor={showTagEditor}
          onOpenScreenshots={onOpenScreenshots}
          onOpenScrapeSettings={onOpenScrapeSettings}
          onRenameVideo={onRenameVideo}
          onDeleteVideo={onDeleteVideo}
          onTagClick={onTagClick}
        />
      )}
    />
  )
}
