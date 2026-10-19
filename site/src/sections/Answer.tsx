import { useEffect, useRef, useState } from 'react'
// M6.6's capture against the staged library, with the host renamed to HomeServer — a public
// page should not carry a real machine's name.
import dashboard from '../assets/dashboard.webp'
import { Term } from './Term.tsx'
import { useCue } from '../traveller/useCue.ts'
import { LAND_EVENT, dotTo } from '../traveller/traveller.ts'
import './Answer.css'

/** Where each part sits on the screenshot, as a fraction of its height. */
const PARTS = [
  { id: 'verdict', label: 'The verdict', y: 0.22, says: 'One word for everything: Operational, or what needs you.' },
  { id: 'server', label: 'The server', y: 0.5, says: 'The computer itself — processor, memory, disk, temperature.' },
  { id: 'services', label: 'Each service', y: 0.81, says: 'Every service, its own state, and a menu to act on it.' },
] as const
type Part = (typeof PARTS)[number]['id']

/**
 * Section 02. What CueSeek is, in one picture: the phone's dashboard.
 *
 * The camera arrives close on the service rows — where section 01's collapsed row landed —
 * and pulls back to show the whole phone. Then it visits each part in turn as its label
 * draws on. Hovering or focusing a label sends the camera back to that part. Every move is
 * started by a trigger and runs at its own pace; none is tied to the scroll position.
 */
export function Answer() {
  // Starts when the travelling dot lands on the phone (M6.5b).
  const [ref, seen] = useCue<HTMLElement>('answer', 0.35)
  // With reduced motion there is no tour: the whole phone, still, from the start.
  const [still] = useState(() => matchMedia('(prefers-reduced-motion: reduce)').matches)
  const [focus, setFocus] = useState<Part | null>(still ? null : 'services')
  const [touring, setTouring] = useState(!still)

  useEffect(() => {
    if (!seen || still) return
    const steps: [number, Part | null][] = [
      [300, null], // pull back to the whole phone
      [1500, 'verdict'],
      [2500, 'server'],
      [3500, 'services'],
      [4500, null],
    ]
    const timers = steps.map(([t, f]) => setTimeout(() => setFocus(f), t))
    const done = setTimeout(() => setTouring(false), 4600)
    return () => [...timers, done].forEach(clearTimeout)
  }, [seen, still])

  // The dot's job here: it becomes the dashboard's own "live" dot — the app's heartbeat —
  // and rides along on it while the camera moves around the screen.
  const live = useRef<HTMLSpanElement>(null)
  useEffect(() => {
    let first = true
    let t = 0
    const onLand = (e: Event) => {
      if ((e as CustomEvent<string>).detail !== 'answer') return
      t = window.setTimeout(
        () => dotTo(live.current, { scale: 0.34, pulse: true, track: true, dur: 650 }),
        first ? 1250 : 200, // the first time, after the camera has pulled back
      )
      first = false
    }
    addEventListener(LAND_EVENT, onLand)
    return () => {
      removeEventListener(LAND_EVENT, onLand)
      clearTimeout(t)
    }
  }, [])

  const part = PARTS.find((p) => p.id === focus)
  return (
    <section
      ref={ref}
      id="answer"
      className="answer"
      data-in={seen || undefined}
      aria-labelledby="answer-title"
    >
      <span className="hud">Section 02 — The answer</span>
      <div className="answer-grid">
        <div className="answer-copy">
          <h2 id="answer-title">CueSeek answers for you.</h2>
          <p>
            One app for all of them. A small{' '}
            <Term means="a small program on your server that does the asking. It never runs as root.">
              agent
            </Term>{' '}
            on your server checks every service — and the server itself — and your phone gets
            one answer.
          </p>
          <p>
            When something needs you, it's a couple of taps to restart it — from the sofa, or
            from the other side of the world over your{' '}
            <Term means="Tailscale: a network between your own devices. The server is never on the open internet.">
              private network
            </Term>
            .
          </p>
        </div>

        <div className="stage" data-station="answer" data-land="top">
          <div
            className="shot"
            style={
              {
                '--fy': part ? part.y : 0.5,
                '--zoom': part ? (focus === 'services' && touring && !seen ? 2.2 : 1.18) : 1,
              } as React.CSSProperties
            }
            data-entry={!seen || undefined}
          >
            <div className="screen">
              <img
                src={dashboard}
                alt="The CueSeek dashboard: the server reporting Operational, its processor, memory, disk and temperature, with Jellyfin playing a film and qBittorrent downloading."
                width={1272}
                height={1460}
              />
              {/* Where the screenshot's own "live" dot is. */}
              <span ref={live} className="live-spot" aria-hidden="true" />
            </div>
          </div>
          <ul className="callouts">
            {PARTS.map((p, i) => (
              <li
                key={p.id}
                style={{ '--y': p.y, '--i': i } as React.CSSProperties}
                data-on={focus === p.id || undefined}
              >
                <button
                  type="button"
                  onMouseEnter={() => !touring && setFocus(p.id)}
                  onMouseLeave={() => !touring && setFocus(null)}
                  onFocus={() => !touring && setFocus(p.id)}
                  onBlur={() => !touring && setFocus(null)}
                  aria-describedby={`says-${p.id}`}
                >
                  {p.label}
                </button>
                <span id={`says-${p.id}`} className="says">
                  {p.says}
                </span>
              </li>
            ))}
          </ul>
        </div>
      </div>

      <div className="flow" aria-label="Your server's agent talks to your phone and watch over your private network.">
        <span className="node">your server · agent</span>
        <span className="flow-wire">
          <span className="flow-label">private network</span>
          <i className="flow-packet" />
        </span>
        <span className="node">phone</span>
        <span className="node">watch</span>
      </div>
    </section>
  )
}
