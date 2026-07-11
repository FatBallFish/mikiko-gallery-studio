import { prefersReducedMotion, routeTransitionClass } from './motion'
import { luminousMotion } from './luminousVault'

if (!prefersReducedMotion({ matches: true })) {
  throw new Error('reduced-motion media preference should be detected')
}

if (prefersReducedMotion({ matches: false })) {
  throw new Error('motion should remain enabled when no reduced-motion preference exists')
}

const animatedRoute = routeTransitionClass(false)
if (animatedRoute !== 'pg-route-enter') {
  throw new Error(`route content should use the dedicated pg-route-enter animation, got ${animatedRoute}`)
}

const reducedRoute = routeTransitionClass(true)
if (reducedRoute !== '') {
  throw new Error(`reduced-motion route content should not animate, got ${reducedRoute}`)
}

if (luminousMotion.routeMs !== 220 || luminousMotion.routeMs < 180 || luminousMotion.routeMs > 260) {
  throw new Error(`route transition duration must remain the canonical 220ms, got ${luminousMotion.routeMs}`)
}
