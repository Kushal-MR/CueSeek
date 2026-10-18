import { useEffect, useRef, useState } from 'react'
import { Term } from './Term.tsx'
import { useInView, words } from './inView.ts'
import './Problem.css'

/**
 * Real services and their real default ports, so a self-hoster recognises them. Placed by
 * hand: a cluttered pile reads as "too many tabs", a grid would read as a menu.
 */
const CHIPS = [
  { name: 'Jellyfin', port: ':8096', x: 4, y: 2, r: -3 },
  { name: 'qBittorrent', port: ':8080', x: 40, y: 12, r: 2 },
  { name: 'Plex', port: ':32400', x: 12, y: 24, r: -1 },
  { name: 'Sonarr', port: ':8989', x: 50, y: 34, r: 4 },
  { name: 'Immich', port: ':2283', x: 2, y: 46, r: -4 },
  { name: 'Vaultwarden', port: ':8000', x: 36, y: 56, r: 1 },
  { name: 'Syncthing', port: ':8384', x: 10, y: 68, r: -2 },
  { name: '…and the server they all run on', port: '', x: 40, y: 78, r: 3 },
] as const

const HEADLINE = 'Your home server runs a dozen things.'

/**
 * Section 01. Written for someone who has never heard "self-hosted": the problem first,
 * the product only after.
 *
 * The chips arrive one by one like tabs being opened, until the column is cluttered. Scroll
 * on, and they collapse into a single row — which the next section turns into the phone's
 * dashboard. That collapse is the transition between the two.
 */
export function Problem() {
  const [ref, seen] = useInView<HTMLElement>(0.3)
  const end = useRef<HTMLDivElement>(null)
  const [collapsed, setCollapsed] = useState(false)

  // The collapse starts when the bottom of the section passes the middle of the screen —
  // and undoes itself on the way back up, so scrolling back finds the pile again.
  useEffect(() => {
    const el = end.current
    if (!el) return
    const io = new IntersectionObserver(
      ([e]) => setCollapsed(e.boundingClientRect.top < innerHeight * 0.5),
      { threshold: [0, 1], rootMargin: '0px 0px -50% 0px' },
    )
    io.observe(el)
    return () => io.disconnect()
  }, [])

  const [head, ...rest] = HEADLINE.split('home server')
  return (
    <section
      ref={ref}
      id="problem"
      className="problem"
      data-in={seen || undefined}
      data-collapsed={collapsed || undefined}
      aria-labelledby="problem-title"
    >
      <span className="hud">Section 01 — The problem</span>
      <div className="problem-grid">
        <div className="problem-copy">
          <h2 id="problem-title" className="kinetic">
            <span>
              {words(head.trim()).map(({ w, delay }) => (
                <span key={w + delay}>
                  <span className="w" style={{ animationDelay: `${delay}ms` }}>
                    {w}
                  </span>{' '}
                </span>
              ))}
              <span className="w" style={{ animationDelay: '140ms' }}>
                <Term means="a computer at home running things you'd otherwise rent: a film library, downloads, photo backup.">
                  home server
                </Term>
              </span>{' '}
              {words(rest.join('').trim(), 210).map(({ w, delay }) => (
                <span key={w + delay}>
                  <span className="w" style={{ animationDelay: `${delay}ms` }}>
                    {w}
                  </span>{' '}
                </span>
              ))}
            </span>
          </h2>
          <p className="problem-body">
            Each one has its own page, its own login, and its own way of saying something's
            wrong. Checking on all of them means opening every one.
          </p>
          <p className="problem-turn">
            You can't keep an app for every one of them — let alone reach them all from
            outside your home.
          </p>
        </div>

        <div className="chips" aria-hidden="true">
          {CHIPS.map((c, i) => (
            <span
              key={c.name}
              className="chip"
              style={
                {
                  '--x': `${c.x}%`,
                  '--y': `${c.y}%`,
                  '--r': `${c.r}deg`,
                  '--i': i,
                } as React.CSSProperties
              }
            >
              <i className="lock" />
              <b>{c.name}</b>
              {c.port && <small>{c.port}</small>}
            </span>
          ))}
          <span className="one-row">
            <i className="ok" />
            <b>Everything</b>
            <small>one answer</small>
          </span>
        </div>
      </div>
      <div ref={end} className="problem-end" aria-hidden="true" />
    </section>
  )
}
