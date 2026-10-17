const SEEN = 'cueseek.arrived'

/**
 * Whether this page load gets the entrance. Once per visit — a reload or a return from
 * another page lands on the settled hero — and never with reduced motion.
 *
 * Decided before React renders (main.tsx reads it), because the nav and the copy must start
 * hidden on the very first frame or they would flash in and then vanish.
 */
export const arriving: boolean = (() => {
  if (matchMedia('(prefers-reduced-motion: reduce)').matches) return false
  try {
    if (sessionStorage.getItem(SEEN)) return false
    sessionStorage.setItem(SEEN, '1')
  } catch {
    // Without storage it plays every time, which is a smaller fault than never.
  }
  return true
})()
