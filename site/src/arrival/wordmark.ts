/**
 * The wordmark, drawn by a shader so it can move like a liquid.
 *
 * The text is rasterised once into a texture with the page's own font, then a fragment
 * shader does three things to it: a ripple that spreads out from the dot and writes the
 * letters as it passes, a green smear streaming off them back toward the dot that decays, and — once it has settled
 * — a faint ripple wherever the pointer moves. When none of those are happening it draws a
 * still frame and stops, so an idle page costs nothing.
 *
 * Plain WebGL 2, no library: it is one quad and one shader. If WebGL is unavailable the
 * caller keeps the ordinary text instead.
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
uniform vec2 uOrigin;   // where the ripple starts — the dot — in uv
uniform float uFront;   // the ripple's radius, in canvas heights
uniform float uWave;    // how strongly it displaces, 0..1
uniform float uSmear;   // how much trail is left, 0..1
uniform float uReveal;  // 1 while the ripple is writing the word: nothing beyond the front
uniform vec2 uPtr;
uniform float uPtrAmt;
uniform vec3 uHi;
uniform vec3 uGlow;
in vec2 vUv;
out vec4 o;

float ink(vec2 uv) { return texture(uText, uv).a; }

void main() {
  vec2 uv = vUv;
  float aspect = uRes.x / uRes.y;

  // Distance from the dot, in canvas heights, and how far that is from the ripple's front.
  vec2 q = (uv - uOrigin) * vec2(aspect, 1.0);
  float d = length(q);
  vec2 n = q / max(d, 1e-4);
  float x = d - uFront;

  // The liquid: at the front, letters are pushed along the ripple and wobbled across it.
  float band = exp(-x * x * 70.0) * uWave;
  vec2 disp = (n * sin(d * 48.0 - uTime * 12.0) + vec2(-n.y, n.x) * 0.6 * cos(d * 29.0 + uTime * 7.0)) * band * 0.022;
  disp.x /= aspect;

  // The pointer's ripple, at a tenth of the strength.
  vec2 pd = (uv - uPtr) * vec2(aspect, 1.0);
  float pr = length(pd);
  vec2 pdisp = (pd / max(pr, 1e-4)) * sin(pr * 70.0 - uTime * 14.0) * exp(-pr * 7.0) * 0.003 * uPtrAmt;
  pdisp.x /= aspect;

  vec2 suv = uv + disp + pdisp;
  float ahead = mix(1.0, smoothstep(0.03, -0.03, x), uReveal);
  float core = ink(suv) * ahead;

  // The smear: just behind the front, ink streams back toward the dot and down, then decays.
  float behind = max(-x, 0.0);
  float s = uSmear * exp(-behind * 1.6) * smoothstep(0.0, 0.06, behind);
  vec2 dir = normalize(-n + vec2(0.0, 0.45));
  dir.x /= aspect;
  dir *= 0.012 * (0.5 + s * 2.0);
  float trail = 0.0;
  for (int i = 1; i <= 24; i++) {
    float f = float(i);
    trail += ink(suv + dir * f + vec2(0.0, sin(f * 1.3 + uTime * 4.0) * 0.002 * s)) * (1.0 - f / 25.0);
  }
  trail = trail / 12.0 * s;

  // A soft halo on the letters, always.
  float halo = 0.0;
  for (int i = 0; i < 8; i++) {
    float a = float(i) * 0.785398;
    halo += ink(suv + vec2(cos(a) / aspect, sin(a)) * 0.03);
  }
  halo /= 8.0;

  // The ripple itself, faintly visible in the air: a bright ring and a softer echo inside it.
  float ring = exp(-x * x * 900.0) * 0.30 + exp(-pow(d - uFront * 0.62, 2.0) * 1600.0) * 0.14;
  ring *= uWave * smoothstep(0.0, 0.15, d);

  // Nothing in the air may reach the canvas's edge, or its rectangle would show.
  float edge = smoothstep(0.0, 0.14, uv.x) * smoothstep(1.0, 0.86, uv.x)
             * smoothstep(0.0, 0.2, uv.y) * smoothstep(1.0, 0.8, uv.y);
  float g = clamp((halo * 0.24 * ahead + trail * 1.15 + ring) * edge, 0.0, 1.0);
  vec3 lit = uHi * (1.0 + band * 0.6);   // letters flare as the front passes through them
  vec3 col = lit * core + uGlow * g * (1.0 - core);
  float alpha = core + g * (1.0 - core);
  o = vec4(min(col, vec3(alpha)), alpha);
}`

export interface Wordmark {
  /**
   * Sends a ripple out from a point on screen (the dot); resolves when the smear has
   * decayed. With `reveal`, the word does not exist beyond the ripple — the ripple writes it.
   */
  play(from: { x: number; y: number }, reveal?: boolean): Promise<void>
  /** Hidden until the first play, for an arrival that has not started yet. */
  hide(): void
  /** Re-reads the theme's colours and redraws. */
  repaint(): void
  pointer(clientX: number, clientY: number): void
  destroy(): void
}

