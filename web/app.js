let state={},tasks=[],index=0,term,fit,ws;
const $=id=>document.getElementById(id);
const exam=$('exam'),divider=$('panel-divider'),questionPanel=$('question-panel');
function setQuestionWidth(width){
  const available=exam.clientWidth-divider.offsetWidth;
  if(available<=0||window.matchMedia('(max-width:700px)').matches)return;
  const clamped=Math.max(270,Math.min(available-150,width));
  exam.style.setProperty('--question-ratio',clamped/available);
}
divider.onpointerdown=e=>{
  if(e.button!==0)return;
  e.preventDefault();divider.focus();divider.setPointerCapture(e.pointerId);
  exam.classList.add('resizing');
};
divider.onpointermove=e=>{
  if(divider.hasPointerCapture(e.pointerId))setQuestionWidth(e.clientX-exam.getBoundingClientRect().left-divider.offsetWidth/2);
};
divider.onpointerup=e=>{if(divider.hasPointerCapture(e.pointerId))divider.releasePointerCapture(e.pointerId)};
divider.onlostpointercapture=()=>exam.classList.remove('resizing');
divider.onkeydown=e=>{
  const available=exam.clientWidth-divider.offsetWidth;
  let width=questionPanel.getBoundingClientRect().width;
  if(e.key==='ArrowLeft')width-=e.shiftKey?50:10;
  else if(e.key==='ArrowRight')width+=e.shiftKey?50:10;
  else if(e.key==='Home')width=270;
  else if(e.key==='End')width=available-150;
  else return;
  e.preventDefault();setQuestionWidth(width);
};
new ResizeObserver(()=>{
  const available=exam.clientWidth-divider.offsetWidth;
  if(available<=0||window.matchMedia('(max-width:700px)').matches)return;
  divider.setAttribute('aria-valuemin',Math.ceil(270/available*100));
  divider.setAttribute('aria-valuemax',Math.floor((available-150)/available*100));
  divider.setAttribute('aria-valuenow',Math.round(questionPanel.getBoundingClientRect().width/available*100));
}).observe(questionPanel);
async function action(name,body){const r=await fetch('/api/'+name,{method:'POST',headers:{'Content-Type':'application/json'},body:body?JSON.stringify(body):'{}',keepalive:true});if(!r.ok)alert(await r.text());await refresh()}
$('prepare').onclick=()=>action('prepare');$('retry').onclick=()=>action('retry');$('start').onclick=()=>action('start');$('reset').onclick=()=>{if(confirm('Destroy all VMs belonging to this lab?'))action('reset')};$('submit').onclick=()=>{if(confirm('Submit the exam? This cannot be undone.'))action('submit')};
$('flag').onchange=$('done').onchange=()=>{state.flags??={};state.done??={};state.flags[tasks[index].ID]=$('flag').checked;state.done[tasks[index].ID]=$('done').checked;action('mark',{id:tasks[index].ID,flag:state.flags[tasks[index].ID],done:state.done[tasks[index].ID]})};
function selectQuestion(i){if(i<0||i>=tasks.length)return;index=i;state.selected=tasks[i].ID;draw();action('select',{id:tasks[i].ID})}
$('previous').onclick=()=>selectQuestion(index-1);
$('next').onclick=()=>selectQuestion(index+1);
$('question-select').onchange=e=>selectQuestion(Number(e.target.value));
let renderedInstructions,renderedQuestion;
function renderMarkdown(element,markdown){
  element.innerHTML=DOMPurify.sanitize(marked.parse(markdown),{
    ALLOWED_TAGS:['p','br','strong','em','ul','ol','li','pre','code','blockquote','h3','h4'],
    ALLOWED_ATTR:['class'],ALLOW_DATA_ATTR:false
  });
  element.querySelectorAll('pre code').forEach(block=>Prism.highlightElement(block));
}
$('reveal').onclick=()=>{
  const t=tasks[index];if(!t)return;
  const solution=$('solution'),expanded=solution.hidden;
  if(expanded){
    solution.replaceChildren();
    const heading=document.createElement('h3');heading.textContent='Step-by-step solution';solution.append(heading);
    if(t.Explanation){const explanation=document.createElement('div');renderMarkdown(explanation,t.Explanation);solution.append(explanation)}
    const steps=document.createElement('ol');
    for(const step of t.Steps||['Run the solution commands below.','Check that the resulting resources satisfy the question.']){
      const li=document.createElement('li');renderMarkdown(li,step);steps.append(li);
    }
    solution.append(steps);
    const label=document.createElement('h4');label.textContent='Solution commands';solution.append(label);
    const pre=document.createElement('pre'),code=document.createElement('code');
    code.className='language-bash';code.textContent=t.Solution||'No solution is available for this question.';
    pre.append(code);solution.append(pre);Prism.highlightElement(code);
  }
  solution.hidden=!expanded;
  $('reveal').setAttribute('aria-expanded',String(expanded));
  $('reveal').textContent=expanded?'Hide Instructions':'Reveal Instructions';
};
function draw(){
  const t=tasks[index];if(!t)return;
  if(renderedQuestion!==t.ID){
    $('solution').hidden=true;$('solution').replaceChildren();
    $('reveal').textContent='Reveal Instructions';$('reveal').setAttribute('aria-expanded','false');
    renderedQuestion=t.ID;
  }
  const select=$('question-select');
  tasks.forEach((x,i)=>{
    let option=select.options[i];
    if(!option){option=document.createElement('option');option.value=i;select.append(option)}
    const text=`Question ${i+1}: ${x.Title}${state.flags?.[x.ID]?' ⚑':''}${state.done?.[x.ID]?' ✓':''}`;
    if(option.textContent!==text)option.textContent=text;
  });
  while(select.options.length>tasks.length)select.remove(select.options.length-1);
  if(select.value!==String(index))select.value=index;
  $('previous').disabled=index===0;$('next').disabled=index===tasks.length-1;
  $('title').textContent=`Question ${index+1}: ${t.Title} (${t.Points} points)`;
  if(renderedInstructions!==t.Instructions){
    renderMarkdown($('instructions'),t.Instructions);
    renderedInstructions=t.Instructions;
  }
  $('flag').checked=!!state.flags?.[t.ID];$('done').checked=!!state.done?.[t.ID];
  tick();
}
function terminalKey(e){
  const key=e.key.toLowerCase();
  if(!e.ctrlKey||!e.shiftKey||e.altKey||e.metaKey||!['c','v'].includes(key))return true;
  e.preventDefault();e.stopPropagation();
  if(e.type==='keydown'&&!e.repeat)terminalClipboard(key);
  return false;
}
async function terminalClipboard(key){
  try{
    if(key==='c'&&!term.hasSelection())return;
    if(!navigator.clipboard)throw new Error('Clipboard access requires HTTPS or localhost.');
    if(key==='c')await navigator.clipboard.writeText(term.getSelection());
    else term.paste(await navigator.clipboard.readText());
  }catch(e){alert('Could not '+(key==='c'?'copy terminal selection':'paste into terminal')+': '+e.message)}
}
function connect(){if(ws)ws.close();if(!window.Terminal){$('terminal').textContent='Terminal library could not load. Check network access and reconnect.';return}if(!term){term=new Terminal({cursorBlink:true,convertEol:false});fit=new FitAddon.FitAddon();term.loadAddon(fit);term.open($('terminal'));term.attachCustomKeyEventHandler(terminalKey);term.onData(s=>{if(ws?.readyState===1)ws.send(new TextEncoder().encode(s))});window.addEventListener('resize',resize);new ResizeObserver(resize).observe($('terminal'))}ws=new WebSocket((location.protocol==='https:'?'wss://':'ws://')+location.host+'/api/terminal');ws.binaryType='arraybuffer';ws.onopen=()=>{term.writeln('\r\nConnected.');resize();term.focus()};ws.onmessage=e=>term.write(new Uint8Array(e.data));ws.onclose=()=>{term.writeln('\r\nDisconnected. Use Reconnect to open a new shell.')};}
function resize(){if(!fit||!$('terminal').clientWidth||!$('terminal').clientHeight)return;fit.fit();if(ws?.readyState===1)ws.send(JSON.stringify({cols:term.cols,rows:term.rows}))}$('reconnect').onclick=connect;
let logTerm,logFit,logText='';
function updateLog(text){
  const log=$('log');
  if(logText===text)return;
  log.hidden=!text;
  if(!logTerm&&text){
    logTerm=new Terminal({disableStdin:true,convertEol:true,cursorBlink:false,cursorInactiveStyle:'none',fontSize:14,scrollback:3000,theme:{background:'#111111',foreground:'#eeeeee'}});
    logFit=new FitAddon.FitAddon();logTerm.loadAddon(logFit);logTerm.open(log);
    log.querySelector('textarea').setAttribute('aria-label','Lab operation log (read only)');
    new ResizeObserver(()=>{if(log.clientWidth&&log.clientHeight)logFit.fit()}).observe(log);
  }
  if(logTerm){
    if(log.clientWidth&&log.clientHeight)logFit.fit();
    const appended=text.startsWith(logText);
    logTerm.write(appended?text.slice(logText.length):'\x1bc'+text,()=>logTerm.scrollToBottom());
  }
  logText=text;
}
async function refresh(){try{let old=state.phase;state=await(await fetch('/api/status',{cache:'no-store'})).json();$('phase').textContent=state.phase;$('message').textContent=state.message||'';$('prepare').hidden=state.phase!=='idle';$('start').hidden=state.phase!=='ready';$('retry').hidden=state.phase!=='error'||!['prepare','reset'].includes(state.failed_operation);$('reset').hidden=['running','preparing','grading','resetting'].includes(state.phase);$('submit').hidden=state.phase!=='running';$('home').hidden=['running','completed'].includes(state.phase);$('exam').hidden=state.phase!=='running';$('results').hidden=state.phase!=='completed';if(state.phase==='running'||state.phase==='completed'){tasks=await(await fetch('/api/tasks',{cache:'no-store'})).json();if(state.selected){let selected=tasks.findIndex(t=>t.ID===state.selected);if(selected>=0)index=selected}draw()}if(['preparing','resetting','error'].includes(state.phase)){let log=await(await fetch('/api/log',{cache:'no-store'})).text();updateLog(log)}else updateLog('');if(state.phase==='running'&&old!=='running')connect();if(old==='running'&&state.phase!=='running'&&ws)ws.close();if(state.phase==='completed')renderResults();tick()}catch(e){$('message').textContent='Cannot reach server: '+e}}
function resultCode(parent,text,language='none'){
  const pre=document.createElement('pre'),code=document.createElement('code');
  code.className='language-'+language;code.textContent=text;
  pre.append(code);parent.append(pre);Prism.highlightElement(code);
}
function renderResults(){
  const results=state.results||[],container=$('results');
  const score=results.reduce((n,r)=>n+(r.earned??(r.passed?(tasks.find(t=>t.ID===r.id)?.Points||0):0)),0);
  const hasErrors=results.some(r=>r.error||r.checks?.some(c=>c.error));
  container.replaceChildren();
  const heading=document.createElement('h1');
  heading.textContent=`${hasErrors?'Provisional results (grading errors)':'Results from captured answers'}: ${score} / ${tasks.reduce((n,t)=>n+t.Points,0)}`;
  container.append(heading);
  function resetButton(){
    const button=document.createElement('button');button.textContent='Destroy Lab / Reset';
    button.className='results-reset';button.onclick=$('reset').onclick;container.append(button);
  }
  resetButton();
  const list=document.createElement('ol');list.className='result-list';
  tasks.forEach((t,i)=>{
    const r=results.find(x=>x.id===t.ID),earned=r?.earned??(r?.passed?t.Points:0);
    const status=!r||r.error||r.checks?.some(c=>c.error)?'error':r.passed?'correct':'wrong';
    const card=document.createElement('li');card.className='result-card '+status;
    const title=document.createElement('h2');title.textContent=`Question ${i+1}: ${t.Title}`;card.append(title);
    const summary=document.createElement('p');summary.className='result-status';
    summary.textContent=`${status==='error'?'Grading error':status==='correct'?'Correct':earned>0?'Partially correct':'Wrong'} — ${earned} / ${t.Points} points`;card.append(summary);
    if(!r)resultCode(card,'No evaluation is available.');
    if(r?.error)resultCode(card,r.error);
    if(r?.checks?.length){
      const checks=document.createElement('ul');checks.className='result-checks';
      for(const c of r.checks){
        const item=document.createElement('li');item.className=c.error?'error':c.earned===c.points?'correct':'wrong';
        const label=document.createElement('p');label.textContent=`${c.error?'Grading error':c.earned===c.points?'Correct':'Wrong'}: ${c.description} — ${c.earned} / ${c.points} points`;item.append(label);
        if(c.error)resultCode(item,c.error);
        checks.append(item);
      }
      card.append(checks);
    }
    if(t.Explanation){const explanation=document.createElement('div');renderMarkdown(explanation,t.Explanation);card.append(explanation)}
    const solutionHeading=document.createElement('h3');solutionHeading.textContent='Solution commands';card.append(solutionHeading);
    resultCode(card,t.Solution||'No solution is available for this question.','bash');
    list.append(card);
  });
  container.append(list);
  if(hasErrors){const retry=document.createElement('button');retry.textContent='Retry grading';retry.onclick=()=>action('regrade');container.append(retry)}
  resetButton();
}
function tick(){
  if(state.phase!=='running'){$('timer').textContent='';return}
  const remaining=Math.max(0,(new Date(state.deadline)-Date.now())/1000);
  const percent=Math.min(100,remaining/(state.duration_seconds||7200)*100);
  const minutes=Math.ceil(remaining/60),text=`${minutes} ${minutes===1?'minute':'minutes'}`;
  $('timer').textContent=text;
  $('time-fill').style.width=percent+'%';
  $('time-progress').dataset.level=percent<=10?'red':percent<=25?'orange':'green';
  $('time-progress').setAttribute('aria-valuenow',String(Math.round(percent)));
  $('time-progress').setAttribute('aria-valuetext',text+' remaining');
}
refresh();setInterval(refresh,5000);setInterval(tick,1000);
