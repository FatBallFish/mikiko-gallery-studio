import { modelIconSrc } from '../../shared/modelIcons'

// Renders a route model / group icon with the documented fallback chain:
// uploaded SVG → built-in vendor icon → site favicon mark. Sizing via the
// `size` prop keeps it usable in pickers, model rows and group headers.
export function ModelIcon({ iconKey, iconSvg, size = 20, alt, className }: { iconKey?: string; iconSvg?: string; size?: number; alt?: string; className?: string }) {
  return (
    <img
      src={modelIconSrc(iconKey, iconSvg)}
      alt={alt ?? ''}
      width={size}
      height={size}
      aria-hidden={alt ? undefined : true}
      className={className}
      style={{ width: size, height: size, objectFit: 'contain', flex: 'none' }}
      draggable={false}
    />
  )
}
