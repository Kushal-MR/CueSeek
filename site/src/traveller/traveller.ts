/**
 * The dot, travelling (M6.5b).
 *
 * The Swell's handle leaves the logo and follows the reader down the page. Scroll into a
 * section and it hops there along an arc, stretched by its speed; it lands with a squash on
 * that section's station, and the landing is the cue — the section's motion starts on impact.
 * Scroll back up and it hops back, station by station, into the logo's gap.
 *
 * **Wherever it goes, it has a job.** A section moves it around its own content with
 * `dotTo()` — tapping each service in turn, becoming a status dot, becoming the dashboard's
 * live indicator — and when it stops, it is a dot that means something there. It never just
 * sits in empty space.
 *
 * Stations are elements marked `data-station="<id>"` (`data-land="center"` or `"top"`);
 * home is the logo's handle. Sections listen for `cueseek:land` (see useCue).
 */

export const LAND_EVENT = 'cueseek:land'
const MOVE_EVENT = 'cueseek:dot'

/** Whether a traveller is running — sections fall back to their own trigger if not. */
export let travellerActive = false

/**
 * Which way the dot was going when it last landed on a station: `down` from a station above
 * (or from the logo), `up` from one below. A section reads it on landing to take its own
 * content in the reader's direction.
 */
export let arrivedGoing: 'down' | 'up' = 'down'

export interface DotMove {
  /** How long the hop takes, ms. Short hops between neighbours want ~250. */
  dur?: number
  /** Size relative to the logo's dot, to become a small indicator. */
  scale?: number
  /** Pulse like the app's freshness beat. */
  pulse?: boolean
  /** Keep following the target while it moves (a zooming screenshot). */
  track?: boolean
}

/** Send the dot to an element within the current section, where it will rest. */
export function dotTo(el: Element | null, move: DotMove = {}) {
  if (el) dispatchEvent(new CustomEvent(MOVE_EVENT, { detail: { el, ...move } }))
}

const DOT = 28 // px — the handle's diameter at hero size (r 7.5 of the 60-unit mark × 112/60)

interface Point {
  x: number
  y: number
}

/** A page position (not a viewport one), so a landed dot scrolls with its section. */
function pagePoint(el: Element, land?: string): Point {
  const r = el.getBoundingClientRect()
  const x = r.left + r.width / 2 + scrollX
  if (land === 'top') return { x, y: r.top + scrollY - DOT / 2 + 2 }
  if (land === 'center') return { x, y: r.top + r.height * 0.42 + scrollY }
  return { x, y: r.top + r.height / 2 + scrollY }
}

interface Flight {
  from: Point
  to: () => Point
  t0: number
  dur: number
  lift: number
  /** The station this flight ends at; for a hop within a section, the same station. */
  station: string | null
  /** A hop within a section, to rest on an element there. */
  within: boolean
  fromScale: number
  toScale: number
  pulse: boolean
}

