import { useEffect, useState } from 'react'
import { useInView } from '../sections/inView.ts'
import { LAND_EVENT, travellerActive } from './traveller.ts'

/**
 * A section's go signal: the travelling dot landing on its station.
 *
 * Never the only signal. If the dot is not running (reduced motion) the section starts as
 * soon as it is in view; if the dot is running but late — a fast scroll past, a slow device —
 * the section starts on its own shortly after, so nothing waits on the dot forever.
 */
export function useCue<T extends Element>(station: string, threshold = 0.3) {
  const [ref, inView] = useInView<T>(threshold)
  const [landed, setLanded] = useState(false)
  const [late, setLate] = useState(false)

  useEffect(() => {
    const on = (e: Event) => {
      if ((e as CustomEvent<string>).detail === station) setLanded(true)
    }
    addEventListener(LAND_EVENT, on)
    return () => removeEventListener(LAND_EVENT, on)
  }, [station])

  useEffect(() => {
    if (!inView) return
    const t = setTimeout(() => setLate(true), travellerActive ? 1600 : 0)
    return () => clearTimeout(t)
  }, [inView])

  return [ref, landed || late] as const
}
