import { HANDLE_CENTRE } from './geometry.ts'

/**
 * The dot as a living thing (the Ghost Blob reference).
 *
 * It sits on a spring. Move the pointer and it leans and drifts toward it; it stretches in
 * the direction it is travelling and, because a spring overshoots, squashes as it stops and
 * wobbles to rest. Leave, and it springs home. Poke it and it jumps.
 *
 * All of that is one rule — a damped spring — plus one drawing rule: scale along the
 * velocity. Squash and stretch fall out of the physics rather than being keyframed.
 */
export interface Blob {
  follow(clientX: number, clientY: number): void
  release(): void
  poke(): void
  destroy(): void
}

const REACH = 9 // furthest it will drift, in the mark's units (the dot's radius is 7.5)
const STIFFNESS = 0.11
const DAMPING = 0.8

export function createBlob(svg: SVGSVGElement): Blob {
  const el = svg.querySelector<SVGGElement>('.handle-follow')!
  const pos = { x: 0, y: 0 }
  const vel = { x: 0, y: 0 }
  const target = { x: 0, y: 0 }
  let raf = 0

  function frame() {
    vel.x = (vel.x + (target.x - pos.x) * STIFFNESS) * DAMPING
    vel.y = (vel.y + (target.y - pos.y) * STIFFNESS) * DAMPING
    pos.x += vel.x
    pos.y += vel.y

    const speed = Math.hypot(vel.x, vel.y)
    const stretch = 1 + Math.min(speed * 0.16, 0.5)
    const angle = (Math.atan2(vel.y, vel.x) * 180) / Math.PI
    el.style.transform =
      `translate(${pos.x.toFixed(2)}px, ${pos.y.toFixed(2)}px) ` +
      `rotate(${angle}deg) scale(${stretch}, ${1 / stretch}) rotate(${-angle}deg)`

    const resting =
      speed < 0.004 && Math.hypot(target.x - pos.x, target.y - pos.y) < 0.01
    raf = resting ? 0 : requestAnimationFrame(frame)
  }
  const wake = () => {
    if (!raf) raf = requestAnimationFrame(frame)
  }

  return {
    follow(x, y) {
      const box = svg.getBoundingClientRect()
      const unit = box.width / 60 // the mark's viewBox is 60 units wide
      const dx = (x - (box.left + (HANDLE_CENTRE.x - 24) * unit)) / unit
      const dy = (y - (box.top + (HANDLE_CENTRE.y - 26) * unit)) / unit
      const dist = Math.hypot(dx, dy)
      // Leans hard toward a pointer close by, and only a little toward one far away.
      const pull = REACH * (1 - Math.exp(-dist / 28))
      target.x = dist ? (dx / dist) * pull : 0
      target.y = dist ? (dy / dist) * pull : 0
      wake()
    },
    release() {
      target.x = 0
      target.y = 0
      wake()
    },
    poke() {
      // A jump up and out of the dial; the spring brings it back with a squash.
      vel.x += 2.6
      vel.y -= 3.4
      wake()
    },
    destroy() {
      cancelAnimationFrame(raf)
    },
  }
}