export function startTraveller(dot: HTMLElement, body: HTMLElement): () => void {
  travellerActive = true
  document.documentElement.dataset.traveller = ''
  const home = () => document.querySelector<SVGGElement>('.hero-mark .handle-follow')
  const heroMark = () => document.querySelector<SVGSVGElement>('.hero-mark')

  /** The station the reader is at: the last one whose section has come well into view. */
  function current(): { id: string; el: HTMLElement } | null {
    let found: { id: string; el: HTMLElement } | null = null
    for (const el of document.querySelectorAll<HTMLElement>('[data-station]')) {
      const section = el.closest('section') ?? el
      if (section.getBoundingClientRect().top < innerHeight * 0.55) found = { id: el.dataset.station!, el }
    }
    return found
  }

  const pos: Point = { x: 0, y: 0 }
  const spring = { x: 0, y: 0, vx: 0, vy: 0, tx: 0, ty: 0 }
  let at: string | null = null // station id; null = home in the logo
  let rest: { el: Element; track: boolean } | null = null // what it rests on, if not its station
  let scale = 1
  let flight: Flight | null = null
  let raf = 0
  let still = 0

  function homePoint(): Point | null {
    const h = home()
    return h ? pagePoint(h) : null
  }
  function restPoint(): Point | null {
    if (rest) return pagePoint(rest.el)
    if (at === null) return homePoint()
    const st = document.querySelector<HTMLElement>(`[data-station="${at}"]`)
    return st ? pagePoint(st, st.dataset.land) : null
  }

  function setAway(away: boolean) {
    heroMark()?.toggleAttribute('data-dot-away', away)
    dot.toggleAttribute('data-shown', away)
  }

  function fly(
    to: () => Point,
    o: { station: string | null; within: boolean; dur?: number; scale?: number; pulse?: boolean },
  ) {
    const from = flight || at !== null ? { ...pos } : homePoint()
    if (!from) return
    const end = to()
    const dist = Math.hypot(end.x - from.x, end.y - from.y)
    flight = {
      from,
      to,
      t0: performance.now(),
      dur: o.dur ?? Math.min(1100, 420 + dist * 0.45),
      // A hop: the arc rises above both ends — more for a long jump, a little for a short one.
      lift: o.within ? Math.min(90, 18 + dist * 0.3) : Math.min(260, 60 + dist * 0.28),
      station: o.station,
      within: o.within,
      fromScale: scale,
      toScale: o.scale ?? 1,
      pulse: !!o.pulse,
    }
    pos.x = from.x
    pos.y = from.y
    body.classList.remove('pulse')
    setAway(true)
    wake()
  }

  /** To another station (or home). Whatever it was resting on there is left behind. */
  function launch(to: string | null) {
    rest = null
    const target = to === null ? null : current()
    // Coming up from below, a station may name a different element to land on — the end of
    // its content nearest the dot (`data-entry-up`, a selector within its section).
    going = order(to) < order(at) ? 'up' : 'down'
    let el: Element | null = target?.el ?? null
    if (target && going === 'up' && target.el.dataset.entryUp) {
      el = target.el.closest('section')?.querySelector(target.el.dataset.entryUp) ?? el
    }
    fly(() => (to === null || !el ? (homePoint() ?? pos) : pagePoint(el, el === target?.el ? target.el.dataset.land : undefined)), {
      station: to,
      within: false,
    })
  }

  /** A station's place down the page: home is 0, then stations in document order. */
  function order(id: string | null): number {
    if (id === null) return 0
    const all = [...document.querySelectorAll<HTMLElement>('[data-station]')].map((e) => e.dataset.station)
    return all.indexOf(id) + 1
  }
  let going: 'down' | 'up' = 'down'

  function landed(f: Flight) {
    flight = null
    at = f.station
    scale = f.toScale
    body.animate(
      [
        { transform: 'scale(1.35, 0.7)' },
        { transform: 'scale(0.88, 1.14)', offset: 0.4 },
        { transform: 'scale(1.05, 0.96)', offset: 0.75 },
        { transform: 'scale(1, 1)' },
      ],
      { duration: f.within ? 300 : 420, easing: 'cubic-bezier(0.3, 0, 0.2, 1)' },
    )
    if (f.within) {
      if (f.pulse) body.classList.add('pulse')
      dispatchEvent(new CustomEvent('cueseek:dot-arrived', { detail: rest?.el }))
    } else if (f.station === null) {
      setAway(false) // home: hand back to the logo's own handle, with its own spring and hop
    } else {
      arrivedGoing = going
      dispatchEvent(new CustomEvent(LAND_EVENT, { detail: f.station }))
    }
  }

  function frame(now: number) {
    // Cleared first, so that however this frame ends, a later wake() can always schedule the
    // next one — a stale handle here would freeze the dot until something else moved it.
    raf = 0
    let stretch = 1
    let angle = 0
    if (flight) {
      const f = flight
      const k = Math.min((now - f.t0) / f.dur, 1)
      const e = k < 0.5 ? 2 * k * k : 1 - (-2 * k + 2) ** 2 / 2
      const to = f.to() // follows its target if the page reflows mid-flight
      const cx = (f.from.x + to.x) / 2
      const cy = Math.min(f.from.y, to.y) - f.lift
      const u = 1 - e
      const nx = u * u * f.from.x + 2 * u * e * cx + e * e * to.x
      const ny = u * u * f.from.y + 2 * u * e * cy + e * e * to.y
      const speed = Math.hypot(nx - pos.x, ny - pos.y)
      angle = (Math.atan2(ny - pos.y, nx - pos.x) * 180) / Math.PI
      stretch = 1 + Math.min(speed / 28, 0.55)
      scale = f.fromScale + (f.toScale - f.fromScale) * e
      pos.x = nx
      pos.y = ny
      if (k >= 1) {
        landed(f)
        // The landing frame is the last one drawn before it rests: no flight stretch on it.
        stretch = 1
        angle = 0
      }
    } else {
      const p = restPoint()
      if (p) {
        still = Math.hypot(p.x - pos.x, p.y - pos.y) < 0.2 ? still + 1 : 0
        pos.x = p.x
        pos.y = p.y
      }
      // At rest: the same spring as in the logo — smaller when it has become an indicator.
      spring.vx = (spring.vx + (spring.tx * scale - spring.x) * 0.11) * 0.8
      spring.vy = (spring.vy + (spring.ty * scale - spring.y) * 0.11) * 0.8
      spring.x += spring.vx
      spring.y += spring.vy
      const sp = Math.hypot(spring.vx, spring.vy)
      angle = (Math.atan2(spring.vy, spring.vx) * 180) / Math.PI
      stretch = 1 + Math.min(sp * 0.09, 0.5)
    }
    const x = pos.x + (flight ? 0 : spring.x) - DOT / 2
    const y = pos.y + (flight ? 0 : spring.y) - DOT / 2
    dot.style.transform =
      `translate(${x.toFixed(1)}px, ${y.toFixed(1)}px) ` +
      `rotate(${angle}deg) scale(${stretch * scale}, ${scale / stretch}) rotate(${-angle}deg)`

    const springing =
      Math.hypot(spring.vx, spring.vy) > 0.01 ||
      Math.hypot(spring.tx * scale - spring.x, spring.ty * scale - spring.y) > 0.05
    // Tracking a moving target keeps frames coming until it has been still a while.
    const tracking = !!rest?.track && still < 30
    raf = flight || springing || tracking ? requestAnimationFrame(frame) : 0
  }
  function wake() {
    still = 0
    if (!raf) raf = requestAnimationFrame(frame)
  }

  /** Decide where the dot should be, and send it if that has changed. */
  let checking = false
  function check() {
    if (checking) return
    checking = true
    requestAnimationFrame(() => {
      checking = false
      const want = current()?.id ?? null
      const heading = flight && !flight.within ? flight.station : at
      if (want !== heading) launch(want)
      else wake() // a reflow may have moved what it rests on
    })
  }

  function onMove(e: Event) {
    const m = (e as CustomEvent<DotMove & { el: Element }>).detail
    // Only within the section it is in, and not while it is crossing between sections.
    if (at === null || (flight && !flight.within)) return
    if (!m.el.closest('section')?.querySelector(`[data-station="${at}"]`)) return
    // Always follow what it sits on until that has stopped moving: a row still growing into
    // place, a screenshot mid-zoom. Measuring once left it beside its target.
    rest = { el: m.el, track: m.track ?? true }
    fly(() => pagePoint(m.el), { station: at, within: true, dur: m.dur, scale: m.scale, pulse: m.pulse })
  }

  function onPointer(e: PointerEvent) {
    if (at === null || flight) return
    const dx = e.pageX - pos.x
    const dy = e.pageY - pos.y
    const d = Math.hypot(dx, dy)
    const reach = 16 * (1 - Math.exp(-d / 60))
    spring.tx = d ? (dx / d) * reach : 0
    spring.ty = d ? (dy / d) * reach : 0
    wake()
  }
  function onPoke() {
    spring.vx += 3
    spring.vy -= 6
    wake()
  }

  addEventListener('scroll', check, { passive: true })
  addEventListener('resize', check)
  addEventListener('pointermove', onPointer)
  addEventListener(MOVE_EVENT, onMove)
  // A zoom or other transition can move what the dot sits on.
  document.addEventListener('transitionrun', wake)
  dot.addEventListener('pointerdown', onPoke)
  check()

  return () => {
    travellerActive = false
    delete document.documentElement.dataset.traveller
    cancelAnimationFrame(raf)
    removeEventListener('scroll', check)
    removeEventListener('resize', check)
    removeEventListener('pointermove', onPointer)
    removeEventListener(MOVE_EVENT, onMove)
    document.removeEventListener('transitionrun', wake)
    dot.removeEventListener('pointerdown', onPoke)
    setAway(false)
  }
}
