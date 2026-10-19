/**
 * The reel's drawing kit: a canvas, a clock, the site's colours, and a handful of helpers
 * the scenes are written in. Scenes describe *when* things happen in beats; the kit turns
 * that into pixels, at whatever size and theme the page is in.
 */

export const BPM = 120
export const BARS = 12
export const BAR_SECONDS = 2
export const LENGTH = BARS * BAR_SECONDS

export interface Palette {
  bg: string
  card: string
  text: string
  muted: string
  accent: string
  hi: string
  line: string
  ok: string
  warn: string
  bad: string
  glow: string
}

export type Images = Record<'dashboard' | 'jfPage' | 'jfNow' | 'qbPage' | 'qbNow' | 'watch' | 'complication', HTMLImageElement>

interface TextOptions {
  size: number
  weight?: 400 | 500
  mono?: boolean
  align?: CanvasTextAlign
  color?: string
  alpha?: number
  spacing?: number
}

// ---- easing
export const clamp = (v: number, a = 0, b = 1) => (v < a ? a : v > b ? b : v)
export const lerp = (a: number, b: number, t: number) => a + (b - a) * t
export const out = (x: number) => 1 - (1 - x) ** 3
export const outExpo = (x: number) => (x >= 1 ? 1 : 1 - 2 ** (-10 * x))
export const inCubic = (x: number) => x * x * x
export const inOut = (x: number) => (x < 0.5 ? 4 * x * x * x : 1 - (-2 * x + 2) ** 3 / 2)
export const outBack = (x: number) => 1 + 2.70158 * (x - 1) ** 3 + 1.70158 * (x - 1) ** 2
export function bounce(x: number) {
  const n = 7.5625
  const d = 2.75
  if (x < 1 / d) return n * x * x
  if (x < 2 / d) return n * (x -= 1.5 / d) * x + 0.75
  if (x < 2.5 / d) return n * (x -= 2.25 / d) * x + 0.9375
  return n * (x -= 2.625 / d) * x + 0.984375
}
export const typed = (s: string, p: number) => s.slice(0, Math.floor(clamp(p) * s.length))

