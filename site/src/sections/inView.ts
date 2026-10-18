import { useEffect, useRef, useState } from 'react'

/**
 * True once the element has come far enough into view — and stays true. Sections use it to
 * *start* their motion: a scroll triggers a transition, which then plays at its own pace.
 * Nothing on this site is scrubbed by the scroll position (M6 plan, "Scroll").
 */
export function useInView<T extends Element>(threshold = 0.35) {
  const ref = useRef<T>(null)
  const [seen, setSeen] = useState(false)
  useEffect(() => {
    const el = ref.current
    if (!el || seen) return
    const io = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setSeen(true)
          io.disconnect()
        }
      },
      { threshold },
    )
    io.observe(el)
    return () => io.disconnect()
  }, [seen, threshold])
  return [ref, seen] as const
}

/** Splits a line into words that can be staggered in, keeping it one sentence for readers. */
export function words(text: string, from = 0, step = 70) {
  return text.split(' ').map((w, i) => ({ w, delay: from + i * step }))
}
