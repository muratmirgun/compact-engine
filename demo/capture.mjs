// Render the same HTML replay into frames. Requires Playwright and Chrome.
import {createRequire} from 'node:module';
import {mkdir} from 'node:fs/promises';
import path from 'node:path';
import {pathToFileURL, fileURLToPath} from 'node:url';
const require = createRequire(import.meta.url);
const {chromium} = require('playwright');
const output = path.resolve(process.argv[2] || 'reports/demo-frames');
const fps = Number(process.argv[3] || 10);
await mkdir(output, {recursive:true});
const browser = await chromium.launch({headless:true, channel:'chrome'});
try {
  const page = await browser.newPage({viewport:{width:1280,height:900},deviceScaleFactor:1});
  const entry = path.join(path.dirname(fileURLToPath(import.meta.url)), 'index.html');
  await page.goto(pathToFileURL(entry).href + '?capture=1');
  await page.addStyleTag({content:'* { transition: none !important; }'});
  await page.waitForFunction(() => typeof window.renderAt === 'function');
  await page.evaluate(() => document.fonts.ready);
  for (let frame=0; frame<32*fps; frame++) {
    await page.evaluate(t => window.renderAt(t), frame/fps);
    await page.screenshot({path:path.join(output,`${String(frame).padStart(4,'0')}.png`)});
  }
  for (let index=0; index<3; index++) {
    await page.evaluate(i => window.renderAt(29,i), index);
    await page.screenshot({path:path.join(output,`case-${index}-result.png`)});
  }
  console.log(`Rendered ${32*fps} replay frames and three case screenshots.`);
} finally {
  await browser.close();
}
