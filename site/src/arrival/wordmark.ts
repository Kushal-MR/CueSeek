import { HANDLE, LONG, TAIL } from './geometry.ts'

/**
 * The lockup — the Swell and the word — drawn by a shader so it can behave like lit glass
 * and liquid light (the Sixfold reference).
 *
 * The mark and the word are rasterised once into a texture (word in red, mark in green, so
 * the shader can tell them apart), with mipmaps: sampling a higher mip is a free blur, which
 * is what makes the light soft instead of streaky. The shader then has three moods:
 *
 * - **glass** — letters and icon go dark inside with neon rims, their edges rippling;
 * - **ribbons** — the shapes swept out along curved, slowly folding paths into long silky
 *   ribbons of light, softening as they go: down and to the left off everything, and up
 *   and to the right off the end of the word;
 * - **the pointer** — the path it actually took turns the letters it crossed to glass and
 *   stretches them the way it was moving, then fades from the oldest end.
 *
 * At rest it draws the plain word and stops, so an idle page costs nothing. The crisp SVG
 * mark sits on top except while the mark itself is glass. Plain WebGL 2: one quad, one shader.
 */

/** How many points of the pointer's recent path the shader sees. */
const TRAIL = 10

const VERT = `#version 300 es
in vec2 p;
out vec2 vUv;
void main() { vUv = p * 0.5 + 0.5; gl_Position = vec4(p, 0.0, 1.0); }`

