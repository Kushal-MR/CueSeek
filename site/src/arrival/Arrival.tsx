import { useEffect, useRef, useState } from 'react'
import { Mark } from '../Mark.tsx'
import { flyInKeyframes } from './flyIn.ts'
import { createWordmark, type Wordmark } from './wordmark.ts'
import { arriving } from './arriving.ts'
import './Arrival.css'

/** When each beat starts, in ms — the storyboard's timeline (00 · Arrival). */
const BEAT = { mark: 400, markFor: 900, wave: 1200 } as const

/**
 * Section 00. The Swell flies in and lands; a wave writes the wordmark and leaves a green
 * smear that decays; then the copy and the nav arrive. After that, only the mark breathes,
 * and the wordmark ripples faintly under the pointer.
 */
export function Arrival() {
  const mark = useRef<HTMLSpanElement>(null)
  const title = useRef<HTMLHeadingElement>(null)
  const canvas = useRef<HTMLCanvasElement>(null)
  const wordmark = useRef<Wordmark | null>(null)
  const [shader, setShader] = useState(false)

  useEffect(() => {
    let alive = true
    const root = document.documentElement

    if (arriving && mark.current) {
      mark.current.animate(flyInKeyframes(), {
        duration: BEAT.markFor,
        delay: BEAT.mark,
        easing: 'linear',
        fill: 'backwards',
      })
    }

    // The shader needs the real font in hand before it can rasterise the word.
    const plainText = () => {
      // No WebGL (or no font): the heading stays ordinary text, faded in on the same beat.
      root.dataset.arrivalWave = 'none'
    }
    document.fonts.load(`500 112px "IBM Plex Sans"`).then(() => {
      if (!alive || !canvas.current || !title.current) return
      const wm = createWordmark(canvas.current, title.current)
      if (!wm) return plainText()
      wordmark.current = wm
      setShader(true)
      if (arriving) {
        wm.hide()
        setTimeout(() => alive && wm.play(true), BEAT.wave)
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
        <span ref={mark} className="mark-flight">
          <Mark size={112} className="hero-mark" />
        </span>
        <span className="word">
          <h1 id="hero-title" ref={title} data-shader={shader || undefined}>
            CueSeek
          </h1>
          <canvas ref={canvas} className="word-canvas" aria-hidden="true" />
        </span>
        <span className="hud-box b1" aria-hidden="true">SECTION 01</span>
        <span className="hud-box b2" aria-hidden="true">SECTION 02</span>
        <span className="hud-box b3" aria-hidden="true">SECTION 03</span>
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
