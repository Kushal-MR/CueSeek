/**
 * The excerpts section 06 quotes, taken from the records at build time (the `whyQuotes` plugin in
 * vite.config.ts). The page ships the six excerpts, not the 57 KB of Markdown they come
 * from — and they are still the records' own words, re-read on every build.
 */
import { readFileSync } from 'node:fs'

/** The paragraphs of a record's "## Decision" section — quoted, never paraphrased. */
function decision(md: string) {
  const from = md.indexOf('## Decision')
  const body = md.slice(md.indexOf('\n', from) + 1)
  const end = body.search(/\n## /)
  return paragraphs(end < 0 ? body : body.slice(0, end))
}

function paragraphs(text: string) {
  return text
    .split(/\n\s*\n|\n(?=- )/) // paragraphs, and each list item on its own
    .map((p) => p.replace(/\s*\n\s*/g, ' ').trim())
    .filter(Boolean)
}

/** The wrist's two findings, from the M5 verification record. */
function wristFindings(md: string) {
  const from = md.indexOf('**The wrist found two defects')
  return paragraphs(md.slice(from, md.indexOf('That fix had a trap', from))).filter((p) => /^\*\*\d\./.test(p))
}

/** In the order of section 06's notes. Fails the build if a record no longer has its excerpt. */
export function quotes(docs: string): { quotes: string[][]; files: string[] } {
  const files: string[] = []
  const read = (path: string) => {
    files.push(`${docs}/${path}`)
    return readFileSync(`${docs}/${path}`, 'utf8')
  }
  const all = [
    decision(read('adr/0001-vpn-only-remote-access.md')).slice(0, 1),
    decision(read('adr/0002-host-privilege-dbus-polkit.md')).slice(0, 2),
    decision(read('adr/0008-multi-host-and-computed-health.md')).slice(1, 2),
    decision(read('adr/0004-contract-openapi-sse.md')).filter((p) => p.startsWith('- Writes')),
    paragraphs(read('adr/0005-capability-based-adapters.md')).filter((p) => p.startsWith('- **Actions must be data**')),
    wristFindings(read('m5-verification.md')).map((p) => p.split(/(?<=\.)\s/).slice(0, 2).join(' ')),
  ]
  all.forEach((q, i) => {
    if (!q.length || q.some((p) => !p)) throw new Error(`section 06: note ${i + 1}'s excerpt is no longer in its record`)
  })
  return { quotes: all, files }
}
