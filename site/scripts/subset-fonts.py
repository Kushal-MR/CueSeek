"""The site's fonts, cut down from the apps' own (M6.13).

The Android design module carries the full IBM Plex files, 971 glyphs each — Cyrillic,
Greek and more. This page is English and uses about a quarter of that. Subsetting to what it
uses, as WOFF, takes the four files from 670 KB to about 90 KB; nothing visible changes.

The apps' files stay the single source: these are derived from them, never edited by hand.

    python scripts/subset-fonts.py          # regenerate site/src/assets/fonts
    python scripts/subset-fonts.py --check  # CI: fail if they no longer match their source
"""

import io
import sys
from pathlib import Path

from fontTools import subset
from fontTools.ttLib import TTFont

SITE = Path(__file__).resolve().parent.parent
SOURCE = SITE.parent / 'clients/android/core/design/src/main/res/font'
OUT = SITE / 'src/assets/fonts'
FONTS = ['plex_sans_regular', 'plex_sans_medium', 'plex_mono_regular', 'plex_mono_medium']

# Latin, Latin-1, general punctuation, arrows, and the handful of symbols the page draws.
UNICODES = (
    'U+0020-007E,U+00A0-00FF,U+0131,U+0152-0153,U+02C6,U+02DA,U+02DC,U+2010-2027,U+2030-203A,'
    'U+2044,U+20AC,U+2122,U+2190-2195,U+21BA,U+2212,U+2215,U+2248,U+2260,U+2264-2265,U+22EE,'
    'U+23FB,U+25B6,U+25CF,U+2713,U+2715,U+275A'
)


def build(name: str) -> bytes:
    opts = subset.Options()
    opts.flavor = 'woff'
    opts.layout_features = ['*']  # keep kerning and ligatures
    opts.name_IDs = ['*']  # keep the licence and copyright records
    opts.notdef_outline = True
    font = TTFont(SOURCE / f'{name}.ttf', recalcTimestamp=False)  # same bytes every run
    s = subset.Subsetter(opts)
    s.populate(unicodes=subset.parse_unicodes(UNICODES))
    s.subset(font)
    font.flavor = 'woff'
    buf = io.BytesIO()
    font.save(buf)
    return buf.getvalue()


def tables(woff: bytes) -> dict:
    font = TTFont(io.BytesIO(woff), lazy=True)
    return {tag: font.reader[tag] for tag in font.reader.keys()}


def main() -> int:
    check = '--check' in sys.argv
    OUT.mkdir(parents=True, exist_ok=True)
    stale = []
    for name in FONTS:
        data = build(name)
        target = OUT / f'{name}.woff'
        if check:
            # Compare the fonts, not the files: WOFF is zlib inside, and another platform's
            # zlib can compress identical tables to different bytes.
            if not target.exists() or tables(target.read_bytes()) != tables(data):
                stale.append(target.name)
        else:
            target.write_bytes(data)
            print(f'{target.relative_to(SITE)}  {len(data) // 1024} KB')
    if stale:
        print('Out of date with the apps\' fonts — run python scripts/subset-fonts.py:', ', '.join(stale))
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
