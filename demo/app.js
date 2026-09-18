/* The visual replay reads recorded evidence. It never invents a successful run. */
(() => {
  'use strict';
  const data = window.COMPACT_DEMO;
  const labels = ['Checkpoint', 'Archive', 'Score', 'Reduce', 'Agent patch', 'Verify'];
  const starts = [0, 5, 10, 15, 20, 26];
  const $ = id => document.getElementById(id);
  const escape = value => String(value).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  const format = value => Number(value).toLocaleString('en-US');
  let selected = 0, time = 0, playing = !new URLSearchParams(location.search).has('capture'), previous = performance.now();
  if (!data || !data.cases.length) { $('headline').textContent = 'Recorded evidence is unavailable.'; return; }
  data.cases.forEach((c, index) => { const option = document.createElement('option'); option.value = index; option.textContent = c.title; $('task').append(option); });
  labels.forEach((label, index) => { const button = document.createElement('button'); button.innerHTML = `<b>${index + 1}</b>${label}`; button.onclick = () => {time = starts[index]; render();}; $('steps').append(button); });
  function table(c) {
    return `<table class="score-table"><thead><tr><th>REPRESENTATION</th><th>LOSS ESTIMATE</th><th>DECISION</th></tr></thead><tbody>${['extract','brief','reference','drop'].map(action => `<tr class="${action === c.action ? 'selected' : ''}"><td>${escape(action)}</td><td>${c.scores[action].toFixed(2)}</td><td class="${c.scores[action] >= .2 ? 'denied' : ''}">${action === c.action ? 'SELECTED' : c.scores[action] < .2 ? 'ELIGIBLE' : 'RETAIN MORE'}</td></tr>`).join('')}</tbody></table><div class="threshold"><span>Allowed loss &lt; 0.20</span><b>Estimates, not guarantees</b></div>`;
  }
  function details(c, phase) {
    if (phase === 0) return `<p class="detail-kicker">01 / A REAL CONTINUATION STARTS HERE</p><h3>One task. Two histories.</h3><p>${escape(c.goal)}</p><div class="fact-row"><span>Original investigation</span><strong>${c.original_lines} lines</strong></div><div class="fact-row"><span>Critical findings</span><strong>${c.findings.length} retained facts</strong></div><div class="fact-row"><span>Initial contract tests</span><strong class="fail">FAIL — bug confirmed</strong></div><p class="notice">The repositories are authored fixtures. Codex performs the edits and test runs.</p>`;
    if (phase === 1) return `<p class="detail-kicker">02 / ORIGINAL SAVED BEFORE SCORING</p><h3>Out of context.<br>Still recoverable.</h3><p>The complete request is archived before any content leaves active history.</p><div class="mini-code">snapshot\n<span class="hash">${escape(c.snapshot.slice(0,32))}<br>${escape(c.snapshot.slice(32))}</span></div><div class="fact-row"><span>Recovery check</span><strong>${c.recall_exact ? 'EXACT MATCH' : 'NOT VERIFIED'}</strong></div><p>Recall uses the original message. It does not rerun the tool.</p>`;
    if (phase === 2) return `<p class="detail-kicker">03 / LIVE JEV SCORING</p><h3>Compare actual replacements.</h3><p>Jev sees the proposed content for each action.</p>${table(c)}`;
    if (phase === 3) return `<p class="detail-kicker">04 / ONLY ACTIVE CONTEXT SHRINKS</p><h3>${format(c.before)} → ${format(c.after)} tokens.</h3><p>${c.original_lines - c.retained_lines} original log lines leave active context. The archive retains every line.</p><div class="mini-code">${c.findings.map(line => `<span class="code-add">${escape(line)}</span>`).join('\n')}</div><div class="fact-row"><span>Instructions, current goal, failure</span><strong>UNCHANGED</strong></div>`;
    if (phase === 4) return `<p class="detail-kicker">05 / ACTUAL CODEX OUTPUT</p><h3>A code change, not a mock.</h3><p>${escape(c.module)} · ${escape(data.model)} · recorded compacted run</p><div class="diff">${c.patch.split('\n').filter(line => !line.startsWith('---') && !line.startsWith('+++')).slice(0,10).map(line => `<span class="${line.startsWith('+') ? 'code-add' : line.startsWith('-') ? 'code-remove' : 'code-meta'}">${escape(line)}</span>`).join('\n')}</div><p>Behavior tests and protected file checks run separately.</p>`;
    return `<p class="detail-kicker">06 / HELD-OUT CONTRACT CHECKS</p><h3>Continue. Fix. Verify.</h3><div class="result-grid"><div class="result-box"><span>Full-history continuations</span><strong>${c.full_passed}/${c.full_total}</strong><small>passed</small></div><div class="result-box green"><span>Compacted continuations</span><strong>${c.compact_passed}/${c.compact_total}</strong><small>passed</small></div></div><div class="test-row"><span>Contract tests in this task</span><b class="pass">${c.test_count} checks</b></div><div class="test-row"><span>Original content recovery</span><b class="${c.recall_exact ? 'pass' : 'fail'}">${c.recall_exact ? 'EXACT' : 'UNVERIFIED'}</b></div><div class="test-row"><span>Pilot size</span><b>${data.case_count} tasks · ${data.run_count} runs</b></div><p class="notice">Small pilot, not a general success rate. Context reduction does not imply equal savings in billed model tokens.</p>`;
  }
  function render() {
    const c = data.cases[selected];
    const phase = starts.reduce((found, start, i) => time >= start ? i : found, 0);
    const progress = phase < 3 ? 0 : Math.min(1, (time - 15) / 2.5);
    $('before').textContent = format(c.before);
    $('after').textContent = format(Math.round(c.before + (c.after - c.before) * progress));
    $('reduction').textContent = (100 * (1 - c.after / c.before) * progress).toFixed(1) + '%';
    $('latency').textContent = (c.compaction_ms / 1000).toFixed(2);
    $('task-short').textContent = c.title;
    $('line-count').textContent = progress > .5 ? `${c.retained_lines} literal source lines + archive reference` : `${c.original_lines} source lines · ${c.findings.length} critical findings`;
    $('history-state').textContent = phase < 3 ? 'BEFORE' : 'AFTER';
    $('action-badge').textContent = phase < 3 ? 'FULL' : c.action.toUpperCase();
    $('investigation').classList.toggle('compacted', phase >= 3);
    $('detail-title').textContent = labels[phase];
    $('step-number').textContent = `0${phase + 1} / 06`;
    const lines = progress < .5 ? [...c.original_head, '… ' + (c.original_lines - c.findings.length - 2) + ' more source lines …', ...c.findings] : ['[literal excerpts; other text omitted]', ...c.findings];
    $('log-window').innerHTML = lines.map(line => `<div class="log-line ${line.includes('FINDING') ? 'finding' : line.includes('omitted') ? 'omit' : ''}">${escape(line)}</div>`).join('');
    $('detail-content').innerHTML = details(c, phase);
    $('seek').value = time;
    $('time').textContent = `00:${String(Math.floor(time)).padStart(2,'0')} / 00:32`;
    $('play').textContent = playing ? 'Pause' : 'Play';
    [...$('steps').children].forEach((button, i) => button.classList.toggle('active', phase === i));
  }
  $('task').onchange = event => {selected = Number(event.target.value);time = 0;render();};
  $('seek').oninput = event => {time = Number(event.target.value);playing = false;render();};
  $('play').onclick = () => {if(time >= 31.9)time=0;playing=!playing;render();};
  $('restart').onclick = () => {time=0;playing=true;render();};
  window.renderAt = (seconds, caseIndex=0) => {playing=false;selected=caseIndex;time=Math.max(0,Math.min(31.9,seconds));$('task').value=caseIndex;render();};
  function tick(now) {if(playing){time=Math.min(31.9,time+(now-previous)/1000);if(time>=31.9)playing=false;render();}previous=now;requestAnimationFrame(tick);}
  render();requestAnimationFrame(tick);
})();
