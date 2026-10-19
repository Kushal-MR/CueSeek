/**
 * The reel's soundtrack, composed in the browser: no samples, no files, no services.
 * 24 s on the reel's own clock (120 BPM, four beats a bar), in C major — calm, the way a
 * product keynote is: warm pads, a soft electric-piano line, a gentle sub, a half-time beat,
 * and quiet chimes on the reel's events.
 *
 * It is played into an OfflineAudioContext — the browser's own oscillators, filters, reverb
 * and compressor — rendered once, the first time it is needed, to a plain stereo buffer the
 * player then plays in step with the picture.
 */

const SR = 32000 // plenty for a calm mix: nothing here needs the top octave above 16 kHz
export const DURATION = 24
/** The rendered length: three seconds longer, for the end's tail to ring over the loop. */
export const TAIL = 3
const BEAT = 0.5

/** Seconds, from a bar (1–12) and a beat within it. */
const at = (bar: number, beat = 0) => (bar - 1) * 2 + beat * BEAT
const hz = (midi: number) => 440 * 2 ** ((midi - 69) / 12)

/** One chord a bar, voiced for the pads. */
const CHORDS: number[][] = [
  [48, 55, 59, 62, 64], // 1 boot — Cmaj9
  [45, 52, 55, 59, 60], // 2 the question — Am9
  [41, 48, 52, 57, 59], // 3 the pile — Fmaj7♯11, a little unsettled
  [48, 55, 60, 62, 64], // 4 the answer — Cadd9, arriving
  [43, 50, 57, 59, 64], // 5 the phone — G6/9
  [45, 52, 55, 60, 64], // 6 inside — Am7
  [50, 53, 57, 60, 64], // 7 attention — Dm9, darker
  [46, 50, 53, 57, 60], // 8 control — B♭maj7, waiting
  [41, 48, 53, 55, 60], // 9 the hold — Fsus2
  [48, 55, 59, 62, 64], // 10 everywhere — Cmaj9
  [41, 48, 52, 55, 57], // 11 works with — Fmaj9, lifting
  [36, 48, 55, 59, 62, 64], // 12 end — Cmaj9, home
]
const ROOTS = [36, 33, 29, 36, 31, 33, 38, 34, 29, 36, 29, 36]
/** The piano's line: [beat, note] for the bars that have the groove. */
const MOTIF: Record<number, [number, number][]> = {
  4: [[0, 79], [0.5, 76], [1, 74], [2, 76], [3, 72]],
  5: [[0, 79], [0.5, 74], [1, 71], [2, 74], [3, 69]],
  6: [[0, 76], [0.5, 72], [1, 69], [2, 72], [3, 67]],
  10: [[0, 84], [0.5, 79], [1, 76], [2, 79], [3, 74]],
  11: [[0, 81], [0.5, 77], [1, 72], [2, 76], [3, 79]],
}
const GROOVE = [4, 5, 6, 10, 11]

