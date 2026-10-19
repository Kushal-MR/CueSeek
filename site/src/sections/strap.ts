/**
 * The watch on its strap (M6.8): a small Verlet rope, written for this one object.
 *
 * The upper strap is a chain from a pinned anchor to the watch's top lug; the watch is one
 * heavy point at its centre, and its angle is the angle of the strap where it meets the lug.
 * The lower strap hangs freely from the bottom lug. Everything is in the stage's own pixels,
 * y down, and is integrated at a fixed 240 Hz however fast the display runs.
 */

export interface StrapShape {
  /** Watch radius (the case), px. */
  r: number
  /** Lug: the gap between the case and where a strap attaches, px. */
  lug: number
  /** Length of the upper strap, and of the lower one, px. */
  top: number
  bottom: number
}

interface P {
  x: number
  y: number
  px: number
  py: number
  w: number // inverse mass; 0 = pinned
}

const SEGS_TOP = 9
const SEGS_BOTTOM = 8
const STEP = 1 / 240
const GRAVITY = 2200 // px/s²
const DAMP = 0.9965 // per step
const ITER = 10

export class Strap {
  readonly upper: P[] = []
  readonly lower: P[] = []
  readonly watch: P
  /** Watch rotation, radians; 0 = hanging straight down. */
  angle = 0

  /** A sideways pull, e.g. from a tilted phone, as a fraction of gravity. */
  tilt = 0
  private shape: StrapShape
  private anchor: { x: number; y: number }
  private segTop: number
  private segBottom: number
  private acc = 0
  private time = 0
  private held: { x: number; y: number; lx: number; ly: number } | null = null

  constructor(anchor: { x: number; y: number }, shape: StrapShape) {
    this.anchor = anchor
    this.shape = shape
    this.segTop = shape.top / SEGS_TOP
    this.segBottom = shape.bottom / SEGS_BOTTOM
    for (let i = 0; i <= SEGS_TOP; i++) this.upper.push(pt(anchor.x, anchor.y + i * this.segTop, i ? 1 : 0))
    const wy = anchor.y + shape.top + shape.lug + shape.r
    this.watch = pt(anchor.x, wy, 1 / 7)
    for (let i = 0; i <= SEGS_BOTTOM; i++) this.lower.push(pt(anchor.x, wy + shape.r + shape.lug + i * this.segBottom, i ? 1 : 0))
  }

  /** Lay it out straight up from the anchor, a little to one side, to drop in. */
  raise() {
    const { x, y } = this.anchor
    const all = [...this.upper.slice(1), this.watch, ...this.lower]
    all.forEach((p, i) => {
      const d = (i + 1) * 26
      p.x = p.px = x + d * 0.35
      p.y = p.py = y - d * 0.9
    })
  }

  /** Lay it out hanging still, for reduced motion and first paint. */
  settle() {
    const { x, y } = this.anchor
    this.upper.forEach((p, i) => set(p, x, y + i * this.segTop))
    const wy = y + this.shape.top + this.shape.lug + this.shape.r
    set(this.watch, x, wy)
    this.lower.forEach((p, i) => set(p, x, wy + this.shape.r + this.shape.lug + i * this.segBottom))
    this.angle = 0
  }

  moveAnchor(x: number, y: number) {
    const dx = x - this.anchor.x
    const dy = y - this.anchor.y
    for (const p of [...this.upper, this.watch, ...this.lower]) {
      p.x += dx; p.px += dx; p.y += dy; p.py += dy
    }
    this.anchor = { x, y }
  }

  get pin() {
    return this.anchor
  }

  /** Move only the pin, leaving the strap to follow it — the dot leaning, or being poked. */
  setPin(x: number, y: number) {
    const p = this.upper[0]
    p.x = p.px = x
    p.y = p.py = y
  }

  /** Grab the watch at a point (stage px); it follows `drag` until `release`. */
  grab(x: number, y: number) {
    this.held = { x: x - this.watch.x, y: y - this.watch.y, lx: this.watch.x, ly: this.watch.y }
  }
  drag(x: number, y: number) {
    if (!this.held) return
    this.held.lx = x - this.held.x
    this.held.ly = y - this.held.y
  }
  release() {
    this.held = null
  }

