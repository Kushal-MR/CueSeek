import dashboard from '../assets/dashboard.webp'
import jfPage from '../assets/screens/web-jellyfin.webp'
import jfNow from '../assets/screens/cueseek-jellyfin-now.webp'
import qbPage from '../assets/screens/web-qbittorrent.webp'
import qbNow from '../assets/screens/cueseek-qbittorrent-transfers.webp'
import watch from '../assets/screens/watch-dashboard.webp'
import complication from '../assets/screens/watch-complication.webp'
import { BAR_SECONDS, LENGTH, type Images, type Palette, Stage } from './kit.ts'
import { SCENES } from './scenes.ts'
import { renderSoundtrack } from './synth.ts'

const SOURCES = { dashboard, jfPage, jfNow, qbPage, qbNow, watch, complication }
const TOKENS = ['bg', 'card', 'text', 'muted', 'accent', 'hi', 'line', 'ok', 'warn', 'bad', 'glow'] as const

/**
 * Plays the reel on a canvas: a clock, a frame loop, and — once the visitor asks for it —
 * the synthesised soundtrack, kept in step with the picture. Nothing is pre-rendered; every
 * frame is drawn from `scenes.ts` at the moment it is shown, in the page's current colours.
 */
export class ReelPlayer {
  private canvas: HTMLCanvasElement
  private ctx: CanvasRenderingContext2D
  private images: Images
  private raf = 0
  private playing = false
  private time = 0
  private startedAt = 0 // performance.now() when time was last `startTime`
  private startTime = 0
  private audio: AudioContext | null = null
  private buffer: AudioBuffer | null = null
  private source: AudioBufferSourceNode | null = null
  private sound = false
  /** Told the time on every frame, for the controls and the source view. */
  onTick: (t: number) => void = () => {}

  constructor(canvas: HTMLCanvasElement) {
    this.canvas = canvas
    this.ctx = canvas.getContext('2d')!
    this.images = Object.fromEntries(
      Object.entries(SOURCES).map(([k, src]) => {
        const im = new Image()
        im.decoding = 'async'
        im.src = src
        im.onload = () => this.draw()
        return [k, im]
      }),
    ) as Images
    document.fonts?.ready.then(() => this.draw())
  }

  get isPlaying() {
    return this.playing
  }
  get hasSound() {
    return this.sound
  }
  get now() {
    return this.time
  }

  play() {
    if (this.playing) return
    this.playing = true
    this.startTime = this.time
    this.startedAt = performance.now()
    this.startAudio()
    const frame = () => {
      if (!this.playing) return
      let t = this.startTime + (performance.now() - this.startedAt) / 1000
      if (t >= LENGTH) {
        // Loop: back to bar 1, picture and sound together.
        t %= LENGTH
        this.startTime = t
        this.startedAt = performance.now() - 0
        this.startAudio()
      }
      this.time = t
      this.draw()
      this.raf = requestAnimationFrame(frame)
    }
    this.raf = requestAnimationFrame(frame)
  }

  pause() {
    this.playing = false
    cancelAnimationFrame(this.raf)
    this.stopAudio()
    this.draw()
  }

  seek(t: number) {
    this.time = ((t % LENGTH) + LENGTH) % LENGTH
    this.startTime = this.time
    this.startedAt = performance.now()
    if (this.playing) this.startAudio()
    this.draw()
  }

  seekBar(bar: number) {
    this.seek(bar * BAR_SECONDS + 0.001)
  }

  /** Turn the sound on or off. The first time, the soundtrack is synthesised (≈ a second). */
  async setSound(on: boolean) {
    this.sound = on
    if (!on) return this.stopAudio()
    this.audio ??= new AudioContext()
    // Compose first (≈ 0.4 s, after a frame so "Composing…" can show); the context starts on
    // this same click, but is not waited for.
    if (!this.buffer) await new Promise((r) => setTimeout(r, 40))
    this.buffer ??= renderSoundtrack(this.audio)
    void this.audio.resume()
    // Re-anchor to now: rendering took a moment, and the picture kept going.
    this.startTime = this.time
    this.startedAt = performance.now()
    if (this.playing) this.startAudio()
  }

  private startAudio() {
    this.stopAudio()
    if (!this.sound || !this.audio || !this.buffer || !this.playing) return
    const src = this.audio.createBufferSource()
    src.buffer = this.buffer
    src.connect(this.audio.destination)
    src.start(0, this.time)
    this.source = src
  }

  private stopAudio() {
    try {
      this.source?.stop()
    } catch {
      // already stopped
    }
    this.source = null
  }

  /** Size the canvas to its box, at the screen's own pixel density. */
  resize() {
    const r = this.canvas.getBoundingClientRect()
    const dpr = Math.min(2, devicePixelRatio || 1)
    this.canvas.width = Math.max(1, Math.round(r.width * dpr))
    this.canvas.height = Math.max(1, Math.round(r.height * dpr))
    this.draw()
  }

  draw() {
    const { canvas, ctx } = this
    if (!canvas.width) return
    const css = getComputedStyle(document.documentElement)
    const c = Object.fromEntries(TOKENS.map((k) => [k, css.getPropertyValue(`--${k}`).trim() || '#000'])) as unknown as Palette
    const portrait = canvas.height > canvas.width
    const s = new Stage(ctx, c, this.images, portrait)
    ctx.setTransform(1, 0, 0, 1, 0, 0)
    ctx.globalAlpha = 1
    ctx.fillStyle = c.bg
    ctx.fillRect(0, 0, canvas.width, canvas.height)
    ctx.setTransform(canvas.width / s.w, 0, 0, canvas.height / s.h, 0, 0)
    const t = Math.min(this.time, LENGTH - 1e-3)
    s.t = t
    s.bar = Math.floor(t / BAR_SECONDS)
    s.b = ((t % BAR_SECONDS) / BAR_SECONDS) * 4
    SCENES[s.bar](s)
    ctx.globalAlpha = 1
    s.hud(1 - Math.max(0, (t - (LENGTH - 0.4)) / 0.4))
    s.vignette()
    this.onTick(t)
  }

  destroy() {
    this.pause()
    this.audio?.close()
  }
}
