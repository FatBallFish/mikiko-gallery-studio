import { resetShellScroll, shellActiveNavIndex, shellChromeClasses, shellLayoutClasses } from './shellLayout'
import { readFileSync } from 'node:fs'

const shellSource = readFileSync(new URL('./components.tsx', import.meta.url), 'utf8')
if (!shellSource.includes('resetShellScroll(scrollMode, mainScrollRef.current, window)')) {
  throw new Error('Shell must route scroll resets to the active app or document scroller')
}

const appLayout = shellLayoutClasses('app')
if (!appLayout.shell.includes('h-screen') || !appLayout.main.includes('overflow-y-auto')) {
  throw new Error(`app shell mode should preserve fixed-height scrolling, got ${JSON.stringify(appLayout)}`)
}

if (!shellChromeClasses.sidebar.includes('w-[108px]')) {
  throw new Error(`user sidebar must use the canonical 108px width, got ${shellChromeClasses.sidebar}`)
}

if (!shellChromeClasses.topbar.includes('h-[76px]')) {
  throw new Error(`user topbar must use the canonical 76px height, got ${shellChromeClasses.topbar}`)
}

if (!shellChromeClasses.mobileNav.includes('fixed') || !shellChromeClasses.mobileNav.includes('bottom-0')) {
  throw new Error(`mobile navigation must remain reachable at the bottom, got ${shellChromeClasses.mobileNav}`)
}
if (!shellChromeClasses.mobileNav.includes('grid-cols-5')) {
  throw new Error(`mobile navigation must reserve exactly five stable columns, got ${shellChromeClasses.mobileNav}`)
}

if (appLayout.content !== 'pg-route-enter') {
  throw new Error(`only route content should receive the canonical transition, got ${appLayout.content}`)
}

const reducedLayout = shellLayoutClasses('app', true)
if (reducedLayout.content !== '') {
  throw new Error(`route content must disable motion when requested, got ${reducedLayout.content}`)
}

const primaryNavigation = [
  { route: 'home' as const },
  { route: 'genpic' as const },
  { route: 'gallery' as const },
]
if (shellActiveNavIndex('genpic', primaryNavigation) !== 1) {
  throw new Error('primary navigation routes should resolve to their real indicator position')
}
for (const route of ['profile', 'public-gallery'] as const) {
  if (shellActiveNavIndex(route, primaryNavigation) !== -1) {
    throw new Error(`${route} must not display a false primary-navigation indicator`)
  }
}

const scrollTarget = { scrollTop: 384 }
const appWindowCalls: Array<[number, number]> = []
if (resetShellScroll.length < 3) throw new Error('shell scroll reset must distinguish app and document scrolling')
resetShellScroll('app', scrollTarget, { scrollTo: (x, y) => appWindowCalls.push([x, y]) })
if (scrollTarget.scrollTop !== 0 || appWindowCalls.length !== 0) {
  throw new Error(`app route changes must only reset persistent main scroll, got ${JSON.stringify({ scrollTarget, appWindowCalls })}`)
}

const documentTarget = { scrollTop: 384 }
const documentWindowCalls: Array<[number, number]> = []
resetShellScroll('document', documentTarget, { scrollTo: (x, y) => documentWindowCalls.push([x, y]) })
if (documentTarget.scrollTop !== 384 || JSON.stringify(documentWindowCalls) !== '[[0,0]]') {
  throw new Error(`document route changes must reset window scroll, got ${JSON.stringify({ documentTarget, documentWindowCalls })}`)
}

const documentLayout = shellLayoutClasses('document')
if (!documentLayout.shell.includes('min-h-screen') || !documentLayout.shell.includes('overflow-visible')) {
  throw new Error(`document shell mode should allow page-height layout, got ${JSON.stringify(documentLayout)}`)
}

if (!documentLayout.main.includes('min-h-screen') || !documentLayout.main.includes('overflow-visible')) {
  throw new Error(`document shell mode should expose profile content through normal page flow, got ${JSON.stringify(documentLayout)}`)
}
