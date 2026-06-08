import { formatDate } from './components'

const formatted = formatDate('2026-06-05T13:45:30Z')
if (formatted !== '2026/06/05 13:45') {
  throw new Error(`shared detail date should format ISO timestamps as readable date-time, got ${formatted}`)
}

if (/[TZ]/.test(formatted)) {
  throw new Error(`shared detail date should not expose raw ISO separators, got ${formatted}`)
}

const invalid = formatDate('not-a-date')
if (invalid !== 'not-a-date') {
  throw new Error(`shared detail date should preserve invalid values for troubleshooting, got ${invalid}`)
}

const empty = formatDate('')
if (empty !== '-') {
  throw new Error(`shared detail date should show a stable empty fallback, got ${empty}`)
}
