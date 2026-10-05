# Tarigato website

[Live website](https://tarigato.vercel.app)

Static HTML, CSS, and JavaScript. The homepage has a three-step interactive example; the demo page has the full workflow simulation. Neither runs agents or executes terminal commands.

The interface and quickstart are available in English, Japanese, Simplified Chinese, and German. Detailed articles are in English.

## Preview

From the repository root:

```sh
python3 -m http.server 4173 --bind 127.0.0.1 --directory site/dist
```

Open http://127.0.0.1:4173/. No frontend packages or build step are required.

## Check

```sh
node --check site/dist/app.js
node site/scripts/check-boundary.mjs
node site/scripts/check-example.mjs
python3 site/scripts/check.py
```

## Deploy

Vercel uses the repository root and serves `site/dist`, as configured in `vercel.json`. The framework is Other, with no install or build command. The connected GitHub repository deploys `main` to production.

To deploy from a linked local checkout:

```sh
vercel deploy --prod
```

## Documentation

Repository Markdown is the source for the technical articles. With `markdown-it-py` installed, regenerate them from the repository root:

```sh
python3 site/scripts/sync-docs.py .
python3 site/scripts/check.py
```

Review the homepage and quickstart copy after regeneration. Keep the four locale dictionaries in sync; preserve commands and technical identifiers when translating.

The browser example reuses `dist/example.js` and `dist/boundary.js`. The passing scenario follows the recorded expiry example. Repair and unstable-test scenarios are illustrative.

## Logo

The mark is in `dist/assets/tarigato-mark.png`. Its generation prompt is in [BRAND.md](BRAND.md). Font licenses are included beside their font files.