  /** A shove, px/s — the arrow keys. */
  push(vx: number, vy: number) {
    this.watch.px -= vx * STEP
    this.watch.py -= vy * STEP
  }

  /** Advance by a frame's worth of time (s). Returns false if nothing visibly moved. */
  update(dt: number, sway: number) {
    this.acc += Math.min(dt, 0.05)
    let steps = 0
    const w = this.watch
    const from = { x: w.x, y: w.y }
    const n = Math.floor(this.acc / STEP)
    while (this.acc >= STEP) {
      this.acc -= STEP
      this.time += STEP
      steps += 1
      if (this.held) {
        // Follow the pointer, spread over the frame's steps so a throw keeps its speed.
        const k = steps / n
        const tx = from.x + (this.held.lx - from.x) * k
        const ty = from.y + (this.held.ly - from.y) * k
        w.px = w.x
        w.py = w.y
        w.x = tx
        w.y = ty
      }
      // A draught: the slowest of sways, felt mostly by the watch.
      const gx = this.tilt * GRAVITY + sway * 26 * Math.sin(this.time * 0.55) + sway * 9 * Math.sin(this.time * 1.7)
      this.integrate(gx)
      for (let i = 0; i < ITER; i++) this.solve()
    }
    const d = this.upper[SEGS_TOP]
    const a = Math.atan2(w.x - d.x, w.y - d.y)
    this.angle = -a
    return steps > 0
  }

  private integrate(gx: number) {
    for (const p of [...this.upper, this.watch, ...this.lower]) {
      if (!p.w || (p === this.watch && this.held)) continue
      const vx = (p.x - p.px) * DAMP
      const vy = (p.y - p.py) * DAMP
      p.px = p.x
      p.py = p.y
      p.x += vx + gx * STEP * STEP
      p.y += vy + GRAVITY * STEP * STEP
    }
  }

  private solve() {
    const { r, lug } = this.shape
    const w = this.watch
    const wInv = this.held ? 0 : w.w
    for (let i = 0; i < SEGS_TOP; i++) link(this.upper[i], this.upper[i + 1], this.segTop)
    const last = this.upper[SEGS_TOP]
    link(last, w, r + lug, last.w, wInv)
    // Bending stiffness near the lug: a strap is stiff where it is riveted to the case.
    link(this.upper[SEGS_TOP - 1], w, this.segTop + r + lug, this.upper[SEGS_TOP - 1].w, wInv, 0.35)
    // The lower strap starts at the bottom lug, opposite the top one through the centre.
    const dx = w.x - last.x
    const dy = w.y - last.y
    const len = Math.hypot(dx, dy) || 1
    const b = this.lower[0]
    b.x = w.x + (dx / len) * (r + lug)
    b.y = w.y + (dy / len) * (r + lug)
    for (let i = 0; i < SEGS_BOTTOM; i++) link(this.lower[i], this.lower[i + 1], this.segBottom)
    // ...and leaves the lug straight, along the watch's own axis.
    const b1 = this.lower[1]
    const want = { x: b.x + (dx / len) * this.segBottom, y: b.y + (dy / len) * this.segBottom }
    b1.x += (want.x - b1.x) * 0.3
    b1.y += (want.y - b1.y) * 0.3
  }
}

function pt(x: number, y: number, w: number): P {
  return { x, y, px: x, py: y, w }
}
function set(p: P, x: number, y: number) {
  p.x = p.px = x
  p.y = p.py = y
}

/** Hold two points at a distance, sharing the correction by inverse mass. */
function link(a: P, b: P, rest: number, wa = a.w, wb = b.w, stiff = 1) {
  const dx = b.x - a.x
  const dy = b.y - a.y
  const d = Math.hypot(dx, dy) || 1e-6
  const sum = wa + wb
  if (!sum) return
  const k = ((d - rest) / d / sum) * stiff
  a.x += dx * k * wa
  a.y += dy * k * wa
  b.x -= dx * k * wb
  b.y -= dy * k * wb
}