const FRAG = `#version 300 es
precision highp float;
uniform sampler2D uTex;
uniform vec2 uRes;
uniform float uTime;
uniform vec2 uOrigin;   // the dot, uv — where the writing ripple starts
uniform float uFront;   // the ripple's radius, canvas heights
uniform float uWave;    // ripple strength
uniform float uReveal;  // 1 while the ripple is writing: nothing beyond its front
uniform float uRibbon;  // how far the ribbons reach, 0..1
uniform float uGlass;   // 0 solid letters .. 1 dark glass with neon rims
uniform float uMarkVis; // whether the shader draws the mark (the SVG hides meanwhile)
// The pointer's recent path, newest first: position (uv) and direction of travel there
// (unit, aspect-corrected), and how much life that point has left, 0..1.
uniform vec4 uTrail[10];
uniform float uTrailLife[10];
uniform vec3 uHi;
uniform vec3 uGlow;
in vec2 vUv;
out vec4 o;

// Coverage of the lockup at a uv, blurred by mip level.
float ink(vec2 uv, float lod) { return textureLod(uTex, uv, lod).a; }
float isMark(vec2 uv) { vec4 t = textureLod(uTex, uv, 0.0); return t.a > 0.0 ? step(t.r, t.g) : 0.0; }

// A ribbon: the shapes swept back along a curved path that bends and slowly folds, sampled
// from blurred mips so it reads as smoke or silk rather than copies of letters.
vec2 ribbon(vec2 uv, vec2 dir, float len, float bend, float foldAmt, float phase, float rightOnly) {
  float aspect = uRes.x / uRes.y;
  vec2 perp = vec2(-dir.y, dir.x);
  float total = 0.0;
  float close = 0.0;
  for (int i = 1; i <= 40; i++) {
    float f = float(i) / 40.0;
    float curve = sin(f * 2.4 + phase + uTime * 0.9) * bend;
    float fold = sin(f * 7.0 + dot(uv, perp) * 9.0 + uTime * 1.6) * foldAmt;
    vec2 off = (dir * f + perp * (curve + fold) * f) * len;
    off.x /= aspect;
    vec2 src = uv - off;
    // Already soft at the first step (each sample must overlap the next, or the sweep
    // breaks into dots), softer still with distance.
    float k = ink(src, 2.4 + f * 3.4);
    k *= mix(1.0, smoothstep(0.45, 0.75, src.x), rightOnly);
    float w = (1.0 - f) * (1.0 - f * 0.5);
    total += k * w;
    close += k * w * (1.0 - f);
  }
  return vec2(total, close) / 9.0;
}

void main() {
  vec2 uv = vUv;
  float aspect = uRes.x / uRes.y;

  // ---- The writing ripple, out from the dot.
  vec2 q = (uv - uOrigin) * vec2(aspect, 1.0);
  float d = length(q);
  vec2 n = q / max(d, 1e-4);
  float x = d - uFront;
  float band = exp(-x * x * 70.0) * uWave;
  vec2 disp = (n * sin(d * 40.0 - uTime * 10.0) + vec2(-n.y, n.x) * 0.5 * cos(d * 23.0 + uTime * 6.0)) * band * 0.02;

  // ---- The pointer: along the path it actually took. Each point of the path affects only
  // what is close to it, in the direction the pointer was travelling there, fading as it ages.
  float local = 0.0;
  vec2 along = vec2(1.0, 0.0);
  for (int i = 0; i < 10; i++) {
    vec2 dq = (uv - uTrail[i].xy) * vec2(aspect, 1.0);
    float g = uTrailLife[i] * exp(-dot(dq, dq) * 18.0);
    if (g > local) { local = g; along = uTrail[i].zw; }
  }

  // Glass is wherever the arrival says, or wherever the pointer is.
  float glass = clamp(uGlass + local * 1.3, 0.0, 1.0);

  // Wavy, liquid edges while glass: a slow, smooth wobble — never per-frame noise.
  disp += vec2(sin(uv.y * 26.0 + uTime * 2.2), cos(uv.x * 19.0 - uTime * 1.8)) * 0.0035 * glass;
  // Letters are pulled along with the pointer, the way it is moving.
  disp -= along * local * 0.014;
  disp.x /= aspect;
  vec2 suv = uv + disp;

  float ahead = mix(1.0, smoothstep(0.03, -0.03, x), uReveal);
  float a = ink(suv, 0.0) * ahead;
  float mark = isMark(suv);
  a *= mix(1.0, uMarkVis, mark); // the SVG draws the mark unless the shader has it

  // Glass: dark inside, a bright rim where the shape's blurred self thins out.
  float body = ink(suv, 2.2);
  float rim = a * smoothstep(0.92, 0.55, body);
  float fillA = a * mix(1.0, 0.22, glass);
  float rimA = rim * glass;

  // ---- Ribbons.
  vec2 light = vec2(0.0);
  if (uRibbon > 0.001) {
    // Short ribbons fade too, or light would bunch up on the letters as they retract.
    float reach = smoothstep(0.0, 0.35, uRibbon);
    light += ribbon(suv, normalize(vec2(-1.0, -0.72)), 0.95 * uRibbon, 0.32, 0.05, 0.0, 0.0) * ahead * reach;
    light += ribbon(suv, normalize(vec2(1.0, 0.55)), 0.45 * uRibbon, -0.25, 0.05, 1.7, 1.0) * ahead * 0.8 * reach;
  }
  if (local > 0.01) {
    // Straight along the pointer's own direction — no wander of its own.
    light += ribbon(suv, along, 0.34 * local, 0.0, 0.0, 0.0, 0.0) * min(1.0, local * 1.8);
  }

  // A soft halo, always; the ripple faintly visible in the air.
  float halo = ink(suv, 4.0) * 0.32 * ahead;
  float ring = (exp(-x * x * 900.0) * 0.28 + exp(-pow(d - uFront * 0.62, 2.0) * 1600.0) * 0.12)
             * uWave * smoothstep(0.0, 0.15, d);

  // Nothing in the air may reach the canvas edge, or its rectangle would show.
  float edge = smoothstep(0.0, 0.1, uv.x) * (1.0 - smoothstep(0.9, 1.0, uv.x))
             * smoothstep(0.0, 0.12, uv.y) * (1.0 - smoothstep(0.88, 1.0, uv.y));

  // Tone-mapped, so dense light glows instead of clipping into a flat sheet.
  float silk = (1.0 - exp(-light.x * 2.6)) * edge;
  float silkNear = (1.0 - exp(-light.y * 3.0)) * edge;
  float air = clamp(halo + ring, 0.0, 1.0) * edge;

  // Compose, back to front: air and ribbons, then the glass body, then the rims.
  // Neon: the glow pushed brighter and more saturated than the page's own palette.
  vec3 neon = clamp(mix(uGlow, uHi, 0.4) * 1.3, 0.0, 1.0);
  vec3 col = uGlow * air + neon * silk + uHi * silkNear * 0.2;
  float alpha = clamp(air + silk, 0.0, 1.0);
  vec3 bodyCol = mix(uHi, uGlow * 0.35, glass) * (1.0 + band * 0.5);
  col = bodyCol * fillA + col * (1.0 - fillA);
  alpha = fillA + alpha * (1.0 - fillA);
  vec3 rimCol = mix(uHi, vec3(1.0), 0.25);
  col = rimCol * rimA + col * (1.0 - rimA);
  alpha = rimA + alpha * (1.0 - rimA);
  o = vec4(min(col, vec3(alpha)), alpha);

}`

