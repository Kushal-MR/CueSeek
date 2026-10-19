import { useEffect, useRef, useState } from 'react'
import dashboard from '../assets/screens/watch-dashboard.webp'
import services from '../assets/screens/watch-services.webp'
import jellyfin from '../assets/screens/watch-jellyfin.webp'
import { useCue } from '../traveller/useCue.ts'
import { LAND_EVENT, dotTo } from '../traveller/traveller.ts'
import { Strap, type StrapShape } from './strap.ts'
import './Wrist.css'

/** The real watch app (M6.6 captures), in the order a tap on the face walks through it. */
const SCREENS = [
  { src: dashboard, alt: 'The watch dashboard: HomeServer, Operational, 2 of 2 healthy, with processor, memory and disk.' },
  { src: services, alt: 'Scrolled down: Jellyfin, 1 playing; qBittorrent, 1 active; and a Machine button.' },
  { src: jellyfin, alt: 'Jellyfin on the watch: running, playing Tears of Steel for the Demo user, with a Restart Jellyfin button.' },
]

function shapeFor(width: number): StrapShape {
  return width < 440
    ? { r: 86, lug: 12, top: 96, bottom: 120 }
    : { r: 118, lug: 16, top: 128, bottom: 170 }
}

/**
 * Section 04. The watch, hanging on its strap from the travelling dot.
 *
 * The one playful object on the page: it drops in, swings and settles, then sways as if in a
 * draught. Grab it and throw it; tap the face and it does what the real watch does — moves
 * on through its screens. Arrow keys nudge it. With reduced motion it simply hangs.
 */
