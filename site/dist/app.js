import {evaluateBoundary} from './boundary.js';
import {sourceLines,challengeLines,inspectSource} from './example.js';
// A deterministic browser simulation, never a shell or a connection to an agent.
const supportedLanguages = ['en','ja','zh-CN','de'];
const $ = selector => document.querySelector(selector);
const $$ = selector => [...document.querySelectorAll(selector)];
const escapeHTML = value => String(value).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
let preferred;
try { preferred = localStorage.getItem('tarigato-language'); } catch {}
let language = new URL(location.href).searchParams.get('lang') || preferred || 'en';
if (!supportedLanguages.includes(language)) language = 'en';
const english = await fetch('locales/en.json',{cache:'no-cache'}).then(r => {if(!r.ok) throw Error('English copy unavailable'); return r.json();});
let strings = english;
const t = key => strings[key] || english[key] || key;
let localeRequest = 0;
let copyTimer;
const boundaryState={expiry:100,repaired:false};
let homeRole=0;
function toast(message) { const node=$('.toast'); node.textContent=message; node.hidden=false; clearTimeout(copyTimer); copyTimer=setTimeout(()=>node.hidden=true,2600); }
function localLinks() {
  $$('a[href]').forEach(link => { const url=new URL(link.getAttribute('href'), location.href); if(url.origin===location.origin && /(?:\/|\/(?:docs|demo)\.html)$/.test(url.pathname)){url.searchParams.set('lang',language);link.href=url.pathname+url.search+url.hash;} });
}
async function setLanguage(next, updateURL=true) {
  if(!supportedLanguages.includes(next)) return;
  const request=++localeRequest;
  let translated=english;
  try { if(next!=='en'){ const response=await fetch(`locales/${next}.json`,{cache:'no-cache'}); if(!response.ok) throw Error('Translation unavailable'); translated=await response.json(); } }
  catch { if(request===localeRequest) toast('Translation could not load. Showing English.'); next='en'; }
  if(request!==localeRequest) return;
  language=next; strings={...english,...translated};
  document.documentElement.lang=next; $('#language').value=next;
  $$('[data-i18n]').forEach(node=>node.textContent=t(node.dataset.i18n));
  $('.skip-link').textContent=t('skipContent');
  $('#language').setAttribute('aria-label',t('languageLabel'));
  const menu=$('.menu-toggle'); menu.setAttribute('aria-label',t(menu.getAttribute('aria-expanded')==='true'?'menuClose':'menuOpen'));
  $('#reset')?.setAttribute('aria-label',t('reset'));
  if($('#reset')) $('#reset').title=t('reset');
  if($('#reset')) $('#reset').textContent=t('reset');
  if($('#stage-strip')) $('#stage-strip').setAttribute('aria-label',t('stageNav'));
  if($('.view-switch')) $('.view-switch').setAttribute('aria-label',t('diagramView'));
  $('.trace-cases')?.setAttribute('aria-label',t('traceTryCase'));
  try {localStorage.setItem('tarigato-language',next);} catch {}
  if(updateURL){const url=new URL(location.href);url.searchParams.set('lang',next);history.replaceState(null,'',url);}
  localLinks();
  if($('#home-example')) renderHomeExample();
  if($('#boundary')) renderBoundary();
  if($('#terminal')) { renderSimulation(); renderDiagram(); }
  if($('#docs-content')) await renderDocs();
}
$('#language').addEventListener('change',event=>setLanguage(event.target.value));
$('.menu-toggle').addEventListener('click',event=>{const nav=$('#mobile-nav');nav.hidden=!nav.hidden;event.currentTarget.setAttribute('aria-expanded',String(!nav.hidden));event.currentTarget.setAttribute('aria-label',t(nav.hidden?'menuOpen':'menuClose'));});
$('#mobile-nav').addEventListener('click',event=>{if(event.target.closest('a')){ $('#mobile-nav').hidden=true;$('.menu-toggle').setAttribute('aria-expanded','false');$('.menu-toggle').setAttribute('aria-label',t('menuOpen')); }});
document.addEventListener('keydown',event=>{if(event.key==='Escape'){ $('#mobile-nav').hidden=true;$('.menu-toggle').setAttribute('aria-expanded','false');$('.menu-toggle').setAttribute('aria-label',t('menuOpen')); }});
document.addEventListener('click',async event=>{const button=event.target.closest('[data-copy]');if(!button)return;try{await navigator.clipboard.writeText(button.dataset.copy);toast(t('copied'));}catch{toast(t('copyFailed'));}});

