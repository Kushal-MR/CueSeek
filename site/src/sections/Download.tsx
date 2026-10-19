import { useEffect, useRef, useState } from 'react'
import { useCue } from '../traveller/useCue.ts'
import { LAND_EVENT, arrivedGoing, dotTo } from '../traveller/traveller.ts'
import './Download.css'

/** The release the page offers. One place to change when the next one ships. */
const VERSION = '0.1.4'
const REPO = 'https://github.com/Kushal-MR/CueSeek'
const file = (name: string) => `${REPO}/releases/download/v${VERSION}/${name}`

/** Every line here is from README.md "Installing" and docs/install.md. */
const STEPS = [
  {
    where: 'on your server',
    what: 'The agent',
    meta: `Linux · amd64 · v${VERSION} · 9.7 MB`,
    says: 'One program, a systemd unit, a polkit rule and a config file, with an installer. It runs as an ordinary user and manages nothing until you add your services.',
    needs: 'Needs systemd, polkit 0.106 or later, and Tailscale, WireGuard or a LAN to your phone.',
    file: `cueseek-agent_v${VERSION}_linux_amd64.tar.gz`,
    guide: { label: 'Install guide', href: `${REPO}/blob/main/docs/install.md` },
  },
  {
    where: 'on your phone',
    what: 'Android app',
    meta: `cueseek_${VERSION}.apk · signed · 10.4 MB`,
    says: 'Install the APK, then pair: type your server’s address and the single-use code from `sudo -u cueseek cueseekd pair` — there is no QR. Not on the Play Store.',
    needs: 'Needs Android 8.0 or later.',
    file: `cueseek_${VERSION}.apk`,
    guide: { label: 'Pairing', href: `${REPO}/blob/main/docs/pairing.md` },
  },
  {
    where: 'optional',
    what: 'Wear OS app',
    meta: `cueseek-wear_${VERSION}.apk · same key · 37.2 MB`,
    says: 'Installs over ADB — a computer and a few commands: watches can’t install an APK on their own. Once paired, your phone hands it the server’s address.',
    needs: 'Needs Wear OS 3 or later.',
    file: `cueseek-wear_${VERSION}.apk`,
    guide: { label: 'How to install it', href: `${REPO}/blob/main/docs/install.md#the-watch-optional` },
  },
]

const CHECKS = [
  { says: 'The checksums match what was published', cmd: 'sha256sum -c SHA256SUMS' },
  { says: 'The APKs, the same way', cmd: 'sha256sum -c SHA256SUMS-apk' },
  { says: 'It was built by this repository’s own workflow', cmd: `gh attestation verify cueseek-agent_*_linux_amd64.tar.gz --repo Kushal-MR/CueSeek` },
]

/**
 * Section 08. Three things to download, how to check them, and the one awkward truth — the
 * watch app installs over ADB — said plainly.
 */
export function Download() {
  const [ref, seen] = useCue<HTMLElement>('download', 0.25)
  const list = useRef<HTMLOListElement>(null)
  const [copied, setCopied] = useState<number | null>(null)

  // The dot's job here: it walks the three steps in order, each one answering as it lands,
  // and rests on the last — the way you would read a checklist.
  useEffect(() => {
    const step = (i: number) => list.current?.querySelectorAll('.dl-n')[i] ?? null
    let order: number[] = []
    let next = -1
    let timer = 0
    const onLand = (e: Event) => {
      if ((e as CustomEvent<string>).detail !== 'download') return
      order = arrivedGoing === 'down' ? [0, 1, 2] : [2, 1, 0]
      next = 0
      dotTo(step(order[0]), { scale: 0.42, dur: 380 })
    }
    const onArrive = (e: Event) => {
      const el = (e as CustomEvent<Element>).detail
      if (next < 0 || el !== step(order[next])) return
      const card = (el as Element).closest('.dl-step')
      if (card) {
        card.classList.remove('tapped')
        void (card as HTMLElement).offsetWidth
        card.classList.add('tapped')
      }
      next += 1
      if (next < order.length) timer = window.setTimeout(() => dotTo(step(order[next]), { scale: 0.42, dur: 450 }), 380)
      else next = -1
    }
    addEventListener(LAND_EVENT, onLand)
    addEventListener('cueseek:dot-arrived', onArrive)
    return () => {
      removeEventListener(LAND_EVENT, onLand)
      removeEventListener('cueseek:dot-arrived', onArrive)
      clearTimeout(timer)
    }
  }, [])

  const copy = async (i: number, text: string) => {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(i)
      setTimeout(() => setCopied((c) => (c === i ? null : c)), 1600)
    } catch {
      // No clipboard (an old browser, or permission refused): the command is there to select.
    }
  }

  return (
    <section ref={ref} id="download" className="dl" data-in={seen || undefined} aria-labelledby="download-title">
      <span className="hud">Section 08 — Download</span>
      <h2 id="download-title">Install it.</h2>
      <p className="dl-sub">
        Three pieces, all from the{' '}
        <a href={`${REPO}/releases/tag/v${VERSION}`}>v{VERSION} release</a>, all open source.
      </p>

      <ol ref={list} className="dl-steps" data-station="download">
        {STEPS.map((s, i) => (
          <li key={s.file} className="dl-step" style={{ '--i': i } as React.CSSProperties}>
            <div className="dl-head">
              <span className="dl-n">{i + 1}</span>
              <span className="dl-where">{s.where}</span>
            </div>
            <h3>{s.what}</h3>
            <p className="dl-meta">{s.meta}</p>
            <p className="dl-says">
              {s.says.split(/(`[^`]+`)/).map((part, k) => (part.startsWith('`') ? <code key={k}>{part.slice(1, -1)}</code> : part))}
            </p>
            <p className="dl-needs">{s.needs}</p>
            <div className="dl-actions">
              <a className="dl-get" href={file(s.file)} download>
                Download
              </a>
              <a className="dl-guide" href={s.guide.href}>
                {s.guide.label} →
              </a>
            </div>
          </li>
        ))}
      </ol>

      <div className="dl-check">
        <h3>Check what you downloaded</h3>
        <ul>
          {CHECKS.map((c, i) => (
            <li key={c.cmd}>
              <span className="dl-check-says">{c.says}</span>
              <code>{c.cmd}</code>
              <button type="button" onClick={() => copy(i, c.cmd)} aria-label={`Copy: ${c.cmd}`}>
                {copied === i ? 'copied' : 'copy'}
              </button>
            </li>
          ))}
        </ul>
        <p className="dl-note">
          The attestation needs <code>gh</code> 2.49 or later — Ubuntu 24.04 ships 2.45, so run it from a
          machine with a newer one rather than taking the file on trust.
        </p>
      </div>
    </section>
  )
}
