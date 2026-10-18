import { useEffect, useRef } from 'react'
import { arriving } from '../arrival/arriving.ts'
import { startTraveller } from './traveller.ts'
import './Traveller.css'

/**
 * The travelling dot. Rendered once, at the top of the page. It waits for the arrival's pop
 * to finish — the dot has to be born in the logo before it can leave it — and never runs
 * with reduced motion, where the dot simply stays in the logo.
 */
export function Traveller() {
  const dot = useRef<HTMLDivElement>(null)
  const body = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (matchMedia('(prefers-reduced-motion: reduce)').matches) return
    let stop: (() => void) | undefined
    const timer = setTimeout(
      () => {
        if (dot.current && body.current) stop = startTraveller(dot.current, body.current)
      },
      arriving ? 2000 : 0,
    )
    return () => {
      clearTimeout(timer)
      stop?.()
    }
  }, [])

  return (
    <div ref={dot} className="traveller" aria-hidden="true">
      <div ref={body} className="traveller-body" />
    </div>
  )
}