function renderHomeExample(){
 $$('.mini-roles button').forEach((button,index)=>button.setAttribute('aria-pressed',String(index===homeRole)));
 $('.mini-roles').setAttribute('aria-label',t('miniSteps'));
 $('#mini-title').textContent=t('miniTitle'+homeRole);
 $('#mini-text').textContent=t('miniText'+homeRole);
 const original=sourceLines('pass',[])[3].trim();
 const changed=sourceLines('pass',['builder'])[3].trim();
 const code=homeRole===0?`- ${original}\n+ ${changed}`:homeRole===1?`Valid(100, 100) → ${inspectSource('pass',['builder'],100).candidate}`:'changes.patch\ntests.patch';
 $('#mini-code').textContent=code;
}
if($('#home-example')){
 $('.mini-roles').addEventListener('click',event=>{const button=event.target.closest('[data-home-role]');if(!button)return;homeRole=Number(button.dataset.homeRole);renderHomeExample();});
 $('#mini-next').addEventListener('click',()=>{homeRole=(homeRole+1)%3;renderHomeExample();});
}

function renderBoundary(){
 const result=evaluateBoundary(boundaryState.expiry,boundaryState.repaired);
 $('#expiry-display').textContent=boundaryState.expiry;
 $('#expression-text').textContent=`${boundaryState.expiry} ${boundaryState.repaired?'>':'>='} 100`;
 $('.boundary-expression').setAttribute('aria-label',`${t('candidateLabel')}: ${$('#expression-text').textContent}`);
 $('#boundary').classList.toggle('is-repaired',boundaryState.repaired);
 $('#boundary').dataset.mismatch=String(!result.matches);
 $('#candidate-verdict').textContent=t(result.candidate?'accepts':'rejects');
 $('#requirement-verdict').textContent=t(result.expected?'accepts':'rejects');
 $('#boundary-observation').textContent=t(!result.matches?'boundaryBug':boundaryState.expiry===100?'boundaryFixed':'boundaryAgreement');
 $('#repair-boundary').textContent=t(boundaryState.repaired?'restoreOriginal':'showRepair');
 $('#repair-boundary').setAttribute('aria-pressed',String(boundaryState.repaired));
 $('#expiry').setAttribute('aria-valuetext',`${t('expiryLabel')}: ${boundaryState.expiry}; ${t('nowLabel')}: 100`);
}
if($('#boundary')){
 $('#expiry').addEventListener('input',event=>{boundaryState.expiry=Number(event.target.value);renderBoundary();});
 $('#exact-boundary').addEventListener('click',()=>{boundaryState.expiry=100;$('#expiry').value='100';renderBoundary();});
 $('#repair-boundary').addEventListener('click',()=>{boundaryState.repaired=!boundaryState.repaired;renderBoundary();});
}

