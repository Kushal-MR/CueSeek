import { useEffect, useRef, useState, type KeyboardEvent, type PointerEvent } from 'react'
import { flushSync } from 'react-dom'
import { initialStop, paint, remember } from './apply.ts'
import { STOPS } from './stops.ts'
import './Knob.css'

/** The dial's travel: 270°, open at the bottom — the Swell's own shape. */
const SWEEP = 270
const START = -SWEEP / 2
const STEP = SWEEP / (STOPS.length - 1)
const LAST = STOPS.length - 1

const angleOf = (position: number) => START + position * STEP
const reducedMotion = () => matchMedia('(prefers-reduced-motion: reduce)').matches

/**
 * The theme control: a dial with five detents, from Night to Day through the greens.
 *
 * Turning it blends the page live between neighbouring stops, and letting go always settles
 * on one — so the palettes a reader can rest on are exactly the five the contrast check
 * covers. A tap, a key or the wheel moves one detent at a time, and the new colours spread
 * outward from the knob.
 *
 * To assistive technology it is a slider: "Theme, Sage, 3 of 5", turned with the arrow keys.
 */
export function Knob() {
  const [index, setIndex] = useState(initialStop)
  const [position, setPosition] = useState<number>(index)
  const [dragging, setDragging] = useState(false)
  const self = useRef<HTMLDivElement>(null)
  const drag = useRef<{ x: number; y: number; moved: boolean; detent: number } | null>(null)
  const busy = useRef(false)

  // The first paint already matched the system preference (see vite.config.ts); this
  // corrects it for a visitor who chose something else last time.
  useEffect(() => paint(index), []) // eslint-disable-line react-hooks/exhaustive-deps -- once, on mount

  /** One detent at a time, with the ripple. */
  function go(next: number) {
    const target = Math.min(Math.max(next, 0), LAST)
    if (target === index || busy.current) return
    const commit = () => {
      setIndex(target)
      setPosition(target)
      paint(target)
    }
    remember(target)

    const knob = self.current?.getBoundingClientRect()
    if (!knob || reducedMotion() || !document.startViewTransition) {
      commit()
      return
    }
    const x = knob.left + knob.width / 2
    const y = knob.top + knob.height / 2
    const r = Math.hypot(Math.max(x, innerWidth - x), Math.max(y, innerHeight - y))
    busy.current = true
    const t = document.startViewTransition(() => flushSync(commit))
    t.ready.then(() =>
      document.documentElement.animate(
        { clipPath: [`circle(0px at ${x}px ${y}px)`, `circle(${r}px at ${x}px ${y}px)`] },
        { duration: 560, easing: 'cubic-bezier(0.2, 0, 0, 1)', pseudoElement: '::view-transition-new(root)' },
      ),
    )
    t.finished.finally(() => (busy.current = false))
  }

  // The wheel needs a non-passive listener to stop the page scrolling under it.
  useEffect(() => {
    const el = self.current
    if (!el) return
    let travel = 0
    const onWheel = (e: WheelEvent) => {
      e.preventDefault()
      travel += e.deltaY
      if (Math.abs(travel) < 40) return
      go(index + Math.sign(travel))
      travel = 0
    }
    el.addEventListener('wheel', onWheel, { passive: false })
    return () => el.removeEventListener('wheel', onWheel)
  })

  function positionAt(e: PointerEvent): number {
    const r = self.current!.getBoundingClientRect()
    // 0° is twelve o'clock, clockwise positive — the same frame as the LEDs.
    let deg = (Math.atan2(e.clientX - (r.left + r.width / 2), -(e.clientY - (r.top + r.height / 2))) * 180) / Math.PI
    // In the gap at the bottom, hold whichever end is nearer.
    if (deg > SWEEP / 2) deg = SWEEP / 2
    if (deg < -SWEEP / 2) deg = -SWEEP / 2
    return (deg - START) / STEP
  }

  function onPointerDown(e: PointerEvent<HTMLDivElement>) {
    if (busy.current) return
    e.currentTarget.setPointerCapture(e.pointerId)
    drag.current = { x: e.clientX, y: e.clientY, moved: false, detent: index }
  }

  function onPointerMove(e: PointerEvent<HTMLDivElement>) {
    const d = drag.current
    if (!d) return
    if (!d.moved && Math.hypot(e.clientX - d.x, e.clientY - d.y) < 6) return
    if (!d.moved) {
      d.moved = true
      setDragging(true)
    }
    const p = positionAt(e)
    setPosition(p)
    paint(p)
    // A tick as each detent passes under the finger, where the hardware allows it.
    const detent = Math.round(p)
    if (detent !== d.detent) {
      d.detent = detent
      navigator.vibrate?.(6)
    }
  }

  function onPointerUp() {
    const d = drag.current
    drag.current = null
    if (!d) return
    if (!d.moved) {
      // A tap turns one detent clockwise, and from Day comes round to Night.
      go(index === LAST ? 0 : index + 1)
      return
    }
    setDragging(false)
    settle(position)
  }

  /** Let go between stops: ease onto the nearest. */
  function settle(from: number) {
    const to = Math.round(from)
    setIndex(to)
    remember(to)
    if (reducedMotion()) {
      setPosition(to)
      paint(to)
      return
    }
    const start = performance.now()
    const frame = (now: number) => {
      const k = Math.min((now - start) / 180, 1)
      const p = from + (to - from) * (1 - (1 - k) ** 3)
      setPosition(k === 1 ? to : p)
      paint(k === 1 ? to : p)
      if (k < 1) requestAnimationFrame(frame)
    }
    requestAnimationFrame(frame)
  }

  function onKeyDown(e: KeyboardEvent) {
    const moves: Record<string, number> = {
      ArrowRight: index + 1, ArrowUp: index + 1, PageUp: index + 1,
      ArrowLeft: index - 1, ArrowDown: index - 1, PageDown: index - 1,
      Home: 0, End: LAST,
    }
    if (!(e.key in moves)) return
    e.preventDefault()
    go(moves[e.key])
  }

  const stop = STOPS[index]
  return (
    <div
      ref={self}
      className="knob"
      data-dragging={dragging || undefined}
      role="slider"
      tabIndex={0}
      aria-label="Theme"
      aria-valuemin={0}
      aria-valuemax={LAST}
      aria-valuenow={index}
      aria-valuetext={`${stop.name}, ${index + 1} of ${STOPS.length}`}
      title={`Theme: ${stop.name} — turn, tap or use the arrow keys`}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerCancel={onPointerUp}
      onKeyDown={onKeyDown}
    >
      {STOPS.map((s, i) => (
        <i
          key={s.id}
          className="led"
          data-lit={i <= Math.round(position) || undefined}
          style={{ transform: `rotate(${angleOf(i)}deg) translateY(-21px)` }}
        />
      ))}
      <span className="dial" style={{ transform: `rotate(${angleOf(position)}deg)` }}>
        <span className="notch" />
      </span>
    </div>
  )
}
