/**
 * Fails if any stop puts a reader-facing pair below WCAG AA (4.5 : 1).
 *
 * The knob's promise is that every stop is readable, not that it looked readable to
 * whoever last edited a hex value. Run by CI; run it yourself with `npm run check:contrast`.
 */
import { STOPS, type Token } from '../src/theme/stops.ts'

const AA = 4.5

/** [foreground, background] — every pair a sighted reader actually depends on. */
const PAIRS: [Token, Token][] = [
  ['text', 'bg'],
  ['muted', 'bg'],
  ['accent', 'bg'],
  ['hi', 'bg'],
  ['card-text', 'card'],
  ['ok', 'card'],
  ['warn', 'card'],
  ['bad', 'card'],
]

function luminance(hex: string): number {
  const c = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255)
  const [r, g, b] = c.map((x) => (x <= 0.03928 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4))
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

function ratio(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (hi + 0.05) / (lo + 0.05)
}

let failed = 0
let lowest = Infinity
for (const stop of STOPS) {
  const results = PAIRS.map(([fg, bg]) => {
    const r = ratio(stop.colors[fg], stop.colors[bg])
    lowest = Math.min(lowest, r)
    if (r < AA) failed++
    return `${fg}/${bg} ${r.toFixed(2)}${r < AA ? ' ✗' : ''}`
  })
  console.log(`${stop.name.padEnd(7)} ${results.join('  ')}`)
}

console.log(`\nlowest pair on any stop: ${lowest.toFixed(2)} : 1 (AA is ${AA})`)
if (failed) {
  console.error(`${failed} pair(s) below AA`)
  process.exit(1)
}