const state={scenario:'pass',step:0,playing:false,diagram:'game',block:0,probe:100};
let playbackTimer;
const stepInfo={
 baseline:['BASELINE','baseline','controller'],builder:['BUILDER','builder','builder'],candidate:['CANDIDATE','candidate','controller'],challenger:['CHALLENGER','challenger','challenger'],verify1:['VERIFY 1','verify1','controller'],verify2:['VERIFY 2','verify2','controller'],repair:['REPAIR','repair','builder'],final:['FINAL','final','controller'],report:['REPORT','report','human'],inconclusive:['STOP','inconclusive','human']
};
function sequence(){const base=['baseline','builder','candidate','challenger','verify1','verify2'];if(state.scenario==='inconclusive')return [...base,'inconclusive'];return [...base,...(state.scenario==='repair'?['repair']:[]),'final','report'];}
function stopPlayback(){clearInterval(playbackTimer);state.playing=false;}
function advance(){const steps=sequence();state.step=Math.min(steps.length,state.step+1);if(state.step===steps.length)stopPlayback();renderSimulation();}
function setScenario(scenario){if(!['pass','repair','inconclusive'].includes(scenario))throw new Error('Unknown scenario');stopPlayback();state.scenario=scenario;state.step=0;$('#scenario').value=scenario;renderSimulation();}
function jumpToStep(step){if(!Number.isInteger(step)||step<0||step>sequence().length)throw new Error('Step out of range');stopPlayback();state.step=step;renderSimulation();}
function resultFor(id){
  const fails=state.scenario==='repair'&&id.startsWith('verify');
  if(id==='baseline'||id==='candidate')return ['pass','1 test passed'];
  if(id==='builder')return ['pass','Candidate ready for checks'];
  if(id==='challenger')return ['pass','One test submitted'];
  if(fails)return ['fail','Challenge failed; original tests passed'];
  if(state.scenario==='inconclusive'&&id==='verify2')return ['fail','Assertion failed; previous run passed'];
  if(id.startsWith('verify'))return ['pass','2 tests passed'];
  if(id==='repair')return ['pass','One repair prepared'];
  if(id==='final')return ['pass','2 tests passed'];
  if(id==='inconclusive')return ['fail','Inconsistent outcomes; review required'];
  return ['pass','ready_for_review'];
}
function codeLines(lines){return lines.map((line,i)=>`<span class="code-muted">${String(i+1).padStart(2,' ')}</span>  ${escapeHTML(line)}`).join('\n');}
function renderCode(current){
 const through=sequence().slice(0,state.step);
 let file='session.go',lines=sourceLines(state.scenario,through),caption=!through.includes('builder')?'codeInitial':lines[3].includes('>=')?'codeUnfixed':'codeChanged';
 if(['challenger','verify1','verify2'].includes(current)){file='tarigato_challenge_test.go';caption=state.scenario==='inconclusive'?'codeUnstable':'codeTest';lines=challengeLines(state.scenario);}
 if(current==='report'){file='~/.tarigato/runs/<id>/';caption='codeArtifacts';lines=['candidate.patch','changes.patch','tests.patch','report.md','result.json','logs/'];}
 $('#code-file').textContent=file;$('#code-preview').innerHTML=`<code>${codeLines(lines)}</code>`;$('#code-caption').textContent=t(caption);
}
function renderStory(){
 if(!$('#story-source'))return;
 const steps=sequence(),through=steps.slice(0,state.step),current=steps[state.step-1];
 const role=current?stepInfo[current][2]:null,probe=inspectSource(state.scenario,through,state.probe),source=probe.lines,test=challengeLines(state.scenario);
 $('.builder-pane').classList.toggle('active',role==='builder');
 $('.challenger-pane').classList.toggle('active',role==='challenger');
 $('#source-state').textContent=t(through.includes('repair')?'repairedState':through.includes('builder')?'candidateState':'startingSource');
 $('#test-state').textContent=t(through.includes('verify1')?'admittedState':through.includes('challenger')?'submittedState':'testPreview');
 $('#story-source').innerHTML=`<code>${codeLines(source).replace(/expiresAt &gt;=? now/g,'<span class="code-focus">$&</span>')}</code>`;
 $('#story-test').innerHTML=`<code>${codeLines(test.slice(4)).replace('Valid(100, 100)','<span class="code-focus">Valid(100, 100)</span>')}</code>`;
 $('#story-source-caption').textContent=t(!through.includes('builder')?'codeInitial':source[3].includes('>=')?'codeUnfixed':'codeChanged');
 $('#story-test-caption').textContent=t(state.scenario==='inconclusive'?'codeUnstable':'challengeExpectation');
 $('#trace-expression').classList.toggle('fixed',!source[3].includes('>='));
 $('#trace-expression').setAttribute('aria-label',source[3].trim());
 $('#trace-diff').innerHTML=source[3].includes('>=')?`<span>${escapeHTML(t('traceNoEdit'))}</span>`:'<code class="removed">− expiresAt &gt;= now</code><code class="added">+ expiresAt &gt; now</code>';
 $$('[data-probe]').forEach(button=>button.setAttribute('aria-pressed',String(Number(button.dataset.probe)===state.probe)));
 $('#trace-call').textContent=`Valid(${state.probe}, 100) = ${probe.candidate}`;
 $('#trace-verdict').textContent=t(probe.candidate?'accepts':'rejects');
 $('#trace-response').dataset.mismatch=String(!probe.matches);
 $('#trace-expectation').textContent=t(probe.expected?'traceExpectedTrue':'traceExpectedFalse');
 $('#trace-gate-status').textContent=t(through.includes('candidate')?'gamePassed':'gamePending');
 $('#trace-test-count').classList.toggle('submitted',through.includes('verify1'));
 $('#trace-test-join').textContent=t(through.includes('verify1')?'traceTestJoins':'traceTestProposal');
 $('#check-matrix').innerHTML=['verify1','verify2'].map((id,i)=>{const seen=through.includes(id),failed=seen&&resultFor(id)[0]==='fail';return `<tr><th scope="row">${escapeHTML(t(i?'gameRun2':'gameRun1'))}</th><td>${escapeHTML(t(seen?'gamePassed':'gamePending'))}</td><td class="${failed?'check-failed':''}">${escapeHTML(t(seen?(failed?'gameFailed':'gamePassed'):'gamePending'))}</td></tr>`;}).join('');
 $('#final-check-status').textContent=t(through.includes('final')?'gamePassed':state.step===steps.length?'notRun':'gamePending');
 $('#trace-result').textContent=state.step===steps.length?t(through.includes('final')?'traceReady':'traceNeedsReview'):'';
 const route=through.includes('verify2')?(state.scenario==='repair'?'repair':state.scenario==='inconclusive'?'stop':'pass'):null;
 $('#story-routes').innerHTML=[['pass','gamePassRoute','gamePassAction'],['repair','gameRepairRoute','gameRepairAction'],['stop','gameStopRoute','gameStopAction']].map(([id,label,action])=>`<div class="story-route ${id===route?'active':''}"><strong>${escapeHTML(t(label))}</strong><p>${escapeHTML(t(action))}</p></div>`).join('');
 const patch=lines=>lines[3].includes('>=')?t('artifactNoChange'):`--- a/session.go\n+++ b/session.go\n@@\n-    return expiresAt >= now\n+    return expiresAt > now`;
 $('#candidate-artifact').textContent=through.includes('builder')?patch(sourceLines(state.scenario,['builder'])):t('artifactPending');
 $('#changes-artifact').textContent=through.includes('builder')?(through.includes('final')?'':t('artifactStopped')+'\n\n')+patch(source):t('artifactPending');
 $('#tests-artifact').textContent=through.includes('verify1')?'+++ b/tarigato_challenge_test.go\n'+test.map(line=>'+'+line).join('\n'):t(through.includes('builder')?'artifactEmptyTests':'artifactPending');
 $('#report-artifact').textContent=state.step===steps.length?`status: ${state.scenario==='inconclusive'?'needs_review':'ready_for_review'}\ncandidate observations: ${resultFor('verify1')[0]} / ${resultFor('verify2')[0]}\nsource: ${state.scenario==='inconclusive'?'candidate.patch (unvalidated)':'changes.patch'}\nchallenge: tests.patch\nrecord: result.json + logs/`:t('artifactPending');
}
function renderSimulation(){
 const focusedStep=document.activeElement?.dataset.step;
 const steps=sequence(),current=steps[state.step-1],info=current?stepInfo[current]:null;
 const logs=steps.slice(0,state.step).map(id=>{const [result,detail]=resultFor(id);return `<div class="log-line ${result==='fail'?'is-fail':''} ${id===current?'active':''}"><span class="log-mark">${result==='fail'?'!':'✓'}</span><span class="log-stage">${stepInfo[id][0]}</span><span class="log-detail">${detail}</span></div>`;}).join('');
 $('#terminal-log').innerHTML=logs||`<div class="log-line"><span class="log-mark muted">›</span><span class="log-stage">READY</span><span class="log-detail">${escapeHTML(t('readyText'))}</span></div>`;
 const result=$('#terminal-result'); result.hidden=state.step!==steps.length;result.classList.toggle('warning',state.scenario==='inconclusive');result.textContent=state.scenario==='inconclusive'?'NEEDS REVIEW · inconsistent challenge outcomes':'READY FOR REVIEW · report.md + patches';
 $('#step-progress').textContent=`${state.step} / ${steps.length}`;$('#progress-fill').style.width=`${state.step/steps.length*100}%`;
 $('#play').textContent=t(state.playing?'pause':state.step===steps.length?'replay':'play');$('#next').disabled=state.step===steps.length;
 $('#step-number').textContent=String(state.step).padStart(2,'0');$('#step-title').textContent=t(info?info[1]+'Title':'initialTitle');$('#step-description').textContent=t(info?info[1]+'Description':'initialDescription');$('#step-role').textContent=t(info?info[2]:'initialRole');
 $('#stage-strip').innerHTML=steps.map((id,i)=>`<button data-step="${i+1}" class="${i+1===state.step?'active':''}" aria-current="${i+1===state.step?'step':'false'}">${escapeHTML(t(id==='inconclusive'?'report':id==='builder'||id==='challenger'?id+'Title':id))}</button>`).join('');
 if(focusedStep) $(`[data-step="${focusedStep}"]`)?.focus({preventScroll:true});
 renderCode(current);
 renderStory();
 if(state.diagram==='game') renderDiagram();
}
if($('#terminal')){
 $('.trace-cases').addEventListener('click',event=>{const button=event.target.closest('[data-probe]');if(button){state.probe=Number(button.dataset.probe);renderStory();}});
 $('#scenario').addEventListener('change',event=>setScenario(event.target.value));
 $('#play').addEventListener('click',()=>{if(state.playing){stopPlayback();renderSimulation();return;}if(state.step===sequence().length)state.step=0;state.playing=true;advance();if(state.playing)playbackTimer=setInterval(advance,1500);});
 $('#next').addEventListener('click',()=>{stopPlayback();advance();});
 $('#reset').addEventListener('click',()=>jumpToStep(0));
 $('#stage-strip').addEventListener('click',event=>{const button=event.target.closest('[data-step]');if(button)jumpToStep(Number(button.dataset.step));});
}

