/**
 * The wordmark, drawn by a shader so it can move like a liquid.
 *
 * The text is rasterised once into a texture with the page's own font, then a fragment
 * shader does three things to it: a wave that rolls through the letters left to right, a
 * green smear that trails down-right behind the wave and decays, and — once it has settled
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
uniform float uFront;   // the wave's position across the canvas, in uv.x
uniform float uWave;    // how strongly the wave displaces, 0..1
uniform float uSmear;   // how much trail is left, 0..1
uniform float uReveal;  // 1 while the wave is writing the word: nothing ahead of the front
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

  // The wave: a band around the front, displacing the letters like water.
  float dx = uv.x - uFront;
  float band = exp(-dx * dx * 60.0) * uWave;
  vec2 disp = vec2(sin(uv.y * 34.0 + uTime * 10.0), cos(uv.x * 26.0 - uTime * 8.0)) * band * 0.016;

  // The pointer's ripple, at a tenth of the wave's strength.
  vec2 pd = (uv - uPtr) * vec2(aspect, 1.0);
  float pr = length(pd);
  disp += (pd / max(pr, 1e-4)) * sin(pr * 80.0 - uTime * 14.0) * exp(-pr * 10.0) * 0.003 * uPtrAmt;

  vec2 suv = uv + disp;
  float ahead = mix(1.0, smoothstep(0.04, -0.04, uv.x - uFront), uReveal);
  float core = ink(suv) * ahead;

  // The smear: behind the front, ink is dragged down and to the right. Sampling up-left of
  // this pixel finds the letters that would have been dragged onto it.
  float behind = clamp(uFront - uv.x + 0.08, 0.0, 1.0);
  float s = uSmear * exp(-behind * 2.2) * smoothstep(0.0, 0.08, behind);
  vec2 dir = vec2(-1.0 / aspect, 0.55) * 0.010 * (0.4 + s * 1.6);
  float trail = 0.0;
  for (int i = 1; i <= 22; i++) {
    float f = float(i);
    trail += ink(suv + dir * f + vec2(sin(f * 1.7 + uTime * 3.0), 0.0) * 0.0015 * s) * (1.0 - f / 23.0);
  }
  trail = trail / 11.0 * s;

  // A soft halo, always — the same glow the mark carries.
  float halo = 0.0;
  for (int i = 0; i < 8; i++) {
    float a = float(i) * 0.785398;
    halo += ink(suv + vec2(cos(a) / aspect, sin(a)) * 0.022);
  }
  float g = clamp(halo / 8.0 * 0.22 * ahead + trail * 0.9, 0.0, 1.0);

  vec3 col = uHi * core + uGlow * g * (1.0 - core);
  float alpha = core + g * (1.0 - core);
  o = vec4(col, alpha);
}`

export interface Wordmark {
  /**
   * Rolls the wave through once; resolves when the smear has decayed. With `reveal`, the
   * word does not exist ahead of the wave — the wave writes it.
   */
  play(reveal?: boolean): Promise<void>
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
    ptr: u('uPtr'), ptrAmt: u('uPtrAmt'), hi: u('uHi'), glow: u('uGlow'),
  }

  const texture = gl.createTexture()
  gl.bindTexture(gl.TEXTURE_2D, texture)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
  gl.pixelStorei(gl.UNPACK_FLIP_Y_WEBGL, true)

  const state = { front: -0.3, wave: 0, smear: 0, reveal: 0, ptr: [-1, -1], ptrAmt: 0, ptrTarget: 0 }
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
      state.front = -0.3
      if (!raf) draw(performance.now())
    },
    play(reveal = false) {
      return new Promise((resolve) => {
        animating = true
        state.reveal = reveal ? 1 : 0
        const start = performance.now()
        const WAVE = 1300 // ms for the front to cross
        const DECAY = 1100 // ms for the smear to fade after it has
        const tick = (now: number) => {
          const t = now - start
          const k = Math.min(t / WAVE, 1)
          state.front = -0.25 + 1.6 * easeInOut(k)
          state.wave = Math.sin(Math.PI * Math.min(k * 1.1, 1))
          const fade = Math.max(0, (t - WAVE * 0.6) / DECAY)
          state.smear = Math.min(1, k * 3) * (1 - Math.min(fade, 1)) ** 2
          if (t < WAVE + DECAY) {
            requestAnimationFrame(tick)
          } else {
            Object.assign(state, { front: -0.3, wave: 0, smear: 0, reveal: 0 })
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

const easeInOut = (k: number) => (k < 0.5 ? 2 * k * k : 1 - (-2 * k + 2) ** 2 / 2)