export interface Wordmark {
  /** Nothing drawn until the ripple writes it. */
  hide(): void
  /**
   * The arrival: a ripple from the dot writes the word as glass, ribbons of light sweep out
   * and fold, are drawn back in, and the glass settles into the solid word. `markGlass` is
   * called with true when the shader takes over drawing the mark and false when it hands it
   * back, so the caller can hide and show the SVG.
   */
  play(from: { x: number; y: number }, markGlass: (on: boolean) => void): Promise<void>
  repaint(): void
  pointer(clientX: number, clientY: number): void
  /** Freezes the effect in a pose and draws it once — for tuning and for stills. */
  pose(values: Partial<Record<'ribbon' | 'glass' | 'markVis' | 'wave' | 'front', number>>): void
  destroy(): void
}

export function createWordmark(
  canvas: HTMLCanvasElement,
  text: HTMLElement,
  markEl: SVGSVGElement,
): Wordmark | null {
  const gl = canvas.getContext('webgl2', { premultipliedAlpha: true, antialias: false })
  if (!gl) return null
  const program = link(gl)
  if (!program) return null
  gl.useProgram(program)

  const quad = gl.createBuffer()
  gl.bindBuffer(gl.ARRAY_BUFFER, quad)
  gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 1, -1, -1, 1, 1, 1]), gl.STATIC_DRAW)
  const pa = gl.getAttribLocation(program, 'p')
  gl.enableVertexAttribArray(pa)
  gl.vertexAttribPointer(pa, 2, gl.FLOAT, false, 0, 0)

  const u = (name: string) => gl.getUniformLocation(program, name)
  const U = {
    res: u('uRes'), time: u('uTime'), origin: u('uOrigin'), front: u('uFront'), wave: u('uWave'),
    reveal: u('uReveal'), ribbon: u('uRibbon'), glass: u('uGlass'), markVis: u('uMarkVis'),
    trail: u('uTrail'), trailLife: u('uTrailLife'), hi: u('uHi'), glow: u('uGlow'),
  }

  const texture = gl.createTexture()
  gl.bindTexture(gl.TEXTURE_2D, texture)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR_MIPMAP_LINEAR)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
  gl.pixelStorei(gl.UNPACK_FLIP_Y_WEBGL, true)

  const s = {
    origin: [0, 0.5], front: -1, wave: 0, reveal: 0, ribbon: 0, glass: 0, markVis: 0,
    // The pointer as reported, and as drawn: the drawn values chase the reported ones each
    // frame, so a jittery mouse still produces one calm gesture.
    raw: [-1, -1], ptr: [-1, -1],
    trail: [] as { x: number; y: number; dx: number; dy: number; life: number }[],
    // Where the path was last sampled — kept apart from the trail, whose points fade away.
    anchor: null as null | { x: number; y: number },
  }
  let colors = { hi: [1, 1, 1], glow: [0, 1, 0] }
  let raf = 0
  let playing = false
  let last = performance.now()
  const t0 = last

  function rasterise() {
    // The light is soft by nature; 1.25x keeps the rims sharp without paying for 2x.
    const dpr = Math.min(devicePixelRatio || 1, 1.25)
    const box = canvas.getBoundingClientRect()
    canvas.width = Math.round(box.width * dpr)
    canvas.height = Math.round(box.height * dpr)
    gl!.viewport(0, 0, canvas.width, canvas.height)

    const off = document.createElement('canvas')
    off.width = canvas.width
    off.height = canvas.height
    const ctx = off.getContext('2d')!
    ctx.scale(dpr, dpr)

    // The word, in red, exactly where the real heading sits.
    const style = getComputedStyle(text)
    const r = text.getBoundingClientRect()
    ctx.font = `${style.fontWeight} ${style.fontSize} ${style.fontFamily}`
    ctx.letterSpacing = style.letterSpacing === 'normal' ? '0px' : style.letterSpacing
    ctx.fillStyle = '#f00'
    const word = text.textContent ?? ''
    const m = ctx.measureText(word)
    const middle = (m.actualBoundingBoxAscent - m.actualBoundingBoxDescent) / 2
    ctx.fillText(word, r.left - box.left, r.top - box.top + r.height / 2 + middle)

    // The mark, in green, exactly where the SVG sits.
    const mr = markEl.getBoundingClientRect()
    const unit = mr.width / 60
    ctx.save()
    ctx.translate(mr.left - box.left - 24 * unit, mr.top - box.top - 26 * unit)
    ctx.scale(unit, unit)
    ctx.fillStyle = '#0f0'
    for (const part of [LONG, TAIL, HANDLE]) ctx.fill(new Path2D(part))
    ctx.restore()

    gl!.bindTexture(gl!.TEXTURE_2D, texture)
    gl!.texImage2D(gl!.TEXTURE_2D, 0, gl!.RGBA, gl!.RGBA, gl!.UNSIGNED_BYTE, off)
    gl!.generateMipmap(gl!.TEXTURE_2D)
  }

  function readColors() {
    const cs = getComputedStyle(document.documentElement)
    colors = { hi: rgb(cs.getPropertyValue('--hi')), glow: rgb(cs.getPropertyValue('--glow')) }
  }

  function draw(now: number) {
    gl!.uniform2f(U.res, canvas.width, canvas.height)
    gl!.uniform1f(U.time, (now - t0) / 1000)
    gl!.uniform2f(U.origin, s.origin[0], s.origin[1])
    gl!.uniform1f(U.front, s.front)
    gl!.uniform1f(U.wave, s.wave)
    gl!.uniform1f(U.reveal, s.reveal)
    gl!.uniform1f(U.ribbon, s.ribbon)
    gl!.uniform1f(U.glass, s.glass)
    gl!.uniform1f(U.markVis, s.markVis)
    const path = new Float32Array(TRAIL * 4)
    const life = new Float32Array(TRAIL)
    s.trail.forEach((t, i) => {
      path.set([t.x, t.y, t.dx, t.dy], i * 4)
      life[i] = t.life
    })
    gl!.uniform4fv(U.trail, path)
    gl!.uniform1fv(U.trailLife, life)
    gl!.uniform3fv(U.hi, colors.hi)
    gl!.uniform3fv(U.glow, colors.glow)
    gl!.clearColor(0, 0, 0, 0)
    gl!.clear(gl!.COLOR_BUFFER_BIT)
    gl!.drawArrays(gl!.TRIANGLE_STRIP, 0, 4)
  }

  /**
   * Every frame while anything moves. The pointer is smoothed here, per frame, rather than
   * per mouse event — events arrive unevenly, frames do not.
   */
  function loop(now: number) {
    // A frame's timestamp can be earlier than the moment the loop was woken, so the first
    // step can come out negative; a negative step drained the energy and stopped the loop.
    const dt = Math.min(Math.max((now - last) / 1000, 1 / 120), 0.05)
    last = now
    const box = canvas.getBoundingClientRect()
    const aspect = box.width / box.height

    if (s.raw[0] >= 0) {
      if (s.ptr[0] < 0) s.ptr = [...s.raw]
      // The drawn pointer chases the reported one: one calm gesture from a jittery mouse.
      const follow = 1 - Math.exp(-dt * 18)
      const nx = s.ptr[0] + (s.raw[0] - s.ptr[0]) * follow
      const ny = s.ptr[1] + (s.raw[1] - s.ptr[1]) * follow
      s.ptr = [nx, ny]
      // A new point on the path whenever it has moved far enough to have a direction.
      if (!s.anchor) s.anchor = { x: nx, y: ny }
      const mx = (nx - s.anchor.x) * aspect
      const my = ny - s.anchor.y
      const moved = Math.hypot(mx, my)
      if (moved > 0.018) {
        const speed = moved / dt
        s.trail.unshift({
          x: nx, y: ny, dx: mx / moved, dy: my / moved,
          life: Math.min(1, 0.35 + speed * 0.12),
        })
        s.trail.length = Math.min(s.trail.length, TRAIL)
        s.anchor = { x: nx, y: ny }
      }
    }
    // Each point fades on its own clock, so the oldest part of the path goes first.
    for (const t of s.trail) t.life *= Math.exp(-dt * 2.6)
    s.trail = s.trail.filter((t) => t.life > 0.01)

    draw(now)
    if (playing || s.trail.length > 0 || Math.hypot(s.raw[0] - s.ptr[0], s.raw[1] - s.ptr[1]) > 0.001) {
      raf = requestAnimationFrame(loop)
    } else {
      raf = 0
    }
  }
  const wake = () => {
    if (!raf) {
      last = performance.now() - 16
      raf = requestAnimationFrame(loop)
    }
  }

  rasterise()
  readColors()
  draw(performance.now())
  const resize = new ResizeObserver(() => {
    rasterise()
    draw(performance.now())
  })
  resize.observe(canvas)

  return {
    hide() {
      s.reveal = 1
      s.front = -1
      if (!raf) draw(performance.now())
    },
    play(from, markGlass) {
      return new Promise((resolve) => {
        const box = canvas.getBoundingClientRect()
        s.origin = [(from.x - box.left) / box.width, 1 - (from.y - box.top) / box.height]
        const aspect = box.width / box.height
        const reach = Math.hypot(Math.max(s.origin[0], 1 - s.origin[0]) * aspect, 1) + 0.1
        playing = true
        s.reveal = 1
        s.glass = 1
        s.markVis = 1
        markGlass(true)
        const start = performance.now()
        const SPREAD = 1400 // the ripple writes the word as glass
        const OUT_AT = 500 // ribbons begin while it is still writing
        const OUT = 1300 // they sweep out…
        const HOLD = 700 // …fold for a moment…
        const BACK = 1500 // …and are drawn back in
        const SOLID = 600 // then the glass fills into the solid word
        const BACK_AT = OUT_AT + OUT + HOLD
        const END = BACK_AT + BACK + SOLID
        let handedBack = false
        const ease = (k: number) => 1 - (1 - k) ** 3
        const tick = (now: number) => {
          const t = now - start
          const k = Math.min(t / SPREAD, 1)
          s.front = reach * ease(k)
          // Clamped before squaring: a negative base squared would bring the ripple back.
          const fadeOut = Math.min(1, Math.max(0, (t - SPREAD * 0.7) / 900))
          s.wave = Math.min(1, k * 6) * (1 - fadeOut) ** 2
          s.ribbon =
            t < OUT_AT ? 0
            : t < OUT_AT + OUT ? ease((t - OUT_AT) / OUT)
            : t < BACK_AT ? 1
            : Math.max(0, 1 - ease(Math.min((t - BACK_AT) / BACK, 1)))
          s.glass = t < BACK_AT + BACK ? 1 : Math.max(0, 1 - (t - BACK_AT - BACK) / SOLID)
          if (!handedBack && s.glass < 0.5) {
            // Back to the crisp SVG for the mark, which the dot lives in. The SVG fades in
            // over the shader's own mark, which stays until the end so there is no gap.
            handedBack = true
            markGlass(false)
          }
          if (t < END) {
            requestAnimationFrame(tick)
          } else {
            Object.assign(s, { front: -1, wave: 0, reveal: 0, ribbon: 0, glass: 0, markVis: 0 })
            playing = false
            resolve()
          }
        }
        requestAnimationFrame(tick)
        wake()
      })
    },
    repaint() {
      readColors()
      if (!raf) draw(performance.now())
    },
    pointer(x, y) {
      const box = canvas.getBoundingClientRect()
      s.raw = [(x - box.left) / box.width, 1 - (y - box.top) / box.height]
      wake()
    },
    pose(values) {
      Object.assign(s, { reveal: 0 }, values)
      draw(performance.now())
    },
    destroy() {
      cancelAnimationFrame(raf)
      resize.disconnect()
      gl.getExtension('WEBGL_lose_context')?.loseContext()
    },
  }
}