const diagrams={workflow:[['baseline','controller'],['builder','builder'],['candidate','controller'],['challenger','challenger'],['verify','controller'],['repair','builder'],['final','controller'],['report','human']],workspaces:[['source','human'],['control','controller'],['builderWork','builder'],['challengerWork','challenger'],['checksWork','controller'],['artifacts','human']]};
function renderGame(){
 const steps=sequence(),through=steps.slice(0,state.step),current=steps[state.step-1];
 const activeRole=current?stepInfo[current][2]:null;
 const role=(id,step,label,objective,move)=>`<div class="protocol-role ${activeRole===id?'active':''}"><h3><button data-game-step="${steps.indexOf(step)+1}" aria-current="${activeRole===id?'step':'false'}" aria-controls="terminal">${escapeHTML(t(label))}</button></h3><dl><dt>${escapeHTML(t('gameObjective'))}</dt><dd>${escapeHTML(t(objective))}</dd><dt>${escapeHTML(t('gameMove'))}</dt><dd>${escapeHTML(t(move))}</dd></dl></div>`;
 const observation=id=>through.includes(id)?t(resultFor(id)[0]==='pass'?'gamePassed':'gameFailed'):t('gamePending');
 const observed=through.includes('verify2');
 const route=observed?(state.scenario==='repair'?'repair':state.scenario==='inconclusive'?'stop':'pass'):null;
 return `<div class="protocol-current"><span>${escapeHTML(t('gameCurrent'))}</span><strong>${escapeHTML(t(current?stepInfo[current][1]+'Title':'initialTitle'))}</strong><span class="protocol-mode">${escapeHTML(t('simulationLabel'))}</span></div>
 <div class="protocol-roles" role="group" aria-label="${escapeHTML(t('gameDiagram'))}">
 ${role('builder','builder','builderTitle','gameBuilderObjective','gameBuilderMove')}
 ${role('controller','verify1','controlBlock','gameControllerObjective','gameControllerMove')}
 ${role('challenger','challenger','challengerTitle','gameChallengerObjective','gameChallengerMove')}
 </div>
 <div class="protocol-transfers"><div class="protocol-transfer" data-direction="forward"><strong>${escapeHTML(t('gameCandidate'))}</strong><code>candidate.patch</code><p>${escapeHTML(t('gameCandidateGate'))}</p></div><div class="protocol-transfer" data-direction="back"><strong>${escapeHTML(t('gameCounterexample'))}</strong><code>tarigato_challenge_test.go</code><p>${escapeHTML(t('gameCounterexampleGate'))}</p></div></div>
 <div class="protocol-observations"><p>${escapeHTML(t('gameChecks'))}</p><dl><div><dt>${escapeHTML(t('gameRun1'))}</dt><dd>${escapeHTML(observation('verify1'))}</dd></div><div><dt>${escapeHTML(t('gameRun2'))}</dt><dd>${escapeHTML(observation('verify2'))}</dd></div></dl></div>
 <div class="protocol-outcomes">${[['pass','gamePassRoute','gamePassAction'],['repair','gameRepairRoute','gameRepairAction'],['stop','gameStopRoute','gameStopAction']].map(([id,label,action])=>`<div class="protocol-outcome ${id===route?'active':''}"><strong>${escapeHTML(t(label))}</strong><p>${escapeHTML(t(action))}</p></div>`).join('')}</div>
 <p class="protocol-review">${escapeHTML(t('gameReview'))}</p><p class="protocol-help">${escapeHTML(t('gameControls'))}</p>`;
}
function renderDiagram(){
 const focusedMove=document.activeElement?.dataset.gameStep;
 $('#block-detail').hidden=state.diagram==='game';
 $('#architecture-diagram').dataset.view=state.diagram;
 $('#branch-note').hidden=state.diagram!=='workflow';
 $$('[data-diagram]').forEach(button=>{const active=button.dataset.diagram===state.diagram;button.classList.toggle('active',active);button.setAttribute('aria-pressed',String(active));});
 if(state.diagram==='game'){
  $('#architecture-diagram').innerHTML=renderGame();
  if(focusedMove) $(`[data-game-step="${focusedMove}"]`)?.focus({preventScroll:true});
  return;
 }

 const focusedBlock=document.activeElement?.dataset.block;
 const blocks=diagrams[state.diagram];if(state.block>=blocks.length)state.block=0;
 $('#architecture-diagram').innerHTML=blocks.map(([id,role],index)=>`<button class="diagram-block ${id==='repair'?'optional':''} ${index===state.block?'active':''}" data-block="${index}" data-condition="${escapeHTML(t('repairCondition'))}" aria-pressed="${index===state.block}" aria-controls="block-detail"><strong>${escapeHTML(t(id+'Block'))}</strong><span class="block-role">${escapeHTML(t(role))}</span></button>`).join('');
 if(state.diagram==='workflow'){ const repair=$('[data-block="5"]'); repair.insertAdjacentHTML('beforebegin',`<div class="workflow-branch"><span>${escapeHTML(t('branchPass'))} → ${escapeHTML(t('finalBlock'))}</span><span>${escapeHTML(t('branchFail'))} → ${escapeHTML(t('repairBlock'))}</span><span>${escapeHTML(t('branchStop'))}</span></div>`); }
 $('#architecture-diagram').dataset.view=state.diagram;
 $('#branch-note').hidden=state.diagram!=='workflow';
 if(focusedBlock) $(`[data-block="${focusedBlock}"]`)?.focus({preventScroll:true});
 const [id]=blocks[state.block];
 $('#block-detail').innerHTML=`<div><h3>${escapeHTML(t(id+'Block'))}</h3></div><div><p>${escapeHTML(t(id+'BlockText'))}</p><p class="detail-note">${escapeHTML(t(id+'BlockNote'))}</p></div>`;
 $$('[data-diagram]').forEach(button=>{const active=button.dataset.diagram===state.diagram;button.classList.toggle('active',active);button.setAttribute('aria-pressed',String(active));});
}
if($('#architecture-diagram')){
 $('.view-switch').addEventListener('click',event=>{const button=event.target.closest('[data-diagram]');if(button){state.diagram=button.dataset.diagram;state.block=0;renderDiagram();}});
 $('#architecture-diagram').addEventListener('click',event=>{const move=event.target.closest('[data-game-step]');if(move){jumpToStep(Number(move.dataset.gameStep));return;}const button=event.target.closest('[data-block]');if(button){state.block=Number(button.dataset.block);renderDiagram();}});
}

