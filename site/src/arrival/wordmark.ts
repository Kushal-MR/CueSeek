/**
 * The wordmark, drawn by a shader so it can behave like a liquid.
 *
 * The text is rasterised once into a texture with the page's own font. A fragment shader
 * then does three things with it (the Sixfold reference):
 *
 * - **the ripple** — spreads out from the dot, writing the letters as it passes;
 * - **the extrusion** — the whole word smeared out into long glowing streaks, then pulled
 *   back into the letters;
 * - **the pointer** — the letters under a moving pointer are dragged and smeared in the
 *   direction it moves, and flow back when it stops.
 *
 * Frames are drawn only while something is moving, so an idle page costs nothing. Plain
 * WebGL 2, no library: one quad, one shader. Without WebGL the caller keeps the plain text.
 */

const VERT = `#version 300 es
in vec2 p;
out vec2 vUv;
void main() { vUv = p * 0.5 + 0.5; gl_Position = vec4(p, 0.0, 1.0); }`

const FRAG = `#version 300 es
precision highp float;
uniform sampler2D uText;
uniform vec2 uRes;
uniform float uTime;
uniform vec2 uOrigin;    // where the ripple starts — the dot — in uv
uniform float uFront;    // the ripple's radius, in canvas heights
uniform float uWave;     // how strongly the ripple displaces, 0..1
uniform float uReveal;   // 1 while the ripple is writing: nothing beyond its front
uniform float uExtrude;  // how far the whole word is smeared out, 0..1
uniform vec2 uPtr;       // pointer, uv
uniform vec2 uPtrVel;    // pointer velocity, canvas heights per second
uniform float uPtrAmt;   // 0..1, fades when the pointer stops
uniform vec3 uHi;
uniform vec3 uGlow;
in vec2 vUv;
out vec4 o;

float ink(vec2 uv) { return texture(uText, uv).a; }

// Streaks: brightness varies across the direction of a smear, so it reads as many strands
// of light rather than one blur.
float strands(vec2 uv, vec2 dir, float aspect) {
  vec2 perp = vec2(-dir.y, dir.x);
  float x = dot(uv * vec2(aspect, 1.0), perp) * 160.0;
  return 0.45 + 0.55 * (0.5 + 0.5 * sin(x + sin(x * 0.37) * 2.0));
}

// Ink found by looking back along a smear: what would have been dragged onto this pixel.
// Returns (near, far) so the light can run from the letters' colour into the glow.
vec2 smear(vec2 uv, vec2 dir, float len, float aspect) {
  vec2 stp = vec2(dir.x / aspect, dir.y) * len / 32.0;
  float nearSum = 0.0, farSum = 0.0;
  for (int i = 1; i <= 32; i++) {
    float f = float(i) / 32.0;
    float k = ink(uv - stp * float(i)) * pow(1.0 - f, 1.4);
    nearSum += k * (1.0 - f);
    farSum += k * f;
  }
  float s = strands(uv, dir, aspect);
  return vec2(nearSum, farSum) / 9.0 * s;
}

void main() {
  vec2 uv = vUv;
  float aspect = uRes.x / uRes.y;

  // ---- The ripple from the dot.
  vec2 q = (uv - uOrigin) * vec2(aspect, 1.0);
  float d = length(q);
  vec2 n = q / max(d, 1e-4);
  float x = d - uFront;
  float band = exp(-x * x * 70.0) * uWave;
  vec2 disp = (n * sin(d * 48.0 - uTime * 12.0) + vec2(-n.y, n.x) * 0.6 * cos(d * 29.0 + uTime * 7.0)) * band * 0.022;
  disp.x /= aspect;

  // ---- The pointer drags the letters along with it, like a finger through paint.
  vec2 pq = (uv - uPtr) * vec2(aspect, 1.0);
  float pr = length(pq);
  float speed = length(uPtrVel);
  float local = uPtrAmt * exp(-pr * pr * 5.0);
  vec2 drag = uPtrVel * local * 0.02;
  drag.x /= aspect;

  vec2 suv = uv + disp - drag;
  float ahead = mix(1.0, smoothstep(0.03, -0.03, x), uReveal);
  float core = ink(suv) * ahead;

  vec2 light = vec2(0.0);
  // ---- The extrusion: the whole word streams out down and to the right.
  if (uExtrude > 0.001) {
    light += smear(suv, normalize(vec2(1.0, -0.62)), 0.55 * uExtrude, aspect) * 1.3;
  }
  // ---- The pointer's smear, opposite to where it is heading.
  if (local > 0.01 && speed > 0.01) {
    light += smear(suv, -uPtrVel / speed, min(speed * 0.10, 0.32) * local, aspect) * 1.2 * min(1.0, local * 2.0);
  }

  // A soft halo on the letters, always.
  float halo = 0.0;
  for (int i = 0; i < 8; i++) {
    float a = float(i) * 0.785398;
    halo += ink(suv + vec2(cos(a) / aspect, sin(a)) * 0.03);
  }
  halo /= 8.0;

  // The ripple, faintly visible in the air.
  float ring = exp(-x * x * 900.0) * 0.30 + exp(-pow(d - uFront * 0.62, 2.0) * 1600.0) * 0.14;
  ring *= uWave * smoothstep(0.0, 0.15, d);

  // Nothing in the air may reach the canvas edge, or its rectangle would show.
  float edge = smoothstep(0.0, 0.12, uv.x) * smoothstep(1.0, 0.88, uv.x)
             * smoothstep(0.0, 0.18, uv.y) * smoothstep(1.0, 0.82, uv.y);

  float near = clamp(light.x, 0.0, 1.0) * edge;
  float far = clamp(light.y + halo * 0.24 * ahead + ring, 0.0, 1.0) * edge;

  vec3 lit = uHi * (1.0 + band * 0.6);
  vec3 col = lit * core;
  float alpha = core;
  // Near the letters the light is the letters' own colour; further out, the glow.
  col += (uHi * near + uGlow * far) * (1.0 - alpha);
  alpha += clamp(near + far, 0.0, 1.0) * (1.0 - alpha);
  o = vec4(min(col, vec3(alpha)), alpha);
}`

