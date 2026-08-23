#!/usr/bin/env node
// Renders one of the manual sources to a print-ready HTML page.
//
// The three PDFs under docs/ used to be produced by a pipeline that was never
// committed, so they could not be rebuilt when the product changed. This
// renderer covers exactly the Markdown the manuals use and nothing more: a
// wider subset would be untested weight. Anything it does not recognise is
// emitted as an escaped paragraph rather than silently dropped.
import { readFileSync, writeFileSync } from 'node:fs';

const [, , sourcePath, outputPath, version, buildDate] = process.argv;
if (!sourcePath || !outputPath || !version || !buildDate) {
  console.error('usage: render.mjs <source.md> <output.html> <version> <build-date>');
  process.exit(2);
}

const escapeHTML = (text) => text
  .replace(/&/g, '&amp;')
  .replace(/</g, '&lt;')
  .replace(/>/g, '&gt;')
  .replace(/"/g, '&quot;');

/** Inline markup, applied after escaping so document text can never inject HTML. */
function inline(text) {
  return escapeHTML(text)
    .replace(/`([^`]+)`/g, '<code>$1</code>')
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
    .replace(/\[([^\]]+)\]\(([^)\s]+)\)/g, '<a href="$2">$1</a>');
}

function renderBody(markdown) {
  const lines = markdown.split('\n');
  const out = [];
  let list = null; // 'ul' | 'ol'
  let fence = null;

  const closeList = () => {
    if (list) { out.push(`</${list}>`); list = null; }
  };
  const openList = (kind) => {
    if (list !== kind) { closeList(); out.push(`<${kind}>`); list = kind; }
  };

  for (const raw of lines) {
    if (raw.startsWith('```')) {
      if (fence === null) { closeList(); fence = []; } else { out.push(`<pre>${escapeHTML(fence.join('\n'))}</pre>`); fence = null; }
      continue;
    }
    if (fence !== null) { fence.push(raw); continue; }

    const line = raw.trimEnd();
    if (!line.trim()) { closeList(); continue; }

    const heading = /^(#{1,4})\s+(.*)$/.exec(line);
    if (heading) {
      closeList();
      const level = heading[1].length;
      out.push(`<h${level}>${inline(heading[2])}</h${level}>`);
      continue;
    }
    if (/^---+$/.test(line)) { closeList(); out.push('<hr />'); continue; }

    const ordered = /^\s*\d+\.\s+(.*)$/.exec(line);
    if (ordered) { openList('ol'); out.push(`<li>${inline(ordered[1])}</li>`); continue; }

    const bullet = /^\s*-\s+(.*)$/.exec(line);
    if (bullet) {
      const nested = /^\s{2,}/.test(line);
      openList('ul');
      out.push(`<li${nested ? ' class="nested"' : ''}>${inline(bullet[1])}</li>`);
      continue;
    }

    closeList();
    out.push(`<p>${inline(line)}</p>`);
  }
  closeList();
  if (fence !== null) out.push(`<pre>${escapeHTML(fence.join('\n'))}</pre>`);
  return out.join('\n');
}

const markdown = readFileSync(sourcePath, 'utf8');
const lines = markdown.split('\n');

// The document title and its opening paragraph become the cover, so both are
// consumed here rather than repeated as the first thing on page two.
let cursor = 0;
const takeBlank = () => { while (cursor < lines.length && !lines[cursor].trim()) cursor += 1; };
takeBlank();
const headingLine = /^#\s+(.*)$/.exec(lines[cursor] ?? '');
const title = headingLine ? headingLine[1].trim() : 'igame';
if (headingLine) cursor += 1;
takeBlank();
let summary = '';
if (cursor < lines.length && lines[cursor].trim() && !lines[cursor].startsWith('#') && !/^---+$/.test(lines[cursor].trim())) {
  summary = lines[cursor].trim();
  cursor += 1;
}
takeBlank();
// A rule directly under the consumed front matter would open the body with a
// stray divider.
if (/^---+$/.test((lines[cursor] ?? '').trim())) cursor += 1;

const body = renderBody(lines.slice(cursor).join('\n'));

