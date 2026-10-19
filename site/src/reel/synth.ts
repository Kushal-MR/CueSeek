/**
 * The reel's soundtrack, synthesised in the browser: no samples, no files, no services.
 * 24 s, 120 BPM, A minor — the M5 proof's synth (Synth.java), re-timed to the twelve bars.
 *
 * Rendered once, the first time a visitor turns the sound on, into a plain stereo buffer the
 * player then plays in step with the picture. Every event sits on the same clock as the
 * scenes, so the cuts and the music agree.
 */

const SR = 44100
export const DURATION = 24
const BEAT = 0.5
const SIX = 0.125
const N = SR * DURATION

/** Twelve bars of chords, root first, as MIDI notes. */
const CHORDS: number[][] = [
  [57, 60, 64], // 1 boot — Am
  [57, 60, 64], // 2 the question — Am
  [53, 57, 60], // 3 the pile — F
  [48, 52, 55], // 4 the answer — C
  [55, 59, 62], // 5 the phone — G
  [57, 60, 64], // 6 inside — Am
  [50, 53, 57], // 7 attention — Dm
  [57, 60, 64], // 8 control — Am, once "asked"
  [53, 57, 60], // 9 the hold — F
  [48, 52, 55], // 10 everywhere — C
  [55, 59, 62], // 11 works with — G
  [48, 52, 55], // 12 end — C
]

const hz = (midi: number) => 440 * 2 ** ((midi - 69) / 12)
/** The hold's silence: the threshold is crossed and everything stops for a breath. */
const muted = (t: number) => t >= 17.5 && t < 17.98
/** Attention and control: the arp drops out and a semitone refuses to settle. */
const doubt = (t: number) => t >= 12 && t < 15

