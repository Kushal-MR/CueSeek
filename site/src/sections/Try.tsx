import { useEffect, useRef } from 'react'
import { useCue } from '../traveller/useCue.ts'
import { LAND_EVENT, dotTo } from '../traveller/traveller.ts'
import './Try.css'

/**
 * Section 07. The band that leads to the demo page: a pretend server to break, in the
 * browser. The dot's job here: it becomes the button's light — the way in.
 */
export function Try() {
  const [ref, seen] = useCue<HTMLElement>('try', 0.4)
  const light = useRef<HTMLSpanElement>(null)

  useEffect(() => {
    const onLand = (e: Event) => {
      if ((e as CustomEvent<string>).detail === 'try') dotTo(light.current, { scale: 0.36, pulse: true, dur: 300 })
    }
    addEventListener(LAND_EVENT, onLand)
    return () => removeEventListener(LAND_EVENT, onLand)
  }, [])

  return (
    <section ref={ref} id="try" className="try" data-in={seen || undefined} aria-labelledby="try-title">
      <span className="hud">Section 07 — Try it</span>
      <div className="try-band">
        <div className="try-rule" aria-hidden="true" />
        <div className="try-copy">
          <h2 id="try-title">Break a server. Nothing will happen.</h2>
          <p>A pretend home server, running in your browser. Make a service fail, restart it, try to shut the machine down.</p>
          <a className="try-go" href="./demo" data-station="try">
            <span ref={light} className="try-light" aria-hidden="true" />
            Open the demo <span aria-hidden="true">→</span>
          </a>
        </div>
        <div className="try-rule" aria-hidden="true" />
      </div>
    </section>
  )
}
