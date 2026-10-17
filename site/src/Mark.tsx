import mark from '@brand/cueseek-mark.svg?raw'

/**
 * The Swell, read from the file docs/brand/icon.py generates — never redrawn here. It fills
 * with `currentColor`, so it takes the colour of whatever it sits in.
 */
export function Mark({ size, className }: { size: number; className?: string }) {
  return (
    <span
      className={className}
      style={{ display: 'inline-block', width: size, height: size * (56 / 60), lineHeight: 0 }}
      aria-hidden="true"
      // A file from this repository, inlined at build time; not user content.
      dangerouslySetInnerHTML={{ __html: mark.replace('<svg ', '<svg width="100%" height="100%" ') }}
    />
  )
}
