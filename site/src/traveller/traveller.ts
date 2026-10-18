/**
 * The dot, travelling (M6.5b).
 *
 * The Swell's handle leaves the logo and follows the reader down the page. Scroll into a
 * section and it hops there along an arc, stretched by its speed; it lands with a squash on
 * that section's station, and the landing is the cue — the section's motion starts on
 * impact. Scroll back up and it hops back, station by station, into the logo's gap.
 *
 * Stations are any element marked `data-station="<id>"` (with `data-land="center"` or
 * `"top"`); home is the logo's handle. A section that wants to be cued listens for the
 * `cueseek:land` event (see useCue). New sections join by adding the attribute — nothing
 * here needs to know them.
 *
 * Resting anywhere, it behaves as it does in the logo: it leans toward the pointer on a
 * spring and jumps when poked.
 */

export const LAND_EVENT = 'cueseek:land'

/** Whether a traveller is running — sections fall back to their own trigger if not. */
export let travellerActive = false

const DOT = 28 // px — the handle's diameter at hero size (r 7.5 of the 60-unit mark × 112/60)

interface Point {
  x: number
  y: number
}

/** A page position (not a viewport one), so a landed dot scrolls with its section. */
function pagePoint(el: Element, land: string | undefined): Point {
  const r = el.getBoundingClientRect()
  const x = r.left + r.width / 2 + scrollX
  if (land === 'top') return { x, y: r.top + scrollY - DOT / 2 + 2 }
  return { x, y: r.top + r.height * 0.42 + scrollY }
}

export function startTraveller(dot: HTMLElement, body: HTMLElement): () => void {
  travellerActive = true
  const home = () => document.querySelector<SVGGElement>('.hero-mark .handle-follow')
  const heroMark = () => document.querySelector<SVGSVGElement>('.hero-mark')

  /** The station the reader is at: the last one whose section has come well into view. */
  function current(): { id: string; el: Element; land?: string } | null {
    let found: { id: string; el: Element; land?: string } | null = null
    for (const el of document.querySelectorAll<HTMLElement>('[data-station]')) {
      const section = el.closest('section') ?? el
      if (section.getBoundingClientRect().top < innerHeight * 0.55) {
        found = { id: el.dataset.station!, el, land: el.dataset.land }
      }
    }
    return found
  }

  const pos: Point = { x: 0, y: 0 }
  const spring = { x: 0, y: 0, vx: 0, vy: 0, tx: 0, ty: 0 }
  let at: string | null = null // station id; null = home in the logo
  let flight: null | { from: Point; to: () => Point; c: Point; t0: number; dur: number; id: string | null } = null
  let raf = 0

  function homePoint(): Point | null {
    const h = home()
    if (!h) return null
    const r = h.getBoundingClientRect()
    return { x: r.left + r.width / 2 + scrollX, y: r.top + r.height / 2 + scrollY }
  }

  function setAway(away: boolean) {
    heroMark()?.toggleAttribute('data-dot-away', away)
    dot.toggleAttribute('data-shown', away)
  }

  function launch(to: string | null) {
    const target = to === null ? null : current()
    const from = flight ? { ...pos } : at === null ? homePoint() : { ...pos }
    if (!from) return
    const dest = () =>
      to === null ? (homePoint() ?? from) : pagePoint(target!.el, target!.land)
    const end = dest()
    const dist = Math.hypot(end.x - from.x, end.y - from.y)
    // A hop: the arc rises above both ends, more for a longer jump.
    const lift = Math.min(260, 60 + dist * 0.28)
    const c = { x: (from.x + end.x) / 2, y: Math.min(from.y, end.y) - lift }
    flight = { from, to: dest, c, t0: performance.now(), dur: Math.min(1100, 420 + dist * 0.45), id: to }
    pos.x = from.x
    pos.y = from.y
    setAway(true)
    wake()
  }

  function land(id: string | null) {
    flight = null
    at = id
    // The squash of a landing, on the inner body so it composes with the travel transform.
    body.animate(
      [
        { transform: 'scale(1.35, 0.7)' },
        { transform: 'scale(0.88, 1.14)', offset: 0.4 },
        { transform: 'scale(1.05, 0.96)', offset: 0.75 },
        { transform: 'scale(1, 1)' },
      ],
      { duration: 420, easing: 'cubic-bezier(0.3, 0, 0.2, 1)' },
    )
    if (id === null) {
      // Home: hand back to the logo's own handle, which has its own spring and hop.
      setAway(false)
    } else {
      dispatchEvent(new CustomEvent(LAND_EVENT, { detail: id }))
    }
  }

  function frame(now: number) {
    let stretch = 1
    let angle = 0
    if (flight) {
      const k = Math.min((now - flight.t0) / flight.dur, 1)
      const e = k < 0.5 ? 2 * k * k : 1 - (-2 * k + 2) ** 2 / 2
      const to = flight.to() // follows its target if the page reflows mid-flight
      const u = 1 - e
      const nx = u * u * flight.from.x + 2 * u * e * flight.c.x + e * e * to.x
      const ny = u * u * flight.from.y + 2 * u * e * flight.c.y + e * e * to.y
      const speed = Math.hypot(nx - pos.x, ny - pos.y)
      angle = (Math.atan2(ny - pos.y, nx - pos.x) * 180) / Math.PI
      stretch = 1 + Math.min(speed / 28, 0.55)
      pos.x = nx
      pos.y = ny
      if (k >= 1) {
        land(flight.id)
        // The landing frame is the last one drawn before it rests: it must not keep the
        // flight's stretch, or the dot sits there as an oval.
        stretch = 1
        angle = 0
      }
    } else if (at !== null) {
      const st = document.querySelector(`[data-station="${at}"]`) as HTMLElement | null
      if (st) {
        const p = pagePoint(st, st.dataset.land)
        pos.x = p.x
        pos.y = p.y
      }
      // At rest: the same spring as in the logo.
      spring.vx = (spring.vx + (spring.tx - spring.x) * 0.11) * 0.8
      spring.vy = (spring.vy + (spring.ty - spring.y) * 0.11) * 0.8
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
      `rotate(${angle}deg) scale(${stretch}, ${1 / stretch}) rotate(${-angle}deg)`

    const resting =
      !flight &&
      Math.hypot(spring.vx, spring.vy) < 0.01 &&
      Math.hypot(spring.tx - spring.x, spring.ty - spring.y) < 0.05
    // Resting, it needs no frames: it is placed in the page, so scrolling carries it.
    raf = flight || !resting ? requestAnimationFrame(frame) : 0
  }
  function wake() {
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
      const heading = flight ? flight.id : at
      if (want !== heading) launch(want)
      else wake() // a reflow may have moved the station under it
    })
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
  dot.addEventListener('pointerdown', onPoke)
  check()

  return () => {
    travellerActive = false
    cancelAnimationFrame(raf)
    removeEventListener('scroll', check)
    removeEventListener('resize', check)
    removeEventListener('pointermove', onPointer)
    dot.removeEventListener('pointerdown', onPoke)
    setAway(false)
  }
}
