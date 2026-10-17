# site/ — cueseek's website

The real website, replacing the placeholder at `docs/index.html` (M6 —
[plan](../docs/m6-plan.md)). React + TypeScript, built by Vite to static files, deployed by
Vercel with a preview link on every pull request.

```bash
npm ci
npm run dev       # http://localhost:5173
npm run build     # typecheck, then dist/
npm run preview   # dist/, served with the production headers
```

## Loads nothing but itself

The placeholder promised it, and this site keeps it — and enforces it rather than asserting
it. `vercel.json` sends a Content-Security-Policy of `default-src 'self'`, so a browser
refuses any script, font, image or request from anywhere else. `npm run preview` serves the
same headers, so a violation shows on the laptop before it reaches a preview deploy.

## Nothing here is a copy

The mark and the fonts are imported from where the apps keep them (`@brand` →
`docs/brand`, `@fonts` → the Android design module), so the site cannot drift from the
product.
