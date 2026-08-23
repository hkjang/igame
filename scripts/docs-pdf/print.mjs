#!/usr/bin/env node
// Prints the rendered manual pages to PDF with headless Chromium.
//
// Playwright is deliberately not a repository dependency — docs/release.md
// keeps it out of the product image and the lockfiles — so this runs inside the
// same pinned container the release browser gate uses, installing the client
// there and nowhere else.
import { createRequire } from 'node:module';
import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';

// This file is mounted read-only, so it cannot carry its own node_modules.
// Resolving from the working directory is the same approach scripts/browser-smoke.mjs
// uses for the release browser gate.
const requireFromWorkingDirectory = createRequire(`${process.cwd()}/package.json`);
let chromium;
try {
  ({ chromium } = requireFromWorkingDirectory('playwright'));
} catch (error) {
  console.error('Playwright is not installed in the working directory. Install playwright@1.55.0 there, then retry.');
  throw error;
}

const manifest = JSON.parse(readFileSync(process.argv[2], 'utf8'));
const version = process.argv[3];

const footer = (title) => `
<div style="width:100%;font-size:7pt;color:#6b7c8d;padding:0 16mm;
     font-family:'Noto Sans CJK KR','Noto Sans KR',sans-serif;
     display:flex;justify-content:space-between;">
  <span>${title} · igame v${version}</span>
  <span><span class="pageNumber"></span> / <span class="totalPages"></span></span>
</div>`;

const browser = await chromium.launch();
try {
  for (const { html, pdf, title } of manifest) {
    const page = await browser.newPage();
    const failures = [];
    page.on('requestfailed', (request) => failures.push(request.url()));
    // The manuals must be self-contained: a print that reaches the network
    // would not reproduce inside a closed network.
    page.on('request', (request) => {
      const url = request.url();
      if (!url.startsWith('file:') && !url.startsWith('data:')) failures.push(url);
    });

    await page.goto(pathToFileURL(html).href, { waitUntil: 'load' });
    await page.emulateMedia({ media: 'print' });
    await page.pdf({
      path: pdf,
      format: 'A4',
      printBackground: true,
      displayHeaderFooter: true,
      headerTemplate: '<div></div>',
      footerTemplate: footer(title),
      margin: { top: '18mm', bottom: '20mm', left: '16mm', right: '16mm' },
    });
    await page.close();

    if (failures.length > 0) {
      console.error(`${pdf}: the page referenced external resources: ${[...new Set(failures)].join(', ')}`);
      process.exit(1);
    }
    console.log(`printed ${pdf}`);
  }
} finally {
  await browser.close();
}
