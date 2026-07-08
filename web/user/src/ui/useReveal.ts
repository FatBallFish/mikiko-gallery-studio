import { useEffect, useRef, useState } from 'react'

/**
 * Lightweight scroll-reveal hook using IntersectionObserver.
 * Honors prefers-reduced-motion by keeping visible=true.
 *
 * 重要:必须在 ref 绑定到真实 DOM 后再 setVisible(false),否则当
 * 父组件先 return <LoadingState /> 再渲染真实内容时,visible 会卡在
 * false 永远隐藏(已修过的回归 bug)。
 *
 * Spec section 14: 动效用于反馈与节奏, 不用于炫技.
 */
export function useReveal<T extends HTMLElement = HTMLDivElement>(options: {
  threshold?: number
  rootMargin?: string
  once?: boolean
} = {}) {
  const { threshold = 0.15, rootMargin = '0px 0px -10% 0px', once = true } = options
  const ref = useRef<T | null>(null)
  const [visible, setVisible] = useState(true)

  useEffect(() => {
    if (typeof window === 'undefined' || !('IntersectionObserver' in window)) return
    const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    if (reduceMotion) {
      setVisible(true)
      return
    }
    const node = ref.current
    // 关键修复:node 不存在时不要 setVisible(false)。
    // 父组件先 return LoadingState 再切真实内容时,这里 node 为 null,
    // 若先 setVisible(false) 再 return,effect 不会因 deps 变化重跑,
    // 元素会永久隐藏。保留 visible=true,等下次渲染再触发 effect。
    if (!node) return
    setVisible(false)
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            setVisible(true)
            if (once) observer.disconnect()
          } else if (!once) {
            setVisible(false)
          }
        }
      },
      { threshold, rootMargin },
    )
    observer.observe(node)
    return () => observer.disconnect()
  })

  return { ref, visible }
}

/**
 * Returns a style object for staggered reveal animation delay.
 * Usage: style={revealStyle(visible, index * 60)}
 */
export function revealStyle(visible: boolean, delayMs = 0): React.CSSProperties {
  if (!visible) {
    return { opacity: 0, transform: 'translate3d(0, 16px, 0)' }
  }
  return {
    opacity: 1,
    transform: 'translate3d(0, 0, 0)',
    transition: `opacity 480ms var(--pg-ease-out) ${delayMs}ms, transform 480ms var(--pg-ease-out) ${delayMs}ms`,
  }
}