export interface Wordmark {
  /** Hidden until the ripple writes it, for an arrival that has not started yet. */
  hide(): void
  /**
   * Sends a ripple out from a point on screen (the dot) that writes the word, then smears
   * the whole word out and pulls it back in. Resolves when everything has settled.
   */
  play(from: { x: number; y: number }): Promise<void>
  /** Re-reads the theme's colours and redraws. */
  repaint(): void
  pointer(clientX: number, clientY: number): void
  destroy(): void
}

export function createWordmark(canvas: HTMLCanvasElement, text: HTMLElement): Wordmark | null {
  const gl = canvas.getContext('webgl2', { premultipliedAlpha: true, antialias: false })
  if (!gl) return null
  const program = link(gl)
  if (!program) return null
  gl.useProgram(program)

  const quad = gl.createBuffer()
  gl.bindBuffer(gl.ARRAY_BUFFER, quad)
  gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 1, -1, -1, 1, 1, 1]), gl.STATIC_DRAW)
  const p = gl.getAttribLocation(program, 'p')
  gl.enableVertexAttribArray(p)
  gl.vertexAttribPointer(p, 2, gl.FLOAT, false, 0, 0)

  const u = (name: string) => gl.getUniformLocation(program, name)
  const U = {
    res: u('uRes'), time: u('uTime'), origin: u('uOrigin'), front: u('uFront'), wave: u('uWave'),
    reveal: u('uReveal'), extrude: u('uExtrude'), ptr: u('uPtr'), ptrVel: u('uPtrVel'),
    ptrAmt: u('uPtrAmt'), hi: u('uHi'), glow: u('uGlow'),
  }

  const texture = gl.createTexture()
  gl.bindTexture(gl.TEXTURE_2D, texture)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
  gl.pixelStorei(gl.UNPACK_FLIP_Y_WEBGL, true)

  const s = {
    origin: [0, 0.5], front: -1, wave: 0, reveal: 0, extrude: 0,
    ptr: [-1, -1], vel: [0, 0], amt: 0, lastMove: 0,
    last: null as null | { x: number; y: number; t: number },
  }
  let colors = { hi: [1, 1, 1], glow: [0, 1, 0] }
  let raf = 0
  let playing = false
  const t0 = performance.now()

  function rasterise() {
    // The smear loops are the costly part; 1.5x is sharp enough for a glow.
    const dpr = Math.min(devicePixelRatio || 1, 1.5)
    const box = canvas.getBoundingClientRect()
    canvas.width = Math.round(box.width * dpr)
    canvas.height = Math.round(box.height * dpr)
    gl!.viewport(0, 0, canvas.width, canvas.height)

    // Draw the text exactly where the real heading sits inside this canvas.
    const style = getComputedStyle(text)
    const r = text.getBoundingClientRect()
    const off = document.createElement('canvas')
    off.width = canvas.width
    off.height = canvas.height
    const ctx = off.getContext('2d')!
    ctx.scale(dpr, dpr)
    ctx.font = `${style.fontWeight} ${style.fontSize} ${style.fontFamily}`
    ctx.letterSpacing = style.letterSpacing === 'normal' ? '0px' : style.letterSpacing
    ctx.fillStyle = '#fff'
    const word = text.textContent ?? ''
    const m = ctx.measureText(word)
    const middle = (m.actualBoundingBoxAscent - m.actualBoundingBoxDescent) / 2
    ctx.fillText(word, r.left - box.left, r.top - box.top + r.height / 2 + middle)
    gl!.bindTexture(gl!.TEXTURE_2D, texture)
    gl!.texImage2D(gl!.TEXTURE_2D, 0, gl!.RGBA, gl!.RGBA, gl!.UNSIGNED_BYTE, off)
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
    gl!.uniform1f(U.extrude, s.extrude)
    gl!.uniform2f(U.ptr, s.ptr[0], s.ptr[1])
    gl!.uniform2f(U.ptrVel, s.vel[0], s.vel[1])
    gl!.uniform1f(U.ptrAmt, s.amt)
    gl!.uniform3fv(U.hi, colors.hi)
    gl!.uniform3fv(U.glow, colors.glow)
    gl!.clearColor(0, 0, 0, 0)
    gl!.clear(gl!.COLOR_BUFFER_BIT)
    gl!.drawArrays(gl!.TRIANGLE_STRIP, 0, 4)
  }

  /** Runs frames only while something moves. The pointer's smear relaxes on its own. */
  function loop(now: number) {
    if (now - s.lastMove > 60) {
      s.vel = s.vel.map((v) => v * 0.88)
      s.amt *= 0.95
    }
    draw(now)
    if (playing || s.amt > 0.01) {
      raf = requestAnimationFrame(loop)
    } else {
      raf = 0
      s.amt = 0
      s.vel = [0, 0]
      draw(now)
    }
  }
  const wake = () => {
    if (!raf) raf = requestAnimationFrame(loop)
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
    play(from) {
      return new Promise((resolve) => {
        const box = canvas.getBoundingClientRect()
        s.origin = [(from.x - box.left) / box.width, 1 - (from.y - box.top) / box.height]
        const aspect = box.width / box.height
        const reach = Math.hypot(Math.max(s.origin[0], 1 - s.origin[0]) * aspect, 1) + 0.1
        playing = true
        s.reveal = 1
        const start = performance.now()
        const SPREAD = 1500 // the ripple crosses everything
        const EXTRUDE_AT = 750 // while it is still travelling, the written word…
        const OUT = 1000 // …streams out…
        const BACK = 1300 // …and is pulled back in
        const END = EXTRUDE_AT + OUT + BACK
        const tick = (now: number) => {
          const t = now - start
          const k = Math.min(t / SPREAD, 1)
          s.front = reach * (1 - (1 - k) ** 2.4)
          s.wave = Math.max(0, Math.min(1, k * 6) * (1 - Math.max(0, (t - SPREAD * 0.7) / 900)) ** 2)
          const e = t - EXTRUDE_AT
          s.extrude =
            e < 0 ? 0 : e < OUT ? 1 - (1 - e / OUT) ** 3 : Math.max(0, 1 - ((e - OUT) / BACK) ** 0.8)
          if (t < END) {
            requestAnimationFrame(tick)
          } else {
            Object.assign(s, { front: -1, wave: 0, reveal: 0, extrude: 0 })
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
      const now = performance.now()
      const box = canvas.getBoundingClientRect()
      s.ptr = [(x - box.left) / box.width, 1 - (y - box.top) / box.height]
      if (s.last && now - s.last.t < 200) {
        const dt = Math.max((now - s.last.t) / 1000, 1 / 240)
        // Canvas heights per second, smoothed so a jittery mouse still gives one direction.
        const vx = (x - s.last.x) / box.height / dt
        const vy = -(y - s.last.y) / box.height / dt
        s.vel = [s.vel[0] * 0.7 + vx * 0.3, s.vel[1] * 0.7 + vy * 0.3]
        s.amt = Math.min(1, s.amt + 0.25)
      }
      s.last = { x, y, t: now }
      s.lastMove = now
      wake()
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