export async function composeSoundtrack(): Promise<AudioBuffer> {
  const ctx = new OfflineAudioContext(2, SR * (DURATION + TAIL), SR)
  const rnd = mulberry(7)

  // ---- the mix: everything into a gentle compressor; a reverb and an echo on sends
  const master = ctx.createGain()
  const comp = ctx.createDynamicsCompressor()
  comp.threshold.value = -16
  comp.knee.value = 14
  comp.ratio.value = 2
  comp.attack.value = 0.012
  comp.release.value = 0.25
  const fader = ctx.createGain()
  fader.gain.setValueAtTime(1, 0)
  fader.gain.setValueAtTime(1, DURATION)
  fader.gain.linearRampToValueAtTime(0, DURATION + TAIL) // the tail rings out over bar 1
  master.connect(comp).connect(fader).connect(ctx.destination)

  const reverb = ctx.createConvolver()
  reverb.buffer = impulse(ctx, rnd)
  const wet = ctx.createGain()
  wet.gain.value = 0.42
  reverb.connect(wet).connect(master)

  const delay = ctx.createDelay(1)
  delay.delayTime.value = BEAT * 0.75 // a dotted eighth
  const feedback = ctx.createGain()
  feedback.gain.value = 0.32
  const darken = ctx.createBiquadFilter()
  darken.type = 'lowpass'
  darken.frequency.value = 2600
  delay.connect(darken).connect(feedback).connect(delay)
  const echoes = ctx.createGain()
  echoes.gain.value = 0.5
  darken.connect(echoes)
  echoes.connect(master)
  echoes.connect(reverb)

  // Pads and bass sit on a bus that dips under every kick, so the low end stays clean.
  const ducked = ctx.createGain()
  ducked.connect(master)
  const duck = (t: number) => {
    ducked.gain.setValueAtTime(1, t)
    ducked.gain.linearRampToValueAtTime(0.78, t + 0.012)
    ducked.gain.setTargetAtTime(1, t + 0.02, 0.11)
  }

  const noise = ctx.createBuffer(1, SR * 2, SR)
  noise.getChannelData(0).forEach((_, i, d) => (d[i] = rnd() * 2 - 1))

  /** A place in the mix: a pan, and how much goes to the reverb and to the echo. */
  const bus = (pan = 0, rev = 0.25, echo = 0, to: AudioNode = master) => {
    const p = ctx.createStereoPanner()
    p.pan.value = pan
    p.connect(to)
    if (rev) p.connect(gain(rev)).connect(reverb)
    if (echo) p.connect(gain(echo)).connect(delay)
    return p
  }
  const gain = (v: number) => {
    const g = ctx.createGain()
    g.gain.value = v
    return g
  }
  const osc = (type: OscillatorType, f: number, t0: number, t1: number) => {
    const o = ctx.createOscillator()
    o.type = type
    o.frequency.value = f
    o.start(t0)
    o.stop(t1)
    return o
  }

  // ---- instruments

  // The pad's tone: a triangle with a little sawtooth in it, as one waveform, so each voice
  // is a single oscillator — the pads are most of the score, and this keeps it quick.
  const harmonics = 24
  const real = new Float32Array(harmonics)
  const imag = new Float32Array(harmonics)
  for (let n = 1; n < harmonics; n++) {
    const saw = 0.35 / n
    const tri = n % 2 ? ((n % 4 === 1 ? 1 : -1) * 0.8) / (n * n) : 0
    imag[n] = saw + tri
  }
  const padWave = ctx.createPeriodicWave(real, imag)

  /** A warm pad: two softly detuned voices a note, through a low-pass, swelling in and out. */
  const padNow = (notes: number[], t0: number, t1: number, o: { level: number; attack?: number; release?: number; cutoff?: number; cutoffTo?: number }) => {
    const attack = o.attack ?? 0.35
    const release = o.release ?? 0.7
    const filter = ctx.createBiquadFilter()
    filter.type = 'lowpass'
    filter.Q.value = 0.4
    filter.frequency.setValueAtTime(o.cutoff ?? 1300, t0)
    if (o.cutoffTo) filter.frequency.exponentialRampToValueAtTime(o.cutoffTo, t1)
    const env = ctx.createGain()
    env.gain.setValueAtTime(0, t0)
    env.gain.linearRampToValueAtTime(o.level, t0 + attack)
    env.gain.setValueAtTime(o.level, t1)
    env.gain.linearRampToValueAtTime(0, t1 + release)
    filter.connect(env).connect(bus(0, 0.5, 0, ducked))
    for (const side of [-1, 1]) {
      // every note's voice on this side shares one panner and one level
      const pan = ctx.createStereoPanner()
      pan.pan.value = side * 0.45
      const level = gain(1 / notes.length)
      level.connect(pan).connect(filter)
      for (const n of notes) {
        const o = ctx.createOscillator()
        o.setPeriodicWave(padWave)
        o.frequency.value = hz(n)
        o.detune.value = side * 6
        o.connect(level)
        o.start(t0)
        o.stop(t1 + release + 0.1)
      }
    }
  }

  /** A soft electric piano: two-operator FM, the tone mellowing as it rings. */
  const pianoNow = (note: number, t: number, vel: number, pan = 0, echo = 0.35) => {
    const f = hz(note)
    const carrier = osc('sine', f, t, t + 2.6)
    const chorus = osc('sine', f, t, t + 2.6)
    chorus.detune.value = 7
    const mod = osc('sine', f, t, t + 2.6)
    const depth = ctx.createGain()
    depth.gain.setValueAtTime(f * 0.9 * vel + f * 0.15, t)
    depth.gain.exponentialRampToValueAtTime(f * 0.05, t + 1.1)
    mod.connect(depth).connect(carrier.frequency)
    depth.connect(chorus.frequency)
    const round = ctx.createBiquadFilter()
    round.type = 'lowpass'
    round.frequency.value = 3200
    const tine = osc('sine', f * 4, t, t + 0.4) // the bright edge of the strike
    const tineEnv = ctx.createGain()
    tineEnv.gain.setValueAtTime(0.03 * vel, t)
    tineEnv.gain.exponentialRampToValueAtTime(0.0001, t + 0.25)
    const env = ctx.createGain()
    env.gain.setValueAtTime(0, t)
    env.gain.linearRampToValueAtTime(0.22 * vel, t + 0.005)
    env.gain.exponentialRampToValueAtTime(0.0001, t + 2.5)
    const out = bus(pan, 0.3, echo)
    carrier.connect(round)
    chorus.connect(gain(0.6)).connect(round)
    round.connect(env).connect(out)
    tine.connect(tineEnv).connect(out)
  }

  /** A glass chime: inharmonic FM, long and quiet. The reel's events speak in these. */
  const chimeNow = (note: number, t: number, vel: number, pan = 0) => {
    const f = hz(note)
    const carrier = osc('sine', f, t, t + 3.2)
    const mod = osc('sine', f * 2, t, t + 3.2)
    const depth = ctx.createGain()
    depth.gain.setValueAtTime(f * 0.45, t)
    depth.gain.exponentialRampToValueAtTime(f * 0.01, t + 1.2)
    mod.connect(depth).connect(carrier.frequency)
    const soft = ctx.createBiquadFilter()
    soft.type = 'lowpass'
    soft.frequency.value = 5000
    const env = ctx.createGain()
    env.gain.setValueAtTime(0, t)
    env.gain.linearRampToValueAtTime(0.14 * vel, t + 0.006)
    env.gain.exponentialRampToValueAtTime(0.0001, t + 3)
    carrier.connect(soft).connect(env).connect(bus(pan, 0.55, 0.15))
  }

  /** A soft interface pip: a rounded triangle, gone in a moment. */
  const pipNow = (note: number, t: number, vel: number, pan = 0, len = 0.18) => {
    const o = osc('triangle', hz(note), t, t + len + 0.05)
    const lp = ctx.createBiquadFilter()
    lp.type = 'lowpass'
    lp.frequency.value = 3800
    const env = ctx.createGain()
    env.gain.setValueAtTime(0, t)
    env.gain.linearRampToValueAtTime(0.12 * vel, t + 0.003)
    env.gain.exponentialRampToValueAtTime(0.0001, t + len)
    o.connect(lp).connect(env).connect(bus(pan, 0.35))
  }

  /** A card turning over: a short wooden tok. */
  const tokNow = (t: number, vel: number, pan = 0) => {
    const o = osc('sine', 560, t, t + 0.2)
    o.frequency.exponentialRampToValueAtTime(240, t + 0.06)
    const env = ctx.createGain()
    env.gain.setValueAtTime(0.25 * vel, t)
    env.gain.exponentialRampToValueAtTime(0.0001, t + 0.13)
    o.connect(env).connect(bus(pan, 0.2))
  }

  /** Air moving past: filtered noise, swept, very low. */
  const swishNow = (t: number, len: number, vel: number, panFrom: number, panTo: number, up = true) => {
    const src = ctx.createBufferSource()
    src.buffer = noise
    const bp = ctx.createBiquadFilter()
    bp.type = 'bandpass'
    bp.Q.value = 0.9
    bp.frequency.setValueAtTime(up ? 500 : 2400, t)
    bp.frequency.exponentialRampToValueAtTime(up ? 2400 : 500, t + len)
    const env = ctx.createGain()
    env.gain.setValueAtTime(0, t)
    env.gain.linearRampToValueAtTime(0.09 * vel, t + len * 0.65)
    env.gain.linearRampToValueAtTime(0, t + len)
    const pan = bus(panFrom, 0.4)
    pan.pan.linearRampToValueAtTime(panTo, t + len)
    src.connect(bp).connect(env).connect(pan)
    src.start(t, rnd() * 1.2, len + 0.05)
  }

  const kickNow = (t: number, vel: number) => {
    const o = osc('sine', 120, t, t + 0.3)
    o.frequency.exponentialRampToValueAtTime(48, t + 0.08)
    const env = ctx.createGain()
    env.gain.setValueAtTime(0.65 * vel, t)
    env.gain.exponentialRampToValueAtTime(0.0001, t + 0.22)
    const out = bus(0, 0.04)
    o.connect(env).connect(out)
    const click = osc('triangle', 1800, t, t + 0.02)
    const clickEnv = ctx.createGain()
    clickEnv.gain.setValueAtTime(0.05 * vel, t)
    clickEnv.gain.exponentialRampToValueAtTime(0.0001, t + 0.012)
    click.connect(clickEnv).connect(out)
    duck(t)
  }

  const shakerNow = (t: number, vel: number, pan: number) => {
    const src = ctx.createBufferSource()
    src.buffer = noise
    const bp = ctx.createBiquadFilter()
    bp.type = 'bandpass'
    bp.frequency.value = 6000
    bp.Q.value = 0.7
    const env = ctx.createGain()
    env.gain.setValueAtTime(0, t)
    env.gain.linearRampToValueAtTime(0.24 * vel, t + 0.006)
    env.gain.exponentialRampToValueAtTime(0.0001, t + 0.07)
    src.connect(bp).connect(env).connect(bus(pan, 0.15))
    src.start(t, rnd() * 1.5, 0.1)
  }

  const subNow = (note: number, t0: number, t1: number, level: number) => {
    const o = osc('sine', hz(note), t0, t1 + 0.5)
    const env = ctx.createGain()
    env.gain.setValueAtTime(0, t0)
    env.gain.linearRampToValueAtTime(level, t0 + 0.06)
    env.gain.setValueAtTime(level, t1)
    env.gain.linearRampToValueAtTime(0, t1 + 0.35)
    o.connect(env).connect(bus(0, 0, 0, ducked))
  }

  // ---- just in time: the score is written now, but each bar's sounds are only built shortly
  // before that bar plays. A node that exists is a node the renderer works on, started or
  // not; built all at once, the score is several times slower to compose.
  const plan: { t: number; make: () => void }[] = []
  const later =
    <A extends unknown[]>(when: (...a: A) => number, make: (...a: A) => void) =>
    (...a: A) => {
      plan.push({ t: when(...a), make: () => make(...a) })
    }
  const pad = later((...a: Parameters<typeof padNow>) => a[1] as number, padNow)
  const piano = later((...a: Parameters<typeof pianoNow>) => a[1] as number, pianoNow)
  const chime = later((...a: Parameters<typeof chimeNow>) => a[1] as number, chimeNow)
  const pip = later((...a: Parameters<typeof pipNow>) => a[1] as number, pipNow)
  const tok = later((...a: Parameters<typeof tokNow>) => a[0] as number, tokNow)
  const swish = later((...a: Parameters<typeof swishNow>) => a[0] as number, swishNow)
  const kick = later((...a: Parameters<typeof kickNow>) => a[0] as number, kickNow)
  const shaker = later((...a: Parameters<typeof shakerNow>) => a[0] as number, shakerNow)
  const sub = later((...a: Parameters<typeof subNow>) => a[1] as number, subNow)

  // ---- the arrangement, bar by bar

  // the pads under everything: quiet at the edges, opening at the answer, dark at the doubt
  CHORDS.forEach((notes, i) => {
    const bar = i + 1
    const t0 = at(bar)
    const t1 = at(bar + 1)
    if (bar === 1) return pad(notes, t0, t1, { level: 0.15, attack: 1.3, cutoff: 600, cutoffTo: 1400 })
    if (bar === 9) return pad(notes, t0, at(9, 2.9), { level: 0.07, cutoff: 900, release: 0.4 })
    if (bar === 12) return pad(notes, t0, at(12, 3.4), { level: 0.13, attack: 0.9, cutoff: 1800, release: 2.6 })
    const dark = bar === 7 || bar === 8
    pad(notes, t0, t1, { level: bar < 4 ? 0.15 : dark ? 0.09 : 0.1, cutoff: dark ? 850 : bar === 4 ? 2200 : 1500 })
  })

  // the bass, from the answer on; it steps back for the doubt and leaves for the hold
  ROOTS.forEach((root, i) => {
    const bar = i + 1
    if (bar < 4 || bar === 9) return
    sub(root, at(bar), bar === 12 ? at(12, 3.2) : at(bar + 1) - 0.05, bar === 7 || bar === 8 ? 0.1 : 0.15)
  })

  // the groove: a half-time kick and a shaker, only where things are well
  for (const bar of GROOVE) {
    if (bar !== 11) kick(at(bar, 0), bar === 4 || bar === 10 ? 0.5 : 0.65) // the returns come in softly
    kick(at(bar, 2), 0.5)
    for (let e = 0; e < 8; e++) shaker(at(bar, e / 2), e % 2 ? 0.6 : 0.3, e % 2 ? 0.3 : -0.2)
    for (const [beat, note] of MOTIF[bar]) {
      if (bar === 11 && beat < 1) continue // the cards turning over have the first beat
      piano(note, at(bar, beat), beat % 1 ? 0.38 : 0.5, beat % 1 ? 0.25 : -0.1)
    }
  }

  // bar 1 — boot: the dot appears; the rings go out like sonar
  chime(84, at(1, 0.25), 0.5)
  for (const beat of [1, 2, 3]) pip(67, at(1, beat), 0.35, 0, 0.4)

  // bar 2 — the question: one note a word, rising; the dot, an answer not yet given
  piano(76, at(2, 0), 0.5, -0.2)
  piano(79, at(2, 1), 0.5, 0)
  piano(83, at(2, 2), 0.55, 0.2)
  chime(88, at(2, 3), 0.55)

  // bar 3 — the pile: every service pipes up; a few clamour; then they gather into the dot
  ;[77, 81, 84, 88, 79, 83, 86, 81].forEach((n, i) => pip(n, at(3, i * 0.32), 0.45, i % 2 ? 0.5 : -0.5))
  for (const beat of [1.6, 2.3, 2.9]) {
    pip(81, at(3, beat), 0.22, rnd() - 0.5)
    pip(81, at(3, beat) + 0.09, 0.16, rnd() - 0.5) // a gentle double knock: look at me
  }
  swish(at(3, 3.1), 0.45, 0.8, -0.3, 0.3)
  pad(CHORDS[3], at(3, 2.2), at(4), { level: 0.1, attack: 0.9, cutoff: 500, cutoffTo: 2200, release: 0.2 })

  // bar 4 — the answer: it arrives
  chime(84, at(4, 0), 0.55, -0.15)
  chime(91, at(4, 0) + 0.04, 0.35, 0.15)

  // bar 5 — the phone: a pip as each part is pointed at, top to bottom
  ;[86, 83, 79].forEach((n, i) => pip(n, at(5, 0.8 + i), 0.4, 0.2))

  // bar 6 — inside: a page slides in from the left, CueSeek's view from the right
  for (const k of [0, 2]) {
    swish(at(6, k), 0.35, 0.6, -0.6, -0.1)
    swish(at(6, k + 0.8), 0.35, 0.6, 0.6, 0.1)
  }

  // bar 7 — attention: the groove stops; a warm two-note alert, falling
  chime(76, at(7, 0), 0.6, -0.1)
  chime(72, at(7, 0.5), 0.55, 0.1)

  // bar 8 — control: tap, confirm; "asked" — then Running, resolving upward
  pip(81, at(8, 0.94), 0.45, 0, 0.09)
  pip(81, at(8, 1.94), 0.45, 0, 0.09)
  for (const k of [0.2, 0.36, 0.52]) pip(88, at(8, 2 + k), 0.16, 0.1, 0.06)
  chime(72, at(8, 3), 0.45, -0.1)
  chime(79, at(8, 3) + 0.12, 0.5, 0.1)

  // bar 9 — the hold: a swell that builds with the bar for 1200 ms, then the finger lifts
  pad([53, 60, 65, 67, 72], at(9, 0.6), at(9, 2.9), { level: 0.16, attack: 1.15, cutoff: 300, cutoffTo: 3200, release: 0.6 })
  swish(at(9, 3), 0.5, 0.7, -0.2, 0.2, true)
  pad(CHORDS[9], at(9, 3.05), at(10), { level: 0.07, attack: 0.42, cutoff: 400, cutoffTo: 1800, release: 0.15 })

  // bar 10 — everywhere: the watch answers the phone, across the stereo
  pip(91, at(10, 1), 0.3, 0.6)
  chime(88, at(10, 1.44), 0.4, -0.55)
  chime(88, at(10, 2), 0.32, 0.55)

  // bar 11 — works with: three cards turn over; a lift toward home
  ;[0, 0.3, 0.6].forEach((beat, i) => tok(at(11, beat), 0.45, (i - 1) * 0.5))
  pad(CHORDS[10], at(11, 2.4), at(12), { level: 0.06, attack: 0.8, cutoff: 700, cutoffTo: 2600, release: 0.3 })
  swish(at(11, 3), 0.5, 0.5, -0.2, 0.2)

  // bar 12 — the end card: the arc draws, the dot lands, everything resolves home
  const land = at(12, 0.95)
  sub(36, land, DURATION + 1.6, 0.1) // the bass stays under the last chord as it rings out
  chime(84, land, 0.6, -0.2)
  chime(88, land + 0.03, 0.45, 0)
  chime(91, land + 0.06, 0.4, 0.2)
  piano(72, land, 0.45, 0, 0.2)

  // Build each bar's sounds 0.4 s before the bar begins; the first bar's, before rendering.
  const lead = 0.4
  const groups = new Map<number, (() => void)[]>()
  for (const { t, make } of plan) {
    const bar = Math.min(12, Math.floor(t / 2) + 1)
    const ready = Math.max(0, at(bar) - lead)
    if (!groups.has(ready)) groups.set(ready, [])
    groups.get(ready)!.push(make)
  }
  for (const [ready, makes] of groups) {
    if (ready === 0) makes.forEach((m) => m())
    else
      void ctx.suspend(ready).then(() => {
        makes.forEach((m) => m())
        void ctx.resume()
      })
  }
  const buffer = await ctx.startRendering()
  normalise(buffer, 0.89)
  return buffer
}

