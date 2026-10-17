import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

const repo = (path: string) => fileURLToPath(new URL(`../${path}`, import.meta.url))

// The headers Vercel serves, applied to `vite preview` too, so a request the policy would
// block in production is blocked on the laptop first. One copy of the policy, in
// vercel.json; this reads it rather than repeating it.
const vercel = JSON.parse(readFileSync(new URL('./vercel.json', import.meta.url), 'utf8'))
const headers: Record<string, string> = Object.fromEntries(
  vercel.headers[0].headers.map((h: { key: string; value: string }) => [h.key, h.value]),
)

export default defineConfig({
  plugins: [react()],
  resolve: {
    // The mark and the fonts are read from where the apps read them, not copied. Same rule
    // as docs/brand/icon.py: the only thing that keeps several renderings identical is that
    // none of them is a hand-made copy.
    alias: {
      '@brand': repo('docs/brand'),
      '@fonts': repo('clients/android/core/design/src/main/res/font'),
    },
  },
  server: { fs: { allow: ['..'] } },
  preview: { headers },
})