const html = `<!doctype html>
<html lang="ko">
<head>
<meta charset="utf-8" />
<title>${escapeHTML(title)}</title>
<style>
  /* Fonts resolve from the build container; no network asset is referenced so
     the same page renders identically offline. */
  :root {
    --ink: #101d2b;
    --muted: #4c5c6d;
    --accent: #0b6a8f;
    --rule: #d6e0ea;
    --surface: #f2f6fa;
    --code-bg: #0f1d2e;
    --code-ink: #d6e9f7;
  }
  @page { size: A4; margin: 18mm 16mm 20mm; }
  @page :first { margin: 0; }
  * { box-sizing: border-box; }
  body {
    margin: 0;
    color: var(--ink);
    font-family: "Noto Sans CJK KR", "Noto Sans KR", "Malgun Gothic", "Noto Sans", sans-serif;
    font-size: 10.5pt;
    line-height: 1.7;
    -webkit-print-color-adjust: exact;
    print-color-adjust: exact;
  }
  .cover {
    height: 297mm;
    padding: 46mm 24mm 24mm;
    background: linear-gradient(150deg, #07101d 0%, #0f2c3f 58%, #0b6a8f 100%);
    color: #f3f7fb;
    display: flex;
    flex-direction: column;
    page-break-after: always;
  }
  .cover .mark { font-size: 12pt; letter-spacing: .34em; text-transform: uppercase; opacity: .72; }
  .cover h1 { font-size: 30pt; line-height: 1.25; margin: 14mm 0 0; font-weight: 800; }
  .cover .summary { margin-top: 8mm; font-size: 11.5pt; line-height: 1.8; color: #c7dced; max-width: 132mm; }
  .cover .meta { margin-top: auto; font-size: 10pt; color: #a9c4d8; }
  .cover .meta strong { color: #f3f7fb; font-weight: 700; }
  .cover .rule { width: 34mm; height: 3px; background: #67d7ff; margin-top: 10mm; }
  h2 {
    font-size: 15pt; margin: 11mm 0 4mm; padding-bottom: 2.5mm;
    border-bottom: 2px solid var(--accent); color: var(--accent);
    page-break-after: avoid;
  }
  h3 { font-size: 12pt; margin: 7mm 0 2.5mm; color: var(--ink); page-break-after: avoid; }
  h4 { font-size: 11pt; margin: 5mm 0 2mm; color: var(--muted); page-break-after: avoid; }
  p { margin: 0 0 3mm; }
  ul, ol { margin: 0 0 4mm; padding-left: 7mm; }
  li { margin-bottom: 1.6mm; }
  li.nested { margin-left: 5mm; list-style-type: circle; }
  hr { border: 0; border-top: 1px solid var(--rule); margin: 7mm 0; }
  code {
    font-family: "D2Coding", "Noto Sans Mono CJK KR", ui-monospace, monospace;
    font-size: 9.5pt; background: var(--surface); color: var(--accent);
    padding: 0.4mm 1.2mm; border-radius: 2px;
  }
  pre {
    font-family: "D2Coding", "Noto Sans Mono CJK KR", ui-monospace, monospace;
    font-size: 8.5pt; line-height: 1.5; background: var(--code-bg); color: var(--code-ink);
    padding: 4mm 5mm; border-radius: 3mm; overflow: hidden; white-space: pre;
    page-break-inside: avoid;
  }
  pre code { background: none; color: inherit; padding: 0; }
  strong { font-weight: 700; }
  a { color: var(--accent); text-decoration: none; }
  .cover code {
    background: rgba(103, 215, 255, .16);
    color: #bfe6ff;
  }
</style>
</head>
<body>
  <section class="cover">
    <div class="mark">igame</div>
    <h1>${escapeHTML(title)}</h1>
    <div class="rule"></div>
    <p class="summary">${inline(summary)}</p>
    <div class="meta">
      <div><strong>서비스 버전</strong> v${escapeHTML(version)}</div>
      <div><strong>발행일</strong> ${escapeHTML(buildDate)}</div>
      <div>Apache License 2.0 · 사내 폐쇄망 배포용</div>
    </div>
  </section>
  <main>
${body}
  </main>
</body>
</html>
`;

writeFileSync(outputPath, html);
console.log(`rendered ${sourcePath} -> ${outputPath} (${title})`);
