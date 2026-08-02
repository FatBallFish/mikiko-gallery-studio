import { userShell } from './ui/classes'

const mikikoLogoUrl = new URL('./assets/mikiko-mark.svg', import.meta.url).href

export const siteBrand = {
  name: 'Mikiko Studio',
  logoUrl: mikikoLogoUrl,
} as const

export function BrandMark({ withText = false, inverse = false }: { withText?: boolean; inverse?: boolean }) {
  return (
    <span className={userShell.brandMark}>
      <img className={userShell.brandLogo} src={siteBrand.logoUrl} alt={siteBrand.name} />
      {withText ? <span className={userShell.brandName} style={inverse ? { color: '#fff' } : undefined}>{siteBrand.name}</span> : null}
    </span>
  )
}
