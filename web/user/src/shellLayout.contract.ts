import { shellLayoutClasses } from './shellLayout'

const appLayout = shellLayoutClasses('app')
if (!appLayout.shell.includes('h-screen') || !appLayout.main.includes('overflow-y-auto')) {
  throw new Error(`app shell mode should preserve fixed-height scrolling, got ${JSON.stringify(appLayout)}`)
}

const documentLayout = shellLayoutClasses('document')
if (!documentLayout.shell.includes('min-h-screen') || !documentLayout.shell.includes('overflow-visible')) {
  throw new Error(`document shell mode should allow page-height layout, got ${JSON.stringify(documentLayout)}`)
}

if (!documentLayout.main.includes('min-h-screen') || !documentLayout.main.includes('overflow-visible')) {
  throw new Error(`document shell mode should expose profile content through normal page flow, got ${JSON.stringify(documentLayout)}`)
}
