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
let renderedInstructions;
function draw(){
  const t=tasks[index];if(!t)return;
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
    $('instructions').innerHTML=DOMPurify.sanitize(marked.parse(t.Instructions),{
      ALLOWED_TAGS:['p','br','strong','em','ul','ol','li','pre','code','blockquote','h3','h4'],
      ALLOWED_ATTR:['class'],ALLOW_DATA_ATTR:false
    });
    $('instructions').querySelectorAll('pre code').forEach(block=>Prism.highlightElement(block));
    renderedInstructions=t.Instructions;
  }
  $('flag').checked=!!state.flags?.[t.ID];$('done').checked=!!state.done?.[t.ID];$('solution').textContent=t.Solution?'Solution: '+t.Solution:'';
  tick();
}
function connect(){if(ws)ws.close();if(!window.Terminal){$('terminal').textContent='Terminal library could not load. Check network access and reconnect.';return}if(!term){term=new Terminal({cursorBlink:true,convertEol:false});fit=new FitAddon.FitAddon();term.loadAddon(fit);term.open($('terminal'));term.onData(s=>{if(ws?.readyState===1)ws.send(new TextEncoder().encode(s))});window.addEventListener('resize',resize);new ResizeObserver(resize).observe($('terminal'))}ws=new WebSocket((location.protocol==='https:'?'wss://':'ws://')+location.host+'/api/terminal');ws.binaryType='arraybuffer';ws.onopen=()=>{term.writeln('\r\nConnected.');resize();term.focus()};ws.onmessage=e=>term.write(new Uint8Array(e.data));ws.onclose=()=>{term.writeln('\r\nDisconnected. Use Reconnect to open a new shell.')};}
function resize(){if(!fit||!$('terminal').clientWidth||!$('terminal').clientHeight)return;fit.fit();if(ws?.readyState===1)ws.send(JSON.stringify({cols:term.cols,rows:term.rows}))}$('reconnect').onclick=connect;$('copy').onclick=async()=>{try{await navigator.clipboard.writeText(tasks[index].Instructions)}catch(e){alert('Could not copy instructions: '+e)}};
async function refresh(){try{let old=state.phase;state=await(await fetch('/api/status',{cache:'no-store'})).json();$('phase').textContent=state.phase;$('message').textContent=state.message||'';$('prepare').hidden=state.phase!=='idle';$('start').hidden=state.phase!=='ready';$('retry').hidden=state.phase!=='error'||!['prepare','reset'].includes(state.failed_operation);$('reset').hidden=['running','preparing','grading','resetting'].includes(state.phase);$('submit').hidden=state.phase!=='running';$('home').hidden=['running','completed'].includes(state.phase);$('exam').hidden=state.phase!=='running';$('results').hidden=state.phase!=='completed';if(state.phase==='running'||state.phase==='completed'){tasks=await(await fetch('/api/tasks',{cache:'no-store'})).json();if(state.selected){let selected=tasks.findIndex(t=>t.ID===state.selected);if(selected>=0)index=selected}draw()}if(['preparing','resetting','error'].includes(state.phase)){let log=await(await fetch('/api/log',{cache:'no-store'})).text();$('log').textContent=log}else $('log').textContent='';if(state.phase==='running'&&old!=='running')connect();if(old==='running'&&state.phase!=='running'&&ws)ws.close();if(state.phase==='completed'){let score=state.results.reduce((n,r)=>n+(r.earned??(r.passed?(tasks.find(t=>t.ID===r.id)?.Points||0):0)),0);$('results').replaceChildren();let h=document.createElement('h1');h.textContent=`${state.results.some(r=>r.error)?'Provisional results (grading errors)':'Results from captured answers'}: ${score} / ${tasks.reduce((n,t)=>n+t.Points,0)}`;$('results').append(h);let list=document.createElement('ol');for(const t of tasks){let r=state.results.find(x=>x.id===t.ID),li=document.createElement('li');li.textContent=`${t.Title}: ${r?.error?'GRADING ERROR: '+r.error:`${r?.earned??(r?.passed?t.Points:0)} / ${t.Points} points`} — ${t.Explanation||""} Solution: ${t.Solution}`;if(r?.checks?.length){let parts=document.createElement('ul');for(const c of r.checks){let part=document.createElement('li');part.textContent=`${c.description}: ${c.error?'GRADING ERROR: '+c.error:`${c.earned} / ${c.points}`}`;parts.append(part)}li.append(parts)}list.append(li)}$('results').append(list);if(state.results.some(r=>r.error)){let retry=document.createElement('button');retry.textContent='Retry grading';retry.onclick=()=>action('regrade');$('results').append(retry)}let b=document.createElement('button');b.textContent='Destroy lab / reset';b.onclick=$('reset').onclick;$('results').append(b)}tick()}catch(e){$('message').textContent='Cannot reach server: '+e}}
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