export function createWordmark(
  canvas: HTMLCanvasElement,
  text: HTMLElement,
): Wordmark | null {
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
    res: u('uRes'), time: u('uTime'), front: u('uFront'), wave: u('uWave'), smear: u('uSmear'), reveal: u('uReveal'),
    origin: u('uOrigin'),
    ptr: u('uPtr'), ptrAmt: u('uPtrAmt'), hi: u('uHi'), glow: u('uGlow'),
  }

  const texture = gl.createTexture()
  gl.bindTexture(gl.TEXTURE_2D, texture)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
  gl.pixelStorei(gl.UNPACK_FLIP_Y_WEBGL, true)

  const state = { front: -1, wave: 0, smear: 0, reveal: 0, origin: [0, 0.5], ptr: [-1, -1], ptrAmt: 0, ptrTarget: 0 }
  let colors = { hi: [1, 1, 1], glow: [0, 1, 0] }
  let raf = 0
  let animating = false
  const t0 = performance.now()

  function rasterise() {
    const dpr = Math.min(devicePixelRatio || 1, 2)
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
    ctx.textBaseline = 'middle'
    ctx.fillStyle = '#fff'
    const m = ctx.measureText(text.textContent ?? '')
    const glyphMiddle = (m.actualBoundingBoxAscent - m.actualBoundingBoxDescent) / 2
    ctx.textBaseline = 'alphabetic'
    ctx.fillText(
      text.textContent ?? '',
      r.left - box.left,
      r.top - box.top + r.height / 2 + glyphMiddle,
    )
    gl!.bindTexture(gl!.TEXTURE_2D, texture)
    gl!.texImage2D(gl!.TEXTURE_2D, 0, gl!.RGBA, gl!.RGBA, gl!.UNSIGNED_BYTE, off)
  }

  function readColors() {
    const s = getComputedStyle(document.documentElement)
    colors = { hi: rgb(s.getPropertyValue('--hi')), glow: rgb(s.getPropertyValue('--glow')) }
  }

  function draw(now: number) {
    gl!.uniform2f(U.res, canvas.width, canvas.height)
    gl!.uniform1f(U.time, (now - t0) / 1000)
    gl!.uniform1f(U.front, state.front)
    gl!.uniform2f(U.origin, state.origin[0], state.origin[1])
    gl!.uniform1f(U.wave, state.wave)
    gl!.uniform1f(U.smear, state.smear)
    gl!.uniform1f(U.reveal, state.reveal)
    gl!.uniform2f(U.ptr, state.ptr[0], state.ptr[1])
    gl!.uniform1f(U.ptrAmt, state.ptrAmt)
    gl!.uniform3fv(U.hi, colors.hi)
    gl!.uniform3fv(U.glow, colors.glow)
    gl!.clearColor(0, 0, 0, 0)
    gl!.clear(gl!.COLOR_BUFFER_BIT)
    gl!.drawArrays(gl!.TRIANGLE_STRIP, 0, 4)
  }

  /** Runs frames only while something is moving. */
  function loop(now: number) {
    state.ptrAmt += (state.ptrTarget - state.ptrAmt) * 0.08
    state.ptrTarget *= 0.97
    draw(now)
    const moving = animating || state.ptrAmt > 0.01 || state.ptrTarget > 0.01
    raf = moving ? requestAnimationFrame(loop) : 0
    if (!moving) {
      state.ptrAmt = 0
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
      state.reveal = 1
      state.front = -1
      if (!raf) draw(performance.now())
    },
    play(from, reveal = false) {
      return new Promise((resolve) => {
        const box = canvas.getBoundingClientRect()
        state.origin = [(from.x - box.left) / box.width, 1 - (from.y - box.top) / box.height]
        // Far enough to clear the farthest corner of the canvas.
        const aspect = box.width / box.height
        const reach = Math.hypot(Math.max(state.origin[0], 1 - state.origin[0]) * aspect, 1) + 0.1
        animating = true
        state.reveal = reveal ? 1 : 0
        const start = performance.now()
        const SPREAD = 1500 // ms for the ripple to cross everything
        const DECAY = 1200 // ms for the smear to fade after it has
        const tick = (now: number) => {
          const t = now - start
          const k = Math.min(t / SPREAD, 1)
          state.front = reach * (1 - (1 - k) ** 2.4)
          state.wave = Math.min(1, k * 6) * (1 - Math.max(0, (t - SPREAD * 0.7) / (DECAY * 0.8))) ** 2
          state.wave = Math.max(0, state.wave)
          const fade = Math.max(0, (t - SPREAD * 0.55) / DECAY)
          state.smear = Math.min(1, k * 4) * (1 - Math.min(fade, 1)) ** 2
          if (t < SPREAD + DECAY) {
            requestAnimationFrame(tick)
          } else {
            Object.assign(state, { front: -1, wave: 0, smear: 0, reveal: 0 })
            animating = false
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
      state.ptr = [(x - box.left) / box.width, 1 - (y - box.top) / box.height]
      state.ptrTarget = 1
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
    const s = gl.createShader(type)!
    gl.shaderSource(s, src)
    gl.compileShader(s)
    if (!gl.getShaderParameter(s, gl.COMPILE_STATUS)) {
      console.warn('wordmark shader:', gl.getShaderInfoLog(s))
      return null
    }
    return s
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

