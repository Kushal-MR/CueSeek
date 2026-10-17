/**
 * The Swell's entrance: it arrives from the top right on a curve, stretched along its own
 * path by its speed, lands with a squash, overshoots, and settles. Squash and stretch, the
 * oldest trick in animation — what makes a shape feel like it has weight.
 *
 * Generated as Web Animations keyframes rather than written by hand, so the stretch always
 * follows the real direction of travel.
 */
export function flyInKeyframes(): Keyframe[] {
  const FLIGHT = 0.72 // share of the duration spent in the air
  const STEPS = 28
  // Start, control and end points of the flight path, relative to where it lands (px).
  const p0 = [360, -260]
  const p1 = [300, 40]
  const p2 = [0, 0]
  const at = (t: number) =>
    [0, 1].map((i) => (1 - t) ** 2 * p0[i] + 2 * (1 - t) * t * p1[i] + t * t * p2[i])

  const frames: Keyframe[] = []
  for (let i = 0; i <= STEPS; i++) {
    const s = i / STEPS
    // Fast out of the corner, slowing toward the landing.
    const t = 1 - (1 - s) ** 2.2
    const [x, y] = at(t)
    const [nx, ny] = at(Math.min(t + 0.02, 1))
    const angle = (Math.atan2(ny - y, nx - x) * 180) / Math.PI
    const speed = Math.hypot(nx - x, ny - y)
    const stretch = 1 + Math.min(speed / 14, 0.45) * (1 - s * 0.6)
    frames.push({
      offset: s * FLIGHT,
      opacity: Math.min(1, s * 4),
      transform:
        `translate(${x}px, ${y}px) rotate(${angle}deg) ` +
        `scale(${stretch}, ${1 / stretch}) rotate(${-angle}deg)`,
    })
  }

  // Landing: squash, rebound, settle.
  const land = [
    [0.82, 1.16, 0.86],
    [0.9, 0.95, 1.06],
    [0.96, 1.02, 0.98],
    [1, 1, 1],
  ]
  for (const [offset, sx, sy] of land) {
    frames.push({ offset, opacity: 1, transform: `translate(0px, 0px) scale(${sx}, ${sy})` })
  }
  return frames
}
