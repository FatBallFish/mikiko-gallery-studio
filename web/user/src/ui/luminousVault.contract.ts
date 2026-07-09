import { luminousMotion, luminousRadii, luminousShell, luminousType } from './luminousVault'

if (JSON.stringify(luminousRadii) !== JSON.stringify({ sm: 8, md: 12, lg: 16, xl: 24 })) {
  throw new Error(`unexpected radius scale: ${JSON.stringify(luminousRadii)}`)
}

if (luminousMotion.routeMs < 180 || luminousMotion.routeMs > 260) {
  throw new Error('route motion is outside the approved range')
}

if (luminousShell.sidebarPx !== 108 || luminousShell.topbarPx !== 76) {
  throw new Error('shell geometry drifted')
}

if (!luminousType.display.includes('Satoshi') || !luminousType.ui.includes('sans-serif')) {
  throw new Error('typography stacks are incomplete')
}
