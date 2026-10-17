import { useEffect, useRef, useState } from 'react'
import { arriving } from './arriving.ts'
import { HANDLE_CENTRE } from './geometry.ts'
import { SwellParts } from './SwellParts.tsx'
import { createWordmark, type Wordmark } from './wordmark.ts'
import './Arrival.css'

/** When each beat starts, in ms. */
const BEAT = {
  long: 250, // the long arc draws itself up the left
  tail: 520, // the tail draws up the right, overlapping
  pop: 1050, // the dot pops
  ripple: 1420, // a ripple leaves the dot and writes the word
} as const

/** Stretch along the dot's own direction — up and to the right, out of the dial. */
const along = (sx: number, sy: number, dx = 0, dy = 0) =>
  `translate(${dx}px, ${dy}px) rotate(-45deg) scale(${sx}, ${sy}) rotate(45deg)`

/**
 * Section 00.
 *
 * The Swell assembles itself: its two arcs draw on like pen strokes, then the dot pops into
 * the gap — gathering itself, bursting out stretched, squashing as it lands, wobbling to rest
 * (the Ghost Blob reference: squash, stretch and overshoot are what give a shape weight). As
 * it lands, a glowing liquid ripple spreads out from it across the lockup and writes the word,
 * leaving a green smear that decays (the Sixfold reference), and the frames draw on.
 *
 * Afterwards nothing moves but the dot, which now and then gives a small hop — enough to make
 * the mark feel alive, not enough to compete with the page.
 */
export function Arrival() {
  const swell = useRef<SVGSVGElement>(null)
  const title = useRef<HTMLHeadingElement>(null)
  const canvas = useRef<HTMLCanvasElement>(null)
  const wordmark = useRef<Wordmark | null>(null)
  const [shader, setShader] = useState(false)

  useEffect(() => {
    let alive = true
    const root = document.documentElement
    const svg = swell.current

    if (arriving && svg) {
      const pen = { easing: 'cubic-bezier(0.65, 0, 0.35, 1)', fill: 'backwards' } as const
      svg.querySelector('.draw-long')!.animate(
        { strokeDashoffset: [1, 0] },
        { ...pen, duration: 760, delay: BEAT.long },
      )
      svg.querySelector('.draw-tail')!.animate(
        { strokeDashoffset: [1, 0] },
        { ...pen, duration: 520, delay: BEAT.tail },
      )
      svg.querySelector('.handle')!.animate(
        [
          { offset: 0, opacity: 0, transform: along(0, 0) },
          { offset: 0.12, opacity: 1, transform: along(0.34, 0.34) },
          { offset: 0.3, transform: along(0.3, 0.2) }, // gathers itself
          { offset: 0.5, transform: along(1.4, 0.76, 1.6, -1.6) }, // bursts out
          { offset: 0.66, transform: along(0.82, 1.17) }, // lands
          { offset: 0.8, transform: along(1.07, 0.95) },
          { offset: 0.91, transform: along(0.98, 1.01) },
          { offset: 1, opacity: 1, transform: along(1, 1) },
        ],
        { duration: 760, delay: BEAT.pop, easing: 'cubic-bezier(0.3, 0, 0.2, 1)', fill: 'backwards' },
      )
      svg.querySelectorAll('.burst').forEach((ring, i) =>
        ring.animate(
          [
            { transform: 'scale(1)', opacity: 0 },
            { offset: 0.06, opacity: 0.7 },
            { transform: `scale(${3.4 + i * 1.6})`, opacity: 0 },
          ],
          {
            duration: 900 + i * 300,
            delay: BEAT.pop + 380 + i * 120,
            easing: 'cubic-bezier(0.2, 0, 0, 1)',
            fill: 'backwards',
          },
        ),
      )
    }

    const plainText = () => {
      // No WebGL (or no font): the heading stays ordinary text, faded in on the same beat.
      root.dataset.arrivalWave = 'none'
    }
    // The shader needs the real font in hand before it can rasterise the word.
    document.fonts.load(`500 112px "IBM Plex Sans"`).then(() => {
      if (!alive || !canvas.current || !title.current) return
      const wm = createWordmark(canvas.current, title.current)
      if (!wm) return plainText()
      wordmark.current = wm
      setShader(true)
      if (arriving) {
        wm.hide()
        setTimeout(() => {
          if (!alive || !svg) return
          // The ripple leaves from the dot's real position on screen.
          const box = svg.getBoundingClientRect()
          const scale = box.width / 60
          wm.play(
            {
              x: box.left + (HANDLE_CENTRE.x - 24) * scale,
              y: box.top + (HANDLE_CENTRE.y - 26) * scale,
            },
            true,
          )
        }, BEAT.ripple)
      }
    }, plainText)

    // The knob changes the page's colours; the canvas has to be told.
    const theme = new MutationObserver(() => wordmark.current?.repaint())
    theme.observe(root, { attributes: true, attributeFilter: ['style'] })

    return () => {
      alive = false
      theme.disconnect()
      wordmark.current?.destroy()
      wordmark.current = null
    }
  }, [])

  return (
    <section
      className="hero"
      aria-labelledby="hero-title"
      onPointerMove={(e) => wordmark.current?.pointer(e.clientX, e.clientY)}
    >
      <div className="lockup">
        <canvas ref={canvas} className="word-canvas" aria-hidden="true" />
        <SwellParts ref={swell} size={112} className="hero-mark" />
        <h1 id="hero-title" ref={title} data-shader={shader || undefined}>
          CueSeek
        </h1>
        <span className="hud-box b1" aria-hidden="true">SECTION 01</span>
        <span className="hud-box b2" aria-hidden="true">SECTION 02</span>
        <span className="hud-box b3" aria-hidden="true">SECTION 03</span>
        <span className="wire" aria-hidden="true">
          <i style={{ left: '0%' }} />
          <i style={{ left: '38%' }} />
          <i style={{ left: '71%' }} />
          <i style={{ left: '100%' }} />
        </span>
      </div>
      <p className="tagline">An operations console for self-hosted home servers.</p>
      <p className="sub">
        One glance at your phone or your watch tells you whether everything on your home
        server is fine — and lets you fix it when it isn't.
      </p>
      <div className="track" aria-hidden="true" />
      <span className="scrollcue" aria-hidden="true">SCROLL</span>
    </section>
  )
}