/** Mix two `#rrggbb` colours. */
export function mix(a: string, b: string, t: number) {
  const pa = hex(a)
  const pb = hex(b)
  const c = pa.map((v, i) => Math.round(lerp(v, pb[i], clamp(t))))
  return `rgb(${c[0]} ${c[1]} ${c[2]})`
}
function hex(c: string) {
  const m = c.trim().match(/^#?([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})/i)
  return m ? [1, 2, 3].map((i) => parseInt(m[i], 16)) : [0, 0, 0]
}

/**
 * One frame's worth of drawing. The canvas is a virtual 1920×1080 — or 1080×1350 when the
 * player is portrait (a phone, and the 4:5 export) — scaled to the real pixels.
 */
export class Stage {
  readonly w: number
  readonly h: number
  /** Portrait: compose for 4:5 rather than 16:9. */
  readonly p: boolean
  readonly cx: number
  readonly cy: number
  /** Beats into the current bar, 0–4. */
  b = 0
  /** Seconds into the reel. */
  t = 0
  bar = 0
  private hudLabel = ''
  readonly ctx: CanvasRenderingContext2D
  readonly c: Palette
  readonly img: Images

  constructor(ctx: CanvasRenderingContext2D, c: Palette, img: Images, portrait: boolean) {
    this.ctx = ctx
    this.c = c
    this.img = img
    this.p = portrait
    this.w = portrait ? 1080 : 1920
    this.h = portrait ? 1350 : 1080
    this.cx = this.w / 2
    this.cy = this.h / 2
  }

  /** 0 before `beat`, rising to 1 over `len` beats after it. */
  at(beat: number, len = 0.5) {
    return clamp((this.b - beat) / len)
  }

  label(s: string) {
    this.hudLabel = s
  }

  font(o: TextOptions) {
    return `${o.weight ?? 400} ${o.size}px ${o.mono ? "'IBM Plex Mono', monospace" : "'IBM Plex Sans', sans-serif"}`
  }

  textW(s: string, o: TextOptions) {
    const g = this.ctx
    g.font = this.font(o)
    g.letterSpacing = `${(o.spacing ?? 0) * o.size}px`
    return g.measureText(s).width
  }

  text(s: string, x: number, y: number, o: TextOptions) {
    if (!s || (o.alpha ?? 1) <= 0) return
    const g = this.ctx
    g.save()
    g.font = this.font(o)
    g.letterSpacing = `${(o.spacing ?? 0) * o.size}px`
    g.textAlign = o.align ?? 'left'
    g.textBaseline = 'alphabetic'
    g.globalAlpha *= clamp(o.alpha ?? 1)
    g.fillStyle = o.color ?? this.c.text
    g.fillText(s, x, y)
    g.restore()
  }

  /** The largest size, up to `max`, at which `s` fits in `width`. */
  fit(s: string, max: number, width: number, o: Omit<TextOptions, 'size'> = {}) {
    const w = this.textW(s, { ...o, size: 100 })
    return Math.min(max, (width / w) * 100)
  }

  dot(x: number, y: number, r: number, color = this.c.hi, alpha = 1, glow = 0) {
    if (r <= 0 || alpha <= 0) return
    const g = this.ctx
    g.save()
    g.globalAlpha *= clamp(alpha)
    g.fillStyle = color
    if (glow) {
      g.shadowColor = this.c.glow
      g.shadowBlur = glow
    }
    g.beginPath()
    g.arc(x, y, r, 0, Math.PI * 2)
    g.fill()
    g.restore()
  }

  ring(x: number, y: number, r: number, color: string, alpha = 1, width = 2) {
    if (r <= 0 || alpha <= 0) return
    const g = this.ctx
    g.save()
    g.globalAlpha *= clamp(alpha)
    g.strokeStyle = color
    g.lineWidth = width
    g.beginPath()
    g.arc(x, y, r, 0, Math.PI * 2)
    g.stroke()
    g.restore()
  }

  box(x: number, y: number, w: number, h: number, r: number, fill?: string, stroke?: string, alpha = 1) {
    if (alpha <= 0) return
    const g = this.ctx
    g.save()
    g.globalAlpha *= clamp(alpha)
    g.beginPath()
    g.roundRect(x, y, w, h, r)
    if (fill) {
      g.fillStyle = fill
      g.fill()
    }
    if (stroke) {
      g.strokeStyle = stroke
      g.lineWidth = 2
      g.stroke()
    }
    g.restore()
  }

  /**
   * A capture, `w` wide at its own aspect (or cropped to `h`), in a dark device bezel.
   * `round` makes it a watch face.
   */
  shot(
    im: HTMLImageElement,
    x: number,
    y: number,
    w: number,
    o: { alpha?: number; h?: number; round?: boolean; bezel?: number; from?: number } = {},
  ) {
    const alpha = o.alpha ?? 1
    if (alpha <= 0 || !im.naturalWidth) return
    const h = o.h ?? (w * im.naturalHeight) / im.naturalWidth
    const bz = o.bezel ?? 10
    const g = this.ctx
    g.save()
    g.globalAlpha *= clamp(alpha)
    g.shadowColor = 'rgb(0 0 0 / 0.55)'
    g.shadowBlur = 50
    g.shadowOffsetY = 24
    g.fillStyle = o.round ? '#5d655c' : '#050605'
    g.beginPath()
    if (o.round) g.arc(x + w / 2, y + h / 2, w / 2 + bz, 0, Math.PI * 2)
    else g.roundRect(x - bz, y - bz, w + bz * 2, h + bz * 2, o.round ? w : 26 + bz)
    g.fill()
    g.shadowColor = 'transparent'
    g.beginPath()
    if (o.round) g.arc(x + w / 2, y + h / 2, w / 2, 0, Math.PI * 2)
    else g.roundRect(x, y, w, h, 22)
    g.clip()
    // `from`: where in the picture to start, as a fraction of its height (a camera move).
    const srcH = (im.naturalWidth * h) / w
    const sy = clamp(o.from ?? 0) * Math.max(0, im.naturalHeight - srcH)
    g.drawImage(im, 0, sy, im.naturalWidth, Math.min(srcH, im.naturalHeight), x, y, w, Math.min(h, (w * im.naturalHeight) / im.naturalWidth))
    g.restore()
  }

  /** The frame around every scene: corner marks, the bar counter, a label, the timecode. */
  hud(alpha = 1) {
    const { w, h, c } = this
    const g = this.ctx
    const m = this.p ? 44 : 60
    const len = 50
    const in_ = outExpo(clamp(this.t / 0.55))
    const off = (1 - in_) * 46
    const a = alpha * in_
    g.save()
    g.globalAlpha = a
    g.strokeStyle = c.line
    g.lineWidth = 3
    g.beginPath()
    for (const [x, y, dx, dy] of [
      [m - off, m - off, 1, 1],
      [w - m + off, m - off, -1, 1],
      [m - off, h - m + off, 1, -1],
      [w - m + off, h - m + off, -1, -1],
    ]) {
      g.moveTo(x, y + dy * len)
      g.lineTo(x, y)
      g.lineTo(x + dx * len, y)
    }
    g.stroke()
    g.restore()
    const o = { size: this.p ? 20 : 22, mono: true, color: c.muted, alpha: a * 0.9, spacing: 0.14 }
    const top = m + 28
    const bottom = h - m - 12
    this.text(typed('CUESEEK / MOTION REEL', clamp((this.t - 0.15) / 0.6)), m + 70, top, o)
    this.text(`BAR ${this.bar + 1}/${BARS} · ${BPM} BPM`, w - m - 70, top, { ...o, align: 'right' })
    this.text(typed(this.hudLabel, this.at(0, 0.6)), m + 70, bottom, o)
    const sec = Math.floor(this.t)
    const ff = Math.floor((this.t - sec) * 30)
    if (!this.p) this.text(`00:00:${pad(sec)}:${pad(ff)}`, w - m - 70, bottom, { ...o, align: 'right' })
    g.save()
    g.globalAlpha = a * 0.6
    g.fillStyle = c.line
    g.fillRect(m + 70, h - m + 10, (w - 2 * m - 140) * (this.t / LENGTH), 2)
    g.restore()
  }

  vignette() {
    const g = this.ctx
    const r = Math.hypot(this.w, this.h) / 2
    const grad = g.createRadialGradient(this.cx, this.cy, r * 0.45, this.cx, this.cy, r)
    grad.addColorStop(0, 'rgb(0 0 0 / 0)')
    grad.addColorStop(1, 'rgb(0 0 0 / 0.28)')
    g.fillStyle = grad
    g.fillRect(0, 0, this.w, this.h)
  }
}

const pad = (n: number) => String(n).padStart(2, '0')
