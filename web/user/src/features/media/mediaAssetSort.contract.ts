import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('./MediaAssetsPage.tsx', import.meta.url), 'utf8')
for (const sortOption of ['file_size_bytes:desc', 'duration_ms:desc']) {
  if (!pageSource.includes(sortOption)) throw new Error(`asset sorting must expose ${sortOption}`)
}