/** A room: two channels of noise, decaying and darkening, after a short pre-delay. */
function impulse(ctx: BaseAudioContext, rnd: () => number) {
  const len = Math.floor(SR * 2.6)
  const ir = ctx.createBuffer(2, len, SR)
  for (let ch = 0; ch < 2; ch++) {
    const d = ir.getChannelData(ch)
    let lp = 0
    for (let i = Math.floor(SR * 0.018); i < len; i++) {
      const t = i / SR
      const a = 0.85 * Math.exp(-t / 0.9) + 0.08 // the tail loses its highs as it fades
      lp += a * (rnd() * 2 - 1 - lp)
      d[i] = lp * Math.exp(-t / 0.85)
    }
  }
  return ir
}

function normalise(buffer: AudioBuffer, to: number) {
  let peak = 0
  for (let c = 0; c < buffer.numberOfChannels; c++) for (const v of buffer.getChannelData(c)) peak = Math.max(peak, Math.abs(v))
  const g = peak ? to / peak : 1
  for (let c = 0; c < buffer.numberOfChannels; c++) {
    const d = buffer.getChannelData(c)
    for (let i = 0; i < d.length; i++) d[i] *= g
  }
}

/** A small seeded generator, so the soundtrack is the same on every visit. */
function mulberry(seed: number) {
  let a = seed
  return () => {
    a = (a + 0x6d2b79f5) | 0
    let t = Math.imul(a ^ (a >>> 15), 1 | a)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}
