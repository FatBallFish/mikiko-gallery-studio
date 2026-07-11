// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'
import { adminButton, adminSurface, adminTokens } from './classes'

const stylesSource = readFileSync(new URL('../styles.css', import.meta.url), 'utf8')
const mainSource = readFileSync(new URL('../main.tsx', import.meta.url), 'utf8')
const classesSource = readFileSync(new URL('./classes.ts', import.meta.url), 'utf8')
const componentsSource = readFileSync(new URL('../components.tsx', import.meta.url), 'utf8')
const dataTableSource = readFileSync(new URL('./dataTable.tsx', import.meta.url), 'utf8')
const loginSource = readFileSync(new URL('../pages/LoginPage.tsx', import.meta.url), 'utf8')
const packageSource = readFileSync(new URL('../../package.json', import.meta.url), 'utf8')

if (adminTokens.radius.xs !== '6px' || adminTokens.radius.sm !== '8px' || adminTokens.radius.md !== '12px' || adminTokens.radius.lg !== '12px') {
  throw new Error(`admin radius tokens should use the 6/8/12px Soft Grid Ops scale, got ${JSON.stringify(adminTokens.radius)}`)
}

if (adminSurface.card.includes('rounded-3xl') || adminSurface.lane.includes('rounded-3xl')) {
  throw new Error('admin surfaces should not default to rounded-3xl')
}

if (adminButton.primary.includes('30px') || adminButton.primary.includes('shadow-[')) {
  throw new Error(`primary button should not use large glow shadow, got ${adminButton.primary}`)
}

for (const dependency of ['@fontsource-variable/geist', '@fontsource-variable/geist-mono']) {
  if (!packageSource.includes(`"${dependency}"`) || !mainSource.includes(`'${dependency}`)) {
    throw new Error(`admin runtime must bundle and import ${dependency}`)
  }
}

for (const forbiddenFont of ['Inter', 'Fraunces', 'JetBrains Mono', 'fonts.googleapis.com']) {
  if (stylesSource.includes(forbiddenFont)) {
    throw new Error(`admin typography must not depend on ${forbiddenFont}`)
  }
}

for (const token of [
  '--admin-font-ui:',
  '--admin-font-mono:',
  '--admin-type-label: 11px',
  '--admin-type-support: 12px',
  '--admin-type-body: 14px',
  '--admin-type-section: 16px',
  '--admin-type-page: 24px',
  '--admin-motion-fast: 120ms',
  '--admin-motion-base: 180ms',
  '--admin-motion-slow: 240ms',
  '--pg-topbar-height: 64px',
  '--pg-sidebar-admin-width: 216px',
]) {
  if (!stylesSource.includes(token)) throw new Error(`admin design tokens must define ${token}`)
}

for (const forbiddenClass of ['rounded-2xl', 'rounded-3xl', 'tracking-tight', 'tracking-tighter', 'text-[10px]']) {
  if (classesSource.includes(forbiddenClass)) {
    throw new Error(`shared admin primitives must not use ${forbiddenClass}`)
  }
}

for (const [name, source] of [
  ['shared components', componentsSource],
  ['data table', dataTableSource],
  ['login page', loginSource],
] as const) {
  for (const forbiddenClass of ['rounded-2xl', 'rounded-3xl', 'uppercase', 'tracking-tight', 'tracking-tighter', 'tracking-wide', 'tracking-wider', 'tracking-widest', 'tracking-[', 'text-[10px]']) {
    if (source.includes(forbiddenClass)) {
      throw new Error(`${name} must use the shared radius and typography roles instead of ${forbiddenClass}`)
    }
  }
}