export function renderSoundtrack(ctx: BaseAudioContext): AudioBuffer {
  const L = new Float32Array(N)
  const R = new Float32Array(N)
  const VERB = new Float32Array(N)
  const DUCK = new Float32Array(N).fill(1)
  const rnd = mulberry(11)

  const add = (i: number, l: number, r: number) => {
    if (i >= 0 && i < N) {
      L[i] += l
      R[i] += r
    }
  }
  const addVerb = (i: number, v: number) => {
    if (i >= 0 && i < N) VERB[i] += v
  }

  // ---- kick, and the sidechain duck it leaves on everything else
  const kick = (t0: number, amp: number) => {
    const s0 = Math.floor(t0 * SR)
    let ph = 0
    for (let k = 0; k < SR * 0.45; k++) {
      const t = k / SR
      ph += (2 * Math.PI * (44 + 80 * Math.exp(-t / 0.032))) / SR
      const click = k < 120 ? (rnd() * 2 - 1) * (1 - k / 120) * 0.25 : 0
      const v = amp * (Math.sin(ph) * Math.exp(-t / 0.26) + click)
      add(s0 + k, v, v)
    }
    for (let k = 0; k < SR * 0.3; k++) {
      const i = s0 + k
      if (i < N) DUCK[i] = Math.min(DUCK[i], 1 - 0.55 * Math.exp(-k / (SR * 0.09)))
    }
  }
  for (let t = 0; t < 22; t += BEAT) {
    if (muted(t)) continue
    kick(t, t < 2 ? 0.55 : doubt(t) ? 0.6 : 0.82)
  }
  kick(22, 0.9)
  kick(22.5, 0.7)

  // ---- sub bass
  {
    let ph = 0
    for (let i = 4 * SR; i < 22 * SR; i++) {
      const t = i / SR
      if (muted(t)) continue
      const bar = Math.floor(t / 2)
      let root = CHORDS[bar][0] - 24
      if (bar === 7 && t < 15) root = 40 // E under the doubt, until "asked"
      ph += (2 * Math.PI * hz(root)) / SR
      const v = 0.26 * Math.sin(ph) * Math.min(1, (t % 2) / 0.02) * 0.95 * DUCK[i]
      add(i, v, v)
    }
    let p2 = 0
    for (let i = 22 * SR; i < N; i++) {
      const t = i / SR - 22
      p2 += (2 * Math.PI * hz(36)) / SR
      const v = 0.3 * Math.sin(p2) * Math.exp(-t / 1.6)
      add(i, v, v)
    }
  }

  // ---- arpeggio
  const pluck = (t0: number, f: number, amp: number, pan: number, cutoff: number, send: number) => {
    const s0 = Math.floor(t0 * SR)
    let p1 = rnd()
    let p2 = rnd()
    let lp1 = 0
    let lp2 = 0
    const a = 1 - Math.exp((-2 * Math.PI * cutoff) / SR)
    for (let k = 0; k < SR * 0.6; k++) {
      const i = s0 + k
      if (i >= N) break
      const t = k / SR
      p1 += f / SR
      p2 += (f * 1.004) / SR
      const saw = (2 * (p1 % 1) - 1) * 0.5 + (2 * (p2 % 1) - 1) * 0.5
      const env = Math.min(1, t / 0.003) * Math.exp(-t / 0.16)
      const cut = a * (0.35 + 0.65 * Math.exp(-t / 0.08))
      lp1 += cut * (saw - lp1)
      lp2 += cut * (lp1 - lp2)
      let v = lp2 * env * amp * DUCK[i]
      if (muted(i / SR)) v = 0
      add(i, v * (1 - pan) * 1.4, v * pan * 1.4)
      addVerb(i, v * send)
    }
  }
  const cutoffAt = (t: number) => {
    if (t < 2) return 500
    if (t < 4) return 500 + 1700 * ((t - 2) / 2)
    if (t >= 16 && t < 17.5) return 2600 - 1800 * ((t - 16) / 1.5) // closes as the hold builds
    return 3600
  }
  {
    const pattern = [0, 1, 2, 3, 4, 3, 2, 1]
    let step = 0
    for (let t = 0; t < 22 - 1e-9; t += SIX, step++) {
      if (doubt(t) || (t >= 17.5 && t < 18) || t >= 22) continue
      const bar = Math.floor(t / 2)
      const c = CHORDS[bar]
      const idx = pattern[step % pattern.length]
      const note = c[idx % 3] + 12 * Math.floor(idx / 3) + 12
      const vol = t < 2 ? 0.08 : t < 4 ? 0.12 : 0.16
      const pan = 0.5 + 0.28 * Math.sin(step * 0.9)
      pluck(t, hz(note), vol, pan, cutoffAt(t), 0.35)
      // bar 10: the watch answers the phone — every note echoed, opposite side
      if (bar === 9) pluck(t + 0.125, hz(note), vol * 0.55, 1 - pan, cutoffAt(t) * 0.7, 0.2)
    }
  }

  // ---- the doubt: E and F, a semitone that will not settle
  {
    let k = 0
    for (let t = 12; t < 15 - 1e-9; t += 0.25, k++) {
      const f = hz(k % 2 ? 65 : 64)
      const s0 = Math.floor(t * SR)
      for (let j = 0; j < SR * 0.3; j++) {
        const i = s0 + j
        if (i >= N) break
        const tt = j / SR
        const v =
          (Math.sin(2 * Math.PI * f * tt) + 0.25 * Math.sin(2 * Math.PI * 3 * f * tt)) *
          Math.min(1, tt / 0.004) *
          Math.exp(-tt / 0.13) *
          0.09
        const pan = k % 2 ? 0.62 : 0.38
        add(i, v * (1 - pan) * 2, v * pan * 2)
        addVerb(i, v * 0.4)
      }
    }
  }

  // ---- effects
  const noise = (t0: number, len: number, amp: number, fFrom: number, fTo: number, pan: number, rise: boolean) => {
    const s0 = Math.floor(t0 * SR)
    const n = Math.floor(len * SR)
    let lp = 0
    let hpPrev = 0
    let hpOut = 0
    for (let k = 0; k < n; k++) {
      const i = s0 + k
      if (i >= N) break
      const p = k / n
      const a = 1 - Math.exp((-2 * Math.PI * (fFrom + (fTo - fFrom) * p)) / SR)
      lp += a * (rnd() * 2 - 1 - lp)
      hpOut = 0.97 * (hpOut + lp - hpPrev)
      hpPrev = lp
      const env = rise ? p ** 2.2 * (1 - p ** 40) : Math.exp(-p * 6) * Math.min(1, k / 40)
      let v = hpOut * env * amp
      if (muted(i / SR)) v = 0
      add(i, v * (1 - pan) * 2, v * pan * 2)
      addVerb(i, v * 0.3)
    }
  }
  const tick = (t0: number, amp: number, pan: number) => noise(t0, 0.006, amp, 9000, 9000, pan, false)
  const bell = (t0: number, f: number, amp: number) => {
    const s0 = Math.floor(t0 * SR)
    for (let k = 0; k < SR * 1.6; k++) {
      const i = s0 + k
      if (i >= N) break
      const t = k / SR
      const v =
        (Math.sin(2 * Math.PI * f * t) + 0.35 * Math.sin(2 * Math.PI * 2.76 * f * t) * Math.exp(-t / 0.15)) *
        Math.exp(-t / 0.6) *
        Math.min(1, t / 0.002) *
        amp
      add(i, v, v)
      addVerb(i, v * 0.6)
    }
  }
  // bar 1: the HUD types itself
  for (let t = 0.15; t < 0.75; t += 0.03) tick(t + rnd() * 0.01, 0.1, 0.3 + rnd() * 0.4)
  for (let t = 0; t < 2; t += SIX) tick(t, 0.05, 0.7)
  // bar 2: each word lands with a whip, and the dot with a bell
  for (const t of [2, 2.5, 3]) noise(t - 0.08, 0.16, 0.32, 1200, 5000, 0.5, true)
  bell(3.5, hz(81), 0.16)
  // bar 3: the pile — every service clamouring
  for (let t = 4.05; t < 5.6; t += 0.11) tick(t + rnd() * 0.04, 0.08 + rnd() * 0.06, 0.15 + rnd() * 0.7)
  noise(5.62, 0.4, 0.34, 400, 5500, 0.5, true) // into the answer
  // bar 6: a page, then CueSeek's view of it, sliding in
  for (const t of [10, 10.9, 11, 11.9]) noise(t - 0.12, 0.25, 0.16, 900, 4200, t % 2 > 1 ? 0.65 : 0.35, true)
  // bar 7: the hit of something going wrong
  noise(12, 0.5, 0.34, 2000, 300, 0.5, false)
  // bar 8: two taps, then "asked" typing, then the row turns green
  tick(14.47, 0.25, 0.5)
  tick(14.97, 0.25, 0.5)
  for (let t = 15; t < 15.25; t += 0.025) tick(t, 0.09, 0.5)
  bell(15.5, hz(76), 0.08)
  // bar 10: the handoff from phone to watch
  noise(18.72, 0.45, 0.25, 800, 5000, 0.2, true)
  // bar 11: the riser into the end card
  noise(21, 1, 0.4, 300, 6500, 0.5, true)
  // bar 12: the handle lands; the address types itself
  bell(22.5, hz(84), 0.14)
  bell(22.5, hz(79), 0.08)
  for (let t = 23.15; t < 23.6; t += 0.03) tick(t, 0.05, 0.5)

  // ---- the hold: a swell that builds for 1200 ms, then one dry click and nothing
  {
    const notes = [53, 57, 60, 65]
    const ph = new Array(notes.length * 2).fill(0)
    let lp = 0
    const s0 = Math.floor(16.3 * SR)
    const s1 = Math.floor(17.5 * SR)
    for (let i = s0; i < s1; i++) {
      const p = (i - s0) / (s1 - s0)
      const a = 1 - Math.exp((-2 * Math.PI * (200 + 5200 * p * p)) / SR)
      let sum = 0
      for (let n = 0; n < notes.length; n++)
        for (let d = 0; d < 2; d++) {
          const j = n * 2 + d
          ph[j] += (hz(notes[n]) * (d ? 1.003 : 0.997)) / SR
          sum += 2 * (ph[j] % 1) - 1
        }
      lp += a * (sum / 8 - lp)
      const v = lp * 0.42 * p ** 1.6
      add(i, v, v)
      addVerb(i, v * 0.2)
    }
    noise(16.4, 1.1, 0.28, 300, 6000, 0.5, true)
    tick(17.5, 0.4, 0.5)
  }

  // ---- the end: C add9, warm and open, ringing out
  {
    const notes = [48, 55, 60, 62, 64, 67]
    const ph = new Array(notes.length * 2).fill(0)
    let lp = 0
    for (let i = Math.floor(22.5 * SR); i < N; i++) {
      const t = i / SR - 22.5
      let sum = 0
      for (let n = 0; n < notes.length; n++)
        for (let d = 0; d < 2; d++) {
          const j = n * 2 + d
          ph[j] += (hz(notes[n]) * (d ? 1.004 : 0.996)) / SR
          sum += 2 * (ph[j] % 1) - 1
        }
      const a = 1 - Math.exp((-2 * Math.PI * (900 + 1500 * Math.exp(-t / 0.5))) / SR)
      lp += a * (sum / notes.length / 2 - lp)
      const fade = Math.min(1, (DURATION - i / SR) / 0.35)
      const v = lp * 0.38 * Math.min(1, t / 0.06) * Math.exp(-t / 2.2) * fade
      add(i, v * 0.95, v * 1.05)
      addVerb(i, v * 0.5)
    }
  }

  // ---- reverb: combs and allpasses
  {
    const combs = [1557, 1617, 1491, 1422, 1277, 1356]
    const aps = [556, 441, 341]
    for (let side = 0; side < 2; side++) {
      const out = new Float32Array(N)
      const spread = side * 23
      for (const c of combs) {
        const buf = new Float32Array(c + spread)
        let p = 0
        let store = 0
        for (let i = 0; i < N; i++) {
          const y = buf[p]
          store = y * 0.75 + store * 0.25
          buf[p] = VERB[i] + store * 0.82
          p = (p + 1) % buf.length
          out[i] += y / combs.length
        }
      }
      for (const ap of aps) {
        const buf = new Float32Array(ap + spread)
        let p = 0
        for (let i = 0; i < N; i++) {
          const bo = buf[p]
          const y = -out[i] + bo
          buf[p] = out[i] + bo * 0.5
          p = (p + 1) % buf.length
          out[i] = y
        }
      }
      const dst = side ? R : L
      for (let i = 0; i < N; i++) {
        const fade = Math.min(1, (DURATION - i / SR) / 0.35)
        dst[i] += out[i] * 0.9 * fade * (muted(i / SR) ? 0.15 : 1)
      }
    }
  }

  // ---- master: soft clip, normalise, a 4 ms fade-in so the first sample is not a click
  let peak = 0
  for (let i = 0; i < N; i++) {
    L[i] = Math.tanh(L[i] * 1.15)
    R[i] = Math.tanh(R[i] * 1.15)
    peak = Math.max(peak, Math.abs(L[i]), Math.abs(R[i]))
  }
  const g = 0.89 / (peak || 1)
  const buffer = ctx.createBuffer(2, N, SR)
  const outL = buffer.getChannelData(0)
  const outR = buffer.getChannelData(1)
  for (let i = 0; i < N; i++) {
    const fi = Math.min(1, i / (SR * 0.004))
    outL[i] = L[i] * g * fi
    outR[i] = R[i] * g * fi
  }
  return buffer
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
