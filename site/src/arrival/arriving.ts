const SEEN = 'cueseek.arrived'

/**
 * While the site is being built, the entrance plays on every load so it can be judged on
 * every refresh. **Turn this on before launch (M6.15)**: then it plays once per visit, and a
 * reload or a return from another page lands on the settled hero.
 */
const ONCE_PER_VISIT = false

/**
 * Whether this page load gets the entrance. Never with reduced motion.
 *
 * Decided before React renders (main.tsx reads it), because the nav and the copy must start
 * hidden on the very first frame or they would flash in and then vanish.
 */
export const arriving: boolean = (() => {
  if (matchMedia('(prefers-reduced-motion: reduce)').matches) return false
  if (!ONCE_PER_VISIT) return true
  try {
    if (sessionStorage.getItem(SEEN)) return false
    sessionStorage.setItem(SEEN, '1')
  } catch {
    // Without storage it plays every time, which is a smaller fault than never.
  }
  return true
})()
