import { adminTaskTypeLabel, adminTaskTypeOptions } from './adminTaskTypes'

assertEqual(adminTaskTypeLabel('text_to_image'), '文生图', 'text_to_image label')
assertEqual(adminTaskTypeLabel('image_edit'), '图片编辑', 'image_edit label')
assertEqual(adminTaskTypeLabel('image_to_image'), '图片编辑', 'legacy image_to_image label')
assertEqual(adminTaskTypeLabel(' video_to_image '), 'video_to_image', 'unknown task type trims raw value')
assertEqual(adminTaskTypeLabel(''), '未知类型', 'empty task type fallback')
assertEqual(adminTaskTypeLabel(null), '未知类型', 'null task type fallback')

for (const rawValue of ['text_to_image', 'image_edit']) {
  if (!adminTaskTypeOptions.some((option) => option.value === rawValue)) {
    throw new Error(`admin task type options should preserve raw value ${rawValue}`)
  }
}

for (const option of adminTaskTypeOptions) {
  const label = String(option.label)
  const value = String(option.value)
  if (label === value) {
    throw new Error(`admin task type option should expose localized label for ${option.value}`)
  }
}

function assertEqual(actual: string, expected: string, name: string) {
  if (actual !== expected) {
    throw new Error(`${name}: expected ${expected}, got ${actual}`)
  }
}
