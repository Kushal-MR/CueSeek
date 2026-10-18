import { useEffect, useRef, useState } from 'react'
import { Term } from './Term.tsx'
import { words } from './inView.ts'
import { useCue } from '../traveller/useCue.ts'
import { LAND_EVENT, dotTo } from '../traveller/traveller.ts'
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
  // Starts when the travelling dot lands on the pile (M6.5b).
  const [ref, seen] = useCue<HTMLElement>('problem', 0.3)
  const [collapsed, setCollapsed] = useState(false)
  const chips = useRef<HTMLDivElement>(null)
  const toured = useRef(false)

  // The dot's job here: it plays someone checking their server — tapping one service, then
  // the next, faster and faster — "opening every one of them". When the pile collapses into
  // a single row, it becomes that row's status dot: one answer.
  useEffect(() => {
    const all = () => [...(chips.current?.querySelectorAll<HTMLElement>('.chip') ?? [])]
    const row = () => chips.current?.querySelector('.one-row .ok') ?? null
    const timers: number[] = []
    const later = (ms: number, f: () => void) => timers.push(window.setTimeout(f, ms))

    // A chip flinches as it is tapped.
    const onArrive = (e: Event) => {
      const el = (e as CustomEvent<Element>).detail
      if (!(el instanceof HTMLElement) || !el.classList.contains('chip')) return
      el.classList.remove('tapped')
      void el.offsetWidth
      el.classList.add('tapped')
    }

    const settle = () => {
      const list = all()
      dotTo(chips.current?.closest('.problem')?.hasAttribute('data-collapsed') ? row() : list[list.length - 1], {
        scale: chips.current?.closest('.problem')?.hasAttribute('data-collapsed') ? 0.36 : 1,
      })
    }

    const onLand = (e: Event) => {
      if ((e as CustomEvent<string>).detail !== 'problem') return
      if (toured.current) return later(150, settle)
      toured.current = true
      // After the chips have flown in, tour them, each hop a little quicker than the last.
      let t = 2500
      all().forEach((chip, i) => {
        const dur = Math.max(170, 330 - i * 24)
        later(t, () => dotTo(chip, { dur }))
        t += dur + 70
      })
    }

    addEventListener(LAND_EVENT, onLand)
    addEventListener('cueseek:dot-arrived', onArrive)
    return () => {
      removeEventListener(LAND_EVENT, onLand)
      removeEventListener('cueseek:dot-arrived', onArrive)
      timers.forEach(clearTimeout)
    }
  }, [])

  // Collapsing gathers the pile into one row; the dot follows and becomes its status dot.
  useEffect(() => {
    if (!toured.current) return
    const t = setTimeout(() => {
      const list = chips.current?.querySelectorAll('.chip')
      dotTo(collapsed ? (chips.current?.querySelector('.one-row .ok') ?? null) : (list?.[list.length - 1] ?? null), {
        scale: collapsed ? 0.36 : 1,
        dur: 520,
      })
    }, collapsed ? 420 : 200)
    return () => clearTimeout(t)
  }, [collapsed])

  // The collapse starts once the pile itself has scrolled up past the upper part of the
  // screen — and undoes itself on the way back down, so scrolling back finds the pile again.
  useEffect(() => {
    const check = () => {
      const c = chips.current
      if (c) setCollapsed(c.getBoundingClientRect().bottom < innerHeight * 0.42)
    }
    addEventListener('scroll', check, { passive: true })
    const first = requestAnimationFrame(check)
    return () => {
      removeEventListener('scroll', check)
      cancelAnimationFrame(first)
    }
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

        <div ref={chips} className="chips" aria-hidden="true" data-station="problem" data-land="center">
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
    </section>
  )
}
