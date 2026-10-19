import { useEffect, useMemo, useRef, useState } from 'react'
import source from '../reel/scenes.ts?raw'
import { BARS, BAR_SECONDS, LENGTH } from '../reel/kit.ts'
import { ReelPlayer } from '../reel/player.ts'
import { useCue } from '../traveller/useCue.ts'
import { LAND_EVENT, dotTo } from '../traveller/traveller.ts'
import './Reel.css'

/** The source, cut at each `// bar N — …` heading: the code for each bar, as it runs. */
function splitSource(text: string) {
  const lines = text.split('\n')
  const starts: number[] = []
  lines.forEach((l, i) => {
    if (/^\/\/ bar \d+ —/.test(l)) starts.push(i)
  })
  return starts.map((from, i) => {
    const block = lines.slice(from, starts[i + 1] ?? lines.length)
    while (block.length && !block[block.length - 1].trim()) block.pop()
    // Lines that start something on a beat: `s.at(<beat>`, with a literal beat.
    const cues = block.flatMap((l, j) => [...l.matchAll(/s\.at\((\d+(?:\.\d+)?)/g)].map((m) => ({ line: j, beat: +m[1] })))
    return { lines: block, cues }
  })
}

/**
 * Section 05. The motion reel, drawn live by the page: "This video is not a video."
 *
 * It starts, silent, when it is reached, and pauses when it leaves. Sound is synthesised
 * only when asked for. "View source" shows the code for the bar on screen, the line that is
 * running highlighted. With reduced motion it waits on the end card for a press of Play.
 */
export function Reel() {
  const [ref, seen] = useCue<HTMLElement>('reel', 0.35)
  const canvas = useRef<HTMLCanvasElement>(null)
  const frame = useRef<HTMLDivElement>(null)
  const led = useRef<HTMLSpanElement>(null)
  const player = useRef<ReelPlayer | null>(null)
  const [still] = useState(() => matchMedia('(prefers-reduced-motion: reduce)').matches)
  const [playing, setPlaying] = useState(false)
  const [sound, setSound] = useState(false)
  const [loading, setLoading] = useState(false)
  const [showSource, setShowSource] = useState(false)
  const [flash, setFlash] = useState<{ n: number; playing: boolean } | null>(null)
  const [pos, setPos] = useState({ bar: 0, line: -1 })
  const userPaused = useRef(false)
  const bars = useMemo(() => splitSource(source), [])

  useEffect(() => {
    const p = new ReelPlayer(canvas.current!)
    player.current = p
    p.onTick = (t) => {
      const bar = Math.min(BARS - 1, Math.floor(t / BAR_SECONDS))
      const beat = ((t % BAR_SECONDS) / BAR_SECONDS) * 4
      let line = -1
      for (const c of bars[bar]?.cues ?? []) if (c.beat <= beat) line = Math.max(line, c.line)
      setPos((old) => (old.bar === bar && old.line === line ? old : { bar, line }))
    }
    if (still) p.seek(LENGTH - 0.6) // the end card, still
    const ro = new ResizeObserver(() => p.resize())
    ro.observe(canvas.current!)
    // Follow the knob: the reel is drawn in the page's colours.
    const mo = new MutationObserver(() => p.draw())
    mo.observe(document.documentElement, { attributes: true, attributeFilter: ['style', 'class'] })
    return () => {
      ro.disconnect()
      mo.disconnect()
      p.destroy()
    }
  }, [bars, still])

  // Plays while on screen — once it has been reached — unless the visitor paused it.
  useEffect(() => {
    if (!seen || still) return
    const io = new IntersectionObserver(
      ([e]) => {
        const p = player.current
        if (!p) return
        if (e.isIntersecting && !userPaused.current) {
          p.play()
          setPlaying(true)
          // Compose the soundtrack now, quietly, so "Sound" answers at once when pressed.
          const idle = window.requestIdleCallback ?? ((f: () => void) => setTimeout(f, 1500))
          idle(() => void p.compose())
        } else if (!e.isIntersecting && p.isPlaying) {
          p.pause()
          setPlaying(false)
        }
      },
      { threshold: 0.35 },
    )
    io.observe(frame.current!)
    return () => io.disconnect()
  }, [seen, still])

  // The dot's job here: it enters the reel as bar 1's "live" dot, then becomes the player's
  // own light — lit and beating while the reel plays.
  useEffect(() => {
    let t = 0
    const onLand = (e: Event) => {
      if ((e as CustomEvent<string>).detail !== 'reel') return
      if (!still && !userPaused.current) player.current?.seek(0)
      t = window.setTimeout(() => dotTo(led.current, { scale: 0.32, pulse: true, dur: 600 }), 1900)
    }
    addEventListener(LAND_EVENT, onLand)
    return () => {
      removeEventListener(LAND_EVENT, onLand)
      clearTimeout(t)
    }
  }, [still])

  // The light is lit while the reel plays: if the dot is sitting on it, it rests when paused.
  useEffect(() => {
    const dot = document.querySelector<HTMLElement>('.traveller')
    const check = () => {
      const l = led.current?.getBoundingClientRect()
      const d = dot?.getBoundingClientRect()
      const here = !!l && !!d && Math.hypot(l.x + l.width / 2 - (d.x + d.width / 2), l.y + l.height / 2 - (d.y + d.height / 2)) < 12
      dot?.toggleAttribute('data-resting', here && !playing)
    }
    check()
    addEventListener('scroll', check, { passive: true })
    addEventListener('cueseek:dot-arrived', check)
    return () => {
      removeEventListener('scroll', check)
      removeEventListener('cueseek:dot-arrived', check)
      dot?.removeAttribute('data-resting')
    }
  }, [playing])

  const toggle = () => {
    const p = player.current
    if (!p) return
    if (p.isPlaying) {
      p.pause()
      userPaused.current = true
    } else {
      p.play()
      userPaused.current = false
    }
    setPlaying(p.isPlaying)
    setFlash((f) => ({ n: (f?.n ?? 0) + 1, playing: p.isPlaying }))
  }
  const toggleSound = async () => {
    const p = player.current
    if (!p) return
    setLoading(!sound)
    await p.setSound(!sound)
    setSound(!sound)
    setLoading(false)
  }
  const onKey = (e: React.KeyboardEvent) => {
    const p = player.current
    if (!p) return
    if (e.key === 'ArrowRight' || e.key === 'ArrowLeft') {
      e.preventDefault()
      p.seekBar(pos.bar + (e.key === 'ArrowRight' ? 1 : -1))
    } else if (e.key === ' ' || e.key === 'k') {
      e.preventDefault()
      toggle()
    }
  }

  const block = bars[pos.bar]
  return (
    <section ref={ref} id="reel" className="reel" data-in={seen || undefined} aria-labelledby="reel-title">
      <span className="hud">Section 05 — The reel</span>
      <h2 id="reel-title">This video is not a video.</h2>
      <p className="reel-sub">Drawn by code. Music synthesised by code. Nothing filmed or recorded.</p>

      <div
        ref={frame}
        className="reel-frame"
        data-source={showSource || undefined}
        tabIndex={0}
        role="group"
        aria-label="The CueSeek motion reel. Space plays or pauses; left and right arrows step a bar."
        onKeyDown={onKey}
      >
        <canvas
          ref={canvas}
          onClick={toggle}
          role="img"
          aria-label="A 24-second reel: is everything fine? — Operational. The real phone and watch apps, a service needing attention, a restart that is asked rather than assumed, a shutdown that needs a hold, the three tiers of services, and the CueSeek logo."
        />
        <span className="reel-station" data-station="reel" aria-hidden="true" />
        {/* A tap on the picture plays or pauses it; this flashes which, as players do. */}
        {flash && (
          <span key={flash.n} className="reel-flash" aria-hidden="true">
            {flash.playing ? '▶' : '❚❚'}
          </span>
        )}
        {showSource && (
          <pre className="reel-source" aria-label={`The code for bar ${pos.bar + 1}`}>
            {block?.lines.map((l, i) => (
              <span key={i} className="ln" data-run={i === pos.line || undefined}>
                {l || ' '}
                {'\n'}
              </span>
            ))}
          </pre>
        )}
        {still && !playing && (
          <button type="button" className="reel-big-play" onClick={toggle}>
            ▶ Play the reel
          </button>
        )}
      </div>

      <div className="reel-controls">
        <button type="button" onClick={toggle} aria-pressed={playing}>
          <span ref={led} className="led" data-on={playing || undefined} aria-hidden="true" />
          {playing ? 'Pause' : 'Play'}
        </button>
        <button type="button" onClick={toggleSound} aria-pressed={sound} disabled={loading}>
          {loading ? 'Composing…' : sound ? 'Sound on' : 'Sound off'}
        </button>
        <button type="button" onClick={() => setShowSource((v) => !v)} aria-pressed={showSource}>
          {'</>'} View source
        </button>
        <span className="reel-bars" role="group" aria-label="Bars">
          {Array.from({ length: BARS }, (_, i) => (
            <button
              key={i}
              type="button"
              aria-label={`Bar ${i + 1}`}
              data-done={i < pos.bar || undefined}
              data-now={i === pos.bar || undefined}
              onClick={() => player.current?.seekBar(i)}
            />
          ))}
        </span>
        <span className="reel-note">bar {pos.bar + 1}/12 · ← → step</span>
      </div>
    </section>
  )
}