let docsRequest=0;
const validArticles=['quickstart','design','example','security','contributing'];
async function renderDocs(){
 const request=++docsRequest;
 const params=new URL(location.href).searchParams;
 let article=params.get('article')||'quickstart';if(!validArticles.includes(article))article='quickstart';
 const locale=article==='quickstart'?language:'en';
 $('#docs-english-note').hidden=article==='quickstart'||language==='en';
 $$('[data-article]').forEach(link=>{link.classList.toggle('active',link.dataset.article===article);if(link.dataset.article===article)link.setAttribute('aria-current','page');else link.removeAttribute('aria-current');});
 try{
  const response=await fetch(`content/${article}.${locale}.html`,{cache:'no-cache'});if(!response.ok)throw Error('Article unavailable');const html=await response.text();if(request!==docsRequest)return;
  $('#docs-content').innerHTML=html;
  $('#docs-content h1')?.remove();
  $('#article-title').textContent=t({quickstart:'docsQuickstart',design:'docsDesign',example:'docsExample',security:'docsSecurity',contributing:'docsContributing'}[article]);
  const source=article==='quickstart'?`README${language==='en'?'':'.'+language}.md`:({design:'docs/design.md',example:'docs/demo.md',security:'SECURITY.md',contributing:'CONTRIBUTING.md'}[article]);
  $('#docs-source').href=`https://github.com/tmls-ai/tarigato/blob/main/${source}`;
  $$('#docs-content pre').forEach(pre=>{if(pre.querySelector('.mermaid-graphic'))return;const code=pre.querySelector('code');if(!code)return;const button=document.createElement('button');button.className='copy-button';button.textContent=t('copy');button.dataset.copy=code.textContent;pre.append(button);});
  $('#docs-toc').innerHTML=$$('#docs-content h2').map(h=>`<a href="#${h.id}">${escapeHTML(h.textContent)}</a>`).join('');
  localLinks();
  if(location.hash){let target;try{target=document.getElementById(decodeURIComponent(location.hash.slice(1)));}catch{}target?.scrollIntoView({block:'start'});}
 }catch{if(request===docsRequest)$('#docs-content').textContent=t('docsMissing');}
}
if($('#docs-content')){
 $('.docs-sidebar').addEventListener('click',event=>{const link=event.target.closest('[data-article]');if(!link)return;event.preventDefault();history.pushState(null,'',link.href);renderDocs();window.scrollTo({top:0,behavior:'instant'});});
 window.addEventListener('popstate',()=>setLanguage(new URL(location.href).searchParams.get('lang')||'en',false));
}
await setLanguage(language,false);

