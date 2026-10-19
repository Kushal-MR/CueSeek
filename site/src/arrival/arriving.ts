const SEEN = 'cueseek.arrived'

/**
 * The entrance plays once per visit: a reload, or a return from another page, lands on the
 * settled hero. (While the site was being built it played on every load, to be judged on
 * every refresh; turned on at launch, M6.15.)
 */
const ONCE_PER_VISIT = true

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
