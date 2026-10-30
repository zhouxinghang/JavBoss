import { zh } from '@/utils/i18n'

export function getJavDisplayTitle(item) {
  const code = item?.code?.trim()
  const title = item?.title?.trim()
  const zhTitle = item?.zh_title?.trim()
  return zhTitle || title || code || zh('未知标题', 'Untitled')
}