// Optional browser-native access to the same simulation controls, without executing code.
if($('#terminal')&&document.modelContext?.registerTool){
 const lifecycle=new AbortController();
 const register=tool=>{try{Promise.resolve(document.modelContext.registerTool(tool,{signal:lifecycle.signal})).catch(()=>{});}catch{}};
 register({name:'set_tarigato_simulation',title:'Explore the Tarigato simulation',description:'Select a canned scenario and a completed step in the visible terminal simulation. This does not execute agents, commands, or change files.',inputSchema:{type:'object',properties:{scenario:{type:'string',enum:['pass','repair','inconclusive']},step:{type:'integer',minimum:0,maximum:9}},required:['scenario','step'],additionalProperties:false},annotations:{readOnlyHint:false,untrustedContentHint:false},execute(input){if(!input||typeof input!=='object'||Object.keys(input).some(k=>!['scenario','step'].includes(k))||!['pass','repair','inconclusive'].includes(input.scenario)||!Number.isInteger(input.step))throw new Error('Provide a known scenario and integer step.');const length=input.scenario==='repair'?9:input.scenario==='inconclusive'?7:8;if(input.step<0||input.step>length)throw new Error(`Step must be between 0 and ${length}.`);setScenario(input.scenario);jumpToStep(input.step);return {scenario:state.scenario,step:state.step,total:sequence().length,status:state.step===length?(state.scenario==='inconclusive'?'needs_review':'ready_for_review'):'in_progress',simulated:true};}});
 window.addEventListener('pagehide',()=>lifecycle.abort(),{once:true});
}
