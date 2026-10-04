import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import VideoGrid from '@/features/video/components/VideoGrid'
import '@/index.css'

const TOTAL = 200

const videos = Array.from({ length: TOTAL }, (_, index) => ({
  id: index + 1,
  path: `videos/video-${index + 1}.mp4`,
  directory_path: '/library',
  directory: { path: '/library' },
  duration_sec: 600,
  size: 1024,
  fingerprint: '',
  tags: [],
}))

function Fixture() {
  return (
    <div id="grid-host" style={{ padding: '0 16px' }}>
      <VideoGrid videos={videos} selectedIds={new Set()} onToggleSelect={() => {}} />
    </div>
  )
}

window.fixtureVideoCount = TOTAL
createRoot(document.getElementById('root')).render(
  <StrictMode>
    <Fixture />
  </StrictMode>
)
