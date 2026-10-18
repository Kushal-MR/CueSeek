import { useEffect, useRef, useState } from 'react'
import { Term } from './Term.tsx'
import { words } from './inView.ts'
import { useCue } from '../traveller/useCue.ts'
import { LAND_EVENT, arrivedGoing, dotTo, travellerActive } from '../traveller/traveller.ts'
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
  // The tour's controls, for the collapse to use: only one tour may ever run at a time.
  const tourCtl = useRef<{ start: (going: 'down' | 'up', startAt: 0 | 1) => void; stop: () => void; busy: () => boolean } | null>(null)

  // The dot's job here: it plays someone checking their server. It falls onto the first
  // service and each one opens as the dot taps it, top to bottom, a little quicker each time
  // — "opening every one of them". When the pile collapses into a single row, it becomes
  // that row's status dot: one answer.
  useEffect(() => {
    const all = () => [...(chips.current?.querySelectorAll<HTMLElement>('.chip') ?? [])]
    const row = () => chips.current?.querySelector('.one-row .ok') ?? null
    const timers: number[] = []
    const later = (ms: number, f: () => void) => timers.push(window.setTimeout(f, ms))
    const open = (chip: HTMLElement) => chip.classList.add('opened')
    const openAll = () => all().forEach(open)
    const isCollapsed = () => !!chips.current?.closest('.problem')?.hasAttribute('data-collapsed')

    // The tour is a chain through the services in a given order: the next hop starts only
    // when the dot has really arrived, so slow frames can never make it skip one or run
    // ahead of itself. Down the pile when the reader arrives from above, up it from below.
    let route: HTMLElement[] = []
    let next = -1
    const lockOf = (chip: Element) => chip.querySelector('.lock')
    const hop = () => {
      if (next < 0 || next >= route.length) return
      dotTo(lockOf(route[next]), { dur: Math.max(200, 330 - next * 18), scale: 0.62 })
    }
    const tour = (going: 'down' | 'up', startAt: 0 | 1) => {
      route = going === 'down' ? all() : all().reverse()
      next = startAt
      if (startAt === 1) {
        open(route[0]) // it has just landed on the first of them
        tap(route[0])
      }
      later(startAt === 1 ? 240 : 0, hop)
    }
    const tap = (chip: HTMLElement) => {
      chip.classList.remove('tapped')
      void chip.offsetWidth
      chip.classList.add('tapped')
    }
    tourCtl.current = { start: tour, stop: () => { next = -1 }, busy: () => next >= 0 }

    // A chip opens as the dot lands on its lock — or flinches, if it is already open.
    const onArrive = (e: Event) => {
      const lock = (e as CustomEvent<Element>).detail
      const chip = lock instanceof Element ? lock.closest('.chip') : null
      if (!(chip instanceof HTMLElement)) return
      if (!chip.classList.contains('opened')) open(chip)
      else tap(chip)
      if (next >= 0 && chip === route[next]) {
        next += 1
        if (next < route.length) later(110, hop)
        else next = -1 // done: it rests on the last one it reached
      }
    }

    const onLand = (e: Event) => {
      if ((e as CustomEvent<string>).detail !== 'problem') return
      const first = !toured.current
      toured.current = true
      if (!first) openAll()
      // Collapsed already: there is one row, and the dot is its status light.
      if (isCollapsed()) {
        next = -1
        return later(150, () => dotTo(row(), { scale: 0.36 }))
      }
      tour(arrivedGoing, 1)
    }

    addEventListener(LAND_EVENT, onLand)
    addEventListener('cueseek:dot-arrived', onArrive)
    return () => {
      removeEventListener(LAND_EVENT, onLand)
      removeEventListener('cueseek:dot-arrived', onArrive)
      timers.forEach(clearTimeout)
    }
  }, [])

  // Without the dot — reduced motion, or a cue that came late — the services open on their own.
  useEffect(() => {
    if (!seen) return
    // With the dot running, its tour opens them; this only catches a tour that never began
    // (the reader scrolled straight past).
    const t = setTimeout(
      () => {
        if (travellerActive && toured.current) return
        chips.current?.querySelectorAll('.chip').forEach((c) => c.classList.add('opened'))
      },
      travellerActive ? 6000 : 0,
    )
    return () => clearTimeout(t)
  }, [seen])

  // Collapsing gathers the pile into one row; the dot follows and becomes its status light.
  // Un-collapsing happens on the way back up, so the dot climbs the pile from the bottom.
  useEffect(() => {
    if (!toured.current) return
    const t = setTimeout(
      () => {
        if (collapsed) {
          tourCtl.current?.stop() // a tour in progress gives way to the collapse
          dotTo(chips.current?.querySelector('.one-row .ok') ?? null, { scale: 0.36, dur: 520 })
        } else if (!tourCtl.current?.busy()) {
          // Climbing back up: unless a tour is already doing so — two would fight.
          tourCtl.current?.start('up', 0)
        }
      },
      collapsed ? 420 : 250,
    )
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

        <div ref={chips} className="chips" aria-hidden="true">
          {CHIPS.map((c, i) => (
            <span
              key={c.name}
              className="chip"
              data-last={i === CHIPS.length - 1 || undefined}
              // The dot falls onto the first service: it is this section's station.
              style={
                {
                  '--x': `${c.x}%`,
                  '--y': `${c.y}%`,
                  '--r': `${c.r}deg`,
                  '--i': i,
                } as React.CSSProperties
              }
            >
              {/* The dot lands on the lock to open the service; the first is the station. */}
              <i
                className="lock"
                data-station={i === 0 ? 'problem' : undefined}
                // Arriving from below, it lands on the last service instead, and climbs.
                data-entry-up={i === 0 ? '.chip[data-last] .lock' : undefined}
              />
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