function link(gl: WebGL2RenderingContext): WebGLProgram | null {
  const shader = (type: number, src: string) => {
    const sh = gl.createShader(type)!
    gl.shaderSource(sh, src)
    gl.compileShader(sh)
    if (!gl.getShaderParameter(sh, gl.COMPILE_STATUS)) {
      console.warn('wordmark shader:', gl.getShaderInfoLog(sh))
      return null
    }
    return sh
  }
  const v = shader(gl.VERTEX_SHADER, VERT)
  const f = shader(gl.FRAGMENT_SHADER, FRAG)
  if (!v || !f) return null
  const prog = gl.createProgram()!
  gl.attachShader(prog, v)
  gl.attachShader(prog, f)
  gl.linkProgram(prog)
  return gl.getProgramParameter(prog, gl.LINK_STATUS) ? prog : null
}

/** Accepts `#rrggbb` or `rgb(r g b)` — the knob writes the second while blending. */
function rgb(value: string): number[] {
  const v = value.trim()
  const parts = v.startsWith('#')
    ? [1, 3, 5].map((i) => parseInt(v.slice(i, i + 2), 16))
    : (v.match(/[\d.]+/g) ?? ['0', '0', '0']).slice(0, 3).map(Number)
  return parts.map((n) => n / 255)
}