export function Wrist() {
  const [ref, seen] = useCue<HTMLElement>('wrist', 0.3)
  const stage = useRef<HTMLDivElement>(null)
  const pin = useRef<HTMLSpanElement>(null)
  const watch = useRef<HTMLDivElement>(null)
  const upper = useRef<SVGPathElement>(null)
  const lower = useRef<SVGPathElement>(null)
  const holes = useRef<SVGPathElement>(null)
  const upperEdge = useRef<SVGPathElement>(null)
  const lowerEdge = useRef<SVGPathElement>(null)
  const upperSheen = useRef<SVGPathElement>(null)
  const lowerSheen = useRef<SVGPathElement>(null)
  const strap = useRef<Strap | null>(null)
  const [screen, setScreen] = useState(0)
  const [touched, setTouched] = useState(false)
  const [r, setR] = useState(118)

  // The dot's job here: it is the pin the strap hangs from.
  useEffect(() => {
    const onLand = (e: Event) => {
      if ((e as CustomEvent<string>).detail === 'wrist') dotTo(pin.current, { scale: 0.5, dur: 200 })
    }
    addEventListener(LAND_EVENT, onLand)
    return () => removeEventListener(LAND_EVENT, onLand)
  }, [])

  // The strap: built to the stage's size, dropped in on the cue, drawn while on screen.
  useEffect(() => {
    const el = stage.current
    if (!el || !seen) return
    const still = matchMedia('(prefers-reduced-motion: reduce)').matches
    let shape = shapeFor(el.clientWidth)
    const anchorOf = () => ({ x: el.clientWidth / 2, y: 22 })
    const s = new Strap(anchorOf(), shape)
    strap.current = s
    setR(shape.r)
    if (still) s.settle()
    else s.raise()

    const ro = new ResizeObserver(() => {
      const next = shapeFor(el.clientWidth)
      if (next.r !== shape.r) {
        // A different size of watch: start again, hanging.
        shape = next
        const fresh = new Strap(anchorOf(), shape)
        fresh.settle()
        strap.current = fresh
        setR(shape.r)
      } else {
        const a = anchorOf()
        strap.current?.moveAnchor(a.x, a.y)
      }
      draw()
    })
    ro.observe(el)

    let visible = true
    const io = new IntersectionObserver(([e]) => {
      visible = e.isIntersecting
      if (visible) loop()
    })
    io.observe(el)

    // A tilted phone leans the watch, where the browser offers orientation without asking.
    const onTilt = (e: DeviceOrientationEvent) => {
      if (strap.current && e.gamma !== null) strap.current.tilt = Math.max(-0.4, Math.min(0.4, e.gamma / 90)) * 0.7
    }
    if (!still && matchMedia('(pointer: coarse)').matches) addEventListener('deviceorientation', onTilt)

    let raf = 0
    let last = performance.now()
    function loop() {
      cancelAnimationFrame(raf)
      raf = requestAnimationFrame(function frame(now) {
        const dt = (now - last) / 1000
        last = now
        followDot()
        strap.current?.update(dt, still ? 0 : 1)
        draw()
        if (visible && !still) raf = requestAnimationFrame(frame)
      })
    }
    // The dot is the pin: while it rests here, the strap hangs from wherever it is — leaning
    // toward the pointer, or jumping when poked — and the drawn pin steps aside for it.
    const dotEl = document.querySelector('.traveller')
    function followDot() {
      const st = strap.current
      if (!st || !el) return
      const home = st.pin
      let x = home.x
      let y = home.y
      let here = false
      if (dotEl?.hasAttribute('data-shown')) {
        const d = dotEl.getBoundingClientRect()
        const b = el.getBoundingClientRect()
        const dx = d.left + d.width / 2 - b.left
        const dy = d.top + d.height / 2 - b.top
        if (Math.hypot(dx - home.x, dy - home.y) < 40) {
          x = dx
          y = dy
          here = true
        }
      }
      st.setPin(x, y)
      pin.current?.toggleAttribute('data-hidden', here)
    }
    function draw() {
      const st = strap.current
      if (!st) return
      const w = st.watch
      const up = smooth([...st.upper, w])
      const low = smooth([w, ...st.lower])
      for (const p of [upper, upperEdge, upperSheen]) p.current?.setAttribute('d', up)
      for (const p of [lower, lowerEdge, lowerSheen]) p.current?.setAttribute('d', low)
      holes.current?.setAttribute('d', smooth(st.lower.slice(2)))
      if (watch.current) {
        watch.current.style.transform = `translate(${w.x}px, ${w.y}px) rotate(${st.angle}rad)`
      }
    }
    last = performance.now()
    loop()
    return () => {
      cancelAnimationFrame(raf)
      ro.disconnect()
      io.disconnect()
      removeEventListener('deviceorientation', onTilt)
    }
  }, [seen])

  // Grab and throw; a tap that did not travel is a tap on the face.
  const down = useRef<{ x: number; y: number; t: number } | null>(null)
  const local = (e: React.PointerEvent) => {
    const b = stage.current!.getBoundingClientRect()
    return { x: e.clientX - b.left, y: e.clientY - b.top }
  }
  const onDown = (e: React.PointerEvent<HTMLDivElement>) => {
    if (!strap.current) return
    e.currentTarget.setPointerCapture(e.pointerId)
    const p = local(e)
    down.current = { ...p, t: e.timeStamp }
    strap.current.grab(p.x, p.y)
    setTouched(true)
  }
  const onMove = (e: React.PointerEvent) => {
    if (!down.current) return
    const p = local(e)
    strap.current?.drag(p.x, p.y)
  }
  const onUp = (e: React.PointerEvent) => {
    const d = down.current
    down.current = null
    strap.current?.release()
    if (!d) return
    const p = local(e)
    if (Math.hypot(p.x - d.x, p.y - d.y) < 6 && e.timeStamp - d.t < 400) next()
  }
  const next = () => setScreen((i) => (i + 1) % SCREENS.length)
  const onKey = (e: React.KeyboardEvent) => {
    const s = strap.current
    const kick: Record<string, [number, number]> = {
      ArrowLeft: [-700, 0],
      ArrowRight: [700, 0],
      ArrowUp: [0, -900],
      ArrowDown: [0, 400],
    }
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      next()
    } else if (kick[e.key] && s) {
      e.preventDefault()
      s.push(...kick[e.key])
      setTouched(true)
    }
  }

  return (
    <section ref={ref} id="wrist" className="wrist" data-in={seen || undefined} aria-labelledby="wrist-title">
      <span className="hud">Section 04 — On your wrist</span>
      <div className="wrist-grid">
        <div className="wrist-copy">
          <h2 id="wrist-title">Same answer, on your wrist.</h2>
          <p>
            The watch app shows the same verdict, every service and the machine's vitals.
            Restart a service with a tap and a confirm — stopping one takes a press-and-hold —
            or just glance at the watch face: <b>2/2</b> means everything is up.
          </p>
          <p className="small">
            Wear OS · your phone hands it the server's address; from then on it pairs with, and
            talks to, your server on its own.
          </p>
        </div>

        <div ref={stage} className="wrist-stage" style={{ '--r': `${r}px` } as React.CSSProperties}>
          <svg className="strap" aria-hidden="true">
            <path ref={upperEdge} className="edge" />
            <path ref={lowerEdge} className="edge" />
            <path ref={upper} className="band" />
            <path ref={lower} className="band" />
            <path ref={upperSheen} className="sheen" />
            <path ref={lowerSheen} className="sheen" />
            <path ref={holes} className="holes" />
          </svg>
          <span ref={pin} className="pin" data-station="wrist" aria-hidden="true" />
          <div
            ref={watch}
            className="watch"
            role="button"
            tabIndex={0}
            aria-label={`The CueSeek watch app, screen ${screen + 1} of ${SCREENS.length}: ${SCREENS[screen].alt} Press Enter for the next screen; arrow keys swing the watch.`}
            onPointerDown={onDown}
            onPointerMove={onMove}
            onPointerUp={onUp}
            onPointerCancel={onUp}
            onKeyDown={onKey}
          >
            <span className="crown" />
            <span className="dial">
              {SCREENS.map((s, i) => (
                <img key={s.src} src={s.src} alt="" draggable={false} data-on={i === screen || undefined} />
              ))}
            </span>
          </div>
          <span className="hint" data-gone={touched || undefined} aria-hidden="true">
            drag me · tap the face
          </span>
        </div>
      </div>
    </section>
  )
}

/** A smooth path through points: quadratic curves between their midpoints. */
function smooth(ps: { x: number; y: number }[]) {
  if (ps.length < 2) return ''
  let d = `M${ps[0].x.toFixed(1)} ${ps[0].y.toFixed(1)}`
  for (let i = 1; i < ps.length - 1; i++) {
    const mx = (ps[i].x + ps[i + 1].x) / 2
    const my = (ps[i].y + ps[i + 1].y) / 2
    d += ` Q${ps[i].x.toFixed(1)} ${ps[i].y.toFixed(1)} ${mx.toFixed(1)} ${my.toFixed(1)}`
  }
  const z = ps[ps.length - 1]
  return `${d} L${z.x.toFixed(1)} ${z.y.toFixed(1)}`
}
