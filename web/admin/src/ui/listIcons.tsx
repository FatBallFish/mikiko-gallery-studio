import type { LucideIcon, LucideProps } from 'lucide-react'
import { Filter, X, ChevronUp } from 'lucide-react'

function withDefaults(Icon: LucideIcon) {
  return function ListIcon({ strokeWidth = 1.5, ...props }: LucideProps) {
    return <Icon strokeWidth={strokeWidth} {...props} />
  }
}

export const FilterIcon = withDefaults(Filter)
export const XIcon = withDefaults(X)
export const ChevronUpIcon = withDefaults(ChevronUp)