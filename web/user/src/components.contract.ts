import { formatDate, imagePixelsLabel, imageRatioLabel, navItems } from './components'
import { siteBrand } from './brand'

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

if (siteBrand.name !== 'Mikiko Studio') {
  throw new Error(`site brand should use the official name, got ${siteBrand.name}`)
}

const expectedNavigation = [
  ['home', '首页'],
  ['genpic', '创作'],
  ['gallery', '资产'],
  ['checkout', '积分'],
  ['api-keys', '密钥'],
  ['settings', '设置'],
]
const actualNavigation = navItems.map((item) => [item.route, item.label])
if (JSON.stringify(actualNavigation) !== JSON.stringify(expectedNavigation)) {
  throw new Error(`user shell navigation drifted, got ${JSON.stringify(actualNavigation)}`)
}

const navCopy = navItems.map((item) => item.label).join(' ')
if (/图库|Pic Gallery|Vault|Redesign/i.test(navCopy)) {
  throw new Error(`user shell navigation should use production copy, got ${navCopy}`)
}

if (imagePixelsLabel(1024, 768) !== '1024 x 768') {
  throw new Error(`lightbox pixels should show concrete dimensions, got ${imagePixelsLabel(1024, 768)}`)
}

if (imagePixelsLabel(0, 768) !== '未知' || imagePixelsLabel(undefined, 768) !== '未知') {
  throw new Error('lightbox pixels should hide missing or zero dimensions')
}

if (imageRatioLabel(1536, 864) !== '16:9') {
  throw new Error(`lightbox ratio should reduce dimensions, got ${imageRatioLabel(1536, 864)}`)
}

if (imageRatioLabel(1536, 864, '原始比例') !== '原始比例') {
  throw new Error('lightbox ratio should prefer explicit fallback labels')
}

if (imageRatioLabel(undefined, 864) !== '未知') {
  throw new Error('lightbox ratio should fall back when dimensions are incomplete')
}
