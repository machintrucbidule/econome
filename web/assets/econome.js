/* EconoMe — shared mockup interactivity (Stage 3).
   Extracted from the validated forecast.html and made defensive so screens
   that lack forecast-only elements (month nav, savings band, lifecycle) do
   not error. Theme toggle, row expand/collapse with per-user persistence,
   read-only drill-down, scope switch, month/year picker, clamped tooltips,
   collapse panels / mobile drawers, review-strip variant switching.
   No data layer (mockups are static). */
(function(){
  "use strict";
  var $=function(s,r){return (r||document).querySelector(s)};
  var $$=function(s,r){return Array.prototype.slice.call((r||document).querySelectorAll(s))};

  /* theme */
  window.toggleTheme=function(){var h=document.documentElement;h.setAttribute('data-theme',h.getAttribute('data-theme')==='dark'?'light':'dark');};

  /* expand / collapse with persistence (keyed per page so screens don't collide) */
  var OKEY='tf-open:'+(location.pathname.split('/').pop()||'index');
  function openSet(){try{return JSON.parse(localStorage.getItem(OKEY)||'{}')}catch(e){return{}}}
  function save(o){try{localStorage.setItem(OKEY,JSON.stringify(o))}catch(e){}}
  function setRow(k,open){
    $$('.chev[data-k="'+k+'"]').forEach(function(c){c.classList.toggle('open',open)});
    $$('[data-c="'+k+'"],[data-d="'+k+'"]').forEach(function(r){r.classList.toggle('hidden',!open)});
  }
  window.tog=function(k){
    var chev=$('.chev[data-k="'+k+'"]');
    var open=chev?!chev.classList.contains('open'):true;
    setRow(k,open);
    var o=openSet(); if(open)o[k]=1; else delete o[k]; save(o);
  };
  function wire(){
    $$('tr.tog').forEach(function(tr){
      tr.addEventListener('click',function(e){ if(e.target.closest('a,.help,.cell-edit,.rowdel,button,select,input'))return; window.tog(tr.getAttribute('data-k')); });
    });
    $$('.chev').forEach(function(c){c.addEventListener('click',function(e){e.stopPropagation();window.tog(c.getAttribute('data-k'));});c.tabIndex=0;c.addEventListener('keydown',function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();window.tog(c.getAttribute('data-k'));}});});
    var o=openSet();Object.keys(o).forEach(function(k){setRow(k,true)});
  }

  /* scope (rail account list) */
  window.setScope=function(el,scope){
    $$('.acct').forEach(function(a){a.classList.remove('on')}); if(el)el.classList.add('on');
    $$('[data-v^="scope:"]').forEach(function(n){n.classList.toggle('hidden',n.getAttribute('data-v')!=='scope:'+scope)});
  };

  /* band state (forecast sweep savings encart) — guarded */
  window.setBand=function(v){
    $$('[data-bv]').forEach(function(n){n.classList.toggle('hidden',n.getAttribute('data-bv')!==v)});
    var card=$('#s-pb'),val=$('#s-pb-v'),sub=$('#s-pb-s');
    if(!card)return;
    if(v==='overdraft'){card.classList.remove('good');card.classList.add('bad');if(val)val.textContent='−143 €';if(sub)sub.textContent='risque de découvert le 28/06';}
    else{card.classList.remove('bad');card.classList.add('good');if(val)val.textContent='313 €';if(sub)sub.textContent='aucun découvert';}
  };

  /* lifecycle (forecast) — guarded */
  window.setLife=function(v){
    var main=$('#main'),empty=$('#empty'),lock=$('#lockbar'),right=$('#rightpanel');
    if(!main&&!empty&&!lock)return;
    if(v==='notcreated'){if(main)main.classList.add('hidden');if(right)right.style.visibility='hidden';if(empty)empty.classList.remove('hidden');if(lock)lock.classList.add('hidden');}
    else{if(main)main.classList.remove('hidden');if(right)right.style.visibility='';if(empty)empty.classList.add('hidden');if(lock)lock.classList.toggle('hidden',v!=='locked');}
  };

  /* review segmented */
  window.seg=function(b){$$('button',b.parentNode).forEach(function(x){x.classList.remove('on')});b.classList.add('on');};

  /* month picker — only active where a .mnav exists */
  var FR=['Janvier','Février','Mars','Avril','Mai','Juin','Juillet','Août','Septembre','Octobre','Novembre','Décembre'];
  var cM=5,cY=2026,vY=2026;
  function mlabel(){var l=$('#mlabel');if(l)l.textContent=FR[cM]+' '+cY;}
  function mrender(){var y=$('#mpy');if(y)y.textContent=vY;$$('#mp .mp-g button').forEach(function(b,i){b.classList.toggle('cur',i===cM&&vY===cY)});}
  window.mStep=function(d){cM+=d;if(cM<0){cM=11;cY--;}if(cM>11){cM=0;cY++;}mlabel();};
  window.mpToggle=function(){var mp=$('#mp');if(!mp)return;var open=!mp.classList.contains('open');mp.classList.toggle('open',open);if(open){vY=cY;mrender();}};
  window.mpY=function(d){vY+=d;mrender();};
  window.mpPick=function(m){cM=m;cY=vY;mlabel();var mp=$('#mp');if(mp)mp.classList.remove('open');};
  document.addEventListener('click',function(e){if(!e.target.closest('.mnav')){var mp=$('#mp');if(mp)mp.classList.remove('open');}});

  /* tooltip clamped in viewport */
  var tt=$('#tt');
  function showTip(el){if(!tt)return;tt.textContent=el.getAttribute('data-tip');tt.classList.add('show');var r=el.getBoundingClientRect();tt.style.left='0';tt.style.top='0';var w=tt.offsetWidth,h=tt.offsetHeight,m=8;var left=r.left+r.width/2-w/2;left=Math.max(m,Math.min(left,innerWidth-w-m));var top=r.bottom+8;if(top+h+m>innerHeight)top=r.top-h-8;tt.style.left=left+'px';tt.style.top=Math.max(m,top)+'px';}
  function hideTip(){if(tt)tt.classList.remove('show');}
  document.addEventListener('mouseover',function(e){var h=e.target.closest&&e.target.closest('.help');if(h)showTip(h);});
  document.addEventListener('mouseout',function(e){var h=e.target.closest&&e.target.closest('.help');if(h)hideTip();});
  document.addEventListener('focusin',function(e){var h=e.target.closest&&e.target.closest('.help');if(h)showTip(h);});
  document.addEventListener('focusout',function(e){var h=e.target.closest&&e.target.closest('.help');if(h)hideTip();});
  addEventListener('scroll',hideTip,true);

  /* panels */
  window.toggleLeft=function(){var a=$('.app');if(!a)return;a.classList.toggle('mini-left');a.classList.toggle('show-left');};
  window.toggleRight=function(){var a=$('.app');if(!a)return;a.classList.toggle('no-right');a.classList.toggle('show-right');};
  window.closeDrawers=function(){var a=$('.app');if(a)a.classList.remove('show-left','show-right');};

  /* =========================================================
     CUSTOM CONTROLS — floating layer, menu, calendar, month.
     One float open at a time; keyboard + viewport clamping.
     ========================================================= */
  var FL=null;
  function flPlace(el,anchor,o){o=o||{};var r=anchor.getBoundingClientRect();var w=el.offsetWidth,h=el.offsetHeight,m=8;
    var left=o.alignRight?(r.right-w):r.left;left=Math.max(m,Math.min(left,innerWidth-w-m));
    var top=r.bottom+6;if(top+h+m>innerHeight)top=r.top-h-6;top=Math.max(m,top);
    el.style.left=left+'px';el.style.top=top+'px';}
  function flClose(){if(FL){document.removeEventListener('keydown',FL.key,true);if(FL.el.parentNode)FL.el.parentNode.removeChild(FL.el);var a=FL.anchor;FL=null;if(a&&a.focus)try{a.focus()}catch(e){}}}
  function flOpen(el,anchor,key,o){flClose();document.body.appendChild(el);flPlace(el,anchor,o);FL={el:el,anchor:anchor,key:key};document.addEventListener('keydown',key,true);}
  window.emFloatClose=flClose;
  document.addEventListener('mousedown',function(e){if(FL&&!FL.el.contains(e.target)&&e.target!==FL.anchor&&!(FL.anchor&&FL.anchor.contains&&FL.anchor.contains(e.target)))flClose();},true);
  addEventListener('resize',flClose);addEventListener('scroll',function(){flClose()},true);
  function pad(n){return (n<10?'0':'')+n;}

  /* menu(anchor, opts[{value,label,ic?}], current, onPick(value,label,opt)) */
  window.emMenu=function(anchor,opts,current,onPick){
    var el=document.createElement('div');el.className='em-menu';el.setAttribute('role','listbox');
    var hi=0;opts.forEach(function(o,i){if(o.value===current)hi=i;});
    opts.forEach(function(o,i){var d=document.createElement('div');d.className='opt'+(o.value===current?' sel':'');d.setAttribute('role','option');
      d.innerHTML=(o.ic?'<span class="ic">'+o.ic+'</span>':'')+'<span>'+o.label+'</span><span class="tick">✓</span>';
      d.addEventListener('mouseenter',function(){setHi(i)});
      d.addEventListener('click',function(){pick(i)});el.appendChild(d);});
    function items(){return $$('.opt',el);}
    function setHi(i){hi=i;items().forEach(function(x,j){x.classList.toggle('hi',j===i)});var n=items()[i];if(n)n.scrollIntoView({block:'nearest'});}
    function pick(i){var o=opts[i];flClose();if(onPick)onPick(o.value,o.label,o);}
    function key(e){if(e.key==='Escape'){e.preventDefault();flClose();}
      else if(e.key==='ArrowDown'){e.preventDefault();setHi(Math.min(opts.length-1,hi+1));}
      else if(e.key==='ArrowUp'){e.preventDefault();setHi(Math.max(0,hi-1));}
      else if(e.key==='Enter'||e.key===' '){e.preventDefault();pick(hi);}}
    flOpen(el,anchor,key);setHi(hi);
  };

  /* calendar(anchor, curStr 'JJ/MM' | '', onPick('JJ/MM' | '')) — year fixed 2026 (mockup) */
  var FRm=['Janvier','Février','Mars','Avril','Mai','Juin','Juillet','Août','Septembre','Octobre','Novembre','Décembre'];
  window.emCal=function(anchor,curStr,onPick){
    var today={d:26,m:5,y:2026};var base={y:2026,m:5},sel=null;
    if(curStr&&/^\d{2}\/\d{2}$/.test(curStr)){var p=curStr.split('/');base={y:2026,m:parseInt(p[1],10)-1};sel={d:parseInt(p[0],10),m:base.m};}
    var el=document.createElement('div');el.className='em-cal';
    function render(){
      var fw=(new Date(base.y,base.m,1).getDay()+6)%7;var dim=new Date(base.y,base.m+1,0).getDate();
      var h='<div class="em-cal-h"><button data-n="-1" aria-label="Mois précédent">‹</button><span>'+FRm[base.m]+' '+base.y+'</span><button data-n="1" aria-label="Mois suivant">›</button></div><div class="em-cal-g">';
      ['L','M','M','J','V','S','D'].forEach(function(x){h+='<div class="dow">'+x+'</div>'});
      for(var i=0;i<fw;i++)h+='<div class="d mut"></div>';
      for(var dd=1;dd<=dim;dd++){var c='d';if(today.d===dd&&today.m===base.m&&today.y===base.y)c+=' today';if(sel&&sel.d===dd&&sel.m===base.m)c+=' sel';h+='<div class="'+c+'" data-d="'+dd+'" tabindex="0">'+dd+'</div>';}
      h+='</div><div class="em-cal-foot"><button data-t="today">Aujourd\'hui</button><button data-t="clear">Effacer</button></div>';
      el.innerHTML=h;
      $$('.em-cal-h button',el).forEach(function(b){b.onclick=function(){base.m+=parseInt(b.getAttribute('data-n'),10);if(base.m<0){base.m=11;base.y--;}if(base.m>11){base.m=0;base.y++;}render();flPlace(el,anchor);};});
      $$('.em-cal-g .d[data-d]',el).forEach(function(c){c.onclick=function(){flClose();onPick(pad(parseInt(c.getAttribute('data-d'),10))+'/'+pad(base.m+1));};});
      $$('.em-cal-foot button',el).forEach(function(b){b.onclick=function(){flClose();onPick(b.getAttribute('data-t')==='today'?(pad(today.d)+'/'+pad(today.m+1)):'');};});
    }
    function key(e){if(e.key==='Escape'){e.preventDefault();flClose();}}
    render();flOpen(el,anchor,key);
  };

  /* monthPop(anchor, curIdx, curYear, onPick(idx,label,year)) */
  window.emMonth=function(anchor,curIdx,curYear,onPick){
    var vY=curYear||2026;var el=document.createElement('div');el.className='em-mp';
    var SH=['Janv.','Févr.','Mars','Avr.','Mai','Juin','Juil.','Août','Sept.','Oct.','Nov.','Déc.'];
    function render(){
      var h='<div class="em-mp-y"><button data-n="-1">‹</button><span>'+vY+'</span><button data-n="1">›</button></div><div class="em-mp-g">';
      SH.forEach(function(m,i){h+='<button data-m="'+i+'" class="'+(i===curIdx&&vY===(curYear||2026)?'cur':'')+'">'+m+'</button>';});
      h+='</div>';el.innerHTML=h;
      $$('.em-mp-y button',el).forEach(function(b){b.onclick=function(){vY+=parseInt(b.getAttribute('data-n'),10);render();flPlace(el,anchor);};});
      $$('.em-mp-g button',el).forEach(function(b){b.onclick=function(){var i=parseInt(b.getAttribute('data-m'),10);flClose();onPick(i,FRm[i],vY);};});
    }
    function key(e){if(e.key==='Escape'){e.preventDefault();flClose();}}
    render();flOpen(el,anchor,key);
  };

  /* autocomplete(input, items[{label,count}], onPick) — text field + ranked
     suggestion list (matches first, then most-used). Free text allowed; a
     suggestion is taken only when highlighted/clicked. Own floating box. */
  window.emAutocomplete=function(input,items,onPick){
    var box=null,hi=-1,cur=[];
    function esc(s){return String(s).replace(/[&<>]/g,function(c){return ({'&':'&amp;','<':'&lt;','>':'&gt;'})[c];});}
    function hl(label,q){q=(q||'').trim();if(!q)return esc(label);var l=label.toLowerCase(),i=l.indexOf(q.toLowerCase());if(i<0)return esc(label);return esc(label.slice(0,i))+'<b>'+esc(label.slice(i,i+q.length))+'</b>'+esc(label.slice(i+q.length));}
    function rank(q){q=(q||'').toLowerCase().trim();
      return items.map(function(it){var l=it.label.toLowerCase();var s=q?(l.indexOf(q)===0?2:(l.indexOf(q)>=0?1:-1)):0;return {it:it,s:s};})
        .filter(function(x){return x.s>=0;})
        .sort(function(a,b){return b.s!==a.s?b.s-a.s:((b.it.count||0)-(a.it.count||0));})
        .slice(0,8).map(function(x){return x.it;});}
    function place(){var r=input.getBoundingClientRect();box.style.minWidth=Math.max(r.width,180)+'px';box.style.left='0';box.style.top='0';var bw=box.offsetWidth,bh=box.offsetHeight,m=8;var left=Math.max(m,Math.min(r.left,innerWidth-bw-m));var top=r.bottom+4;if(top+bh+m>innerHeight)top=r.top-bh-4;box.style.left=left+'px';box.style.top=Math.max(m,top)+'px';}
    function render(){cur=rank(input.value);if(!cur.length||document.activeElement!==input){close();return;}
      if(!box){box=document.createElement('div');box.className='em-menu em-auto';document.body.appendChild(box);}
      box.innerHTML='';cur.forEach(function(it,i){var d=document.createElement('div');d.className='opt'+(i===hi?' hi':'');
        d.innerHTML='<span class="lbl">'+hl(it.label,input.value)+'</span>'+(it.count?'<span class="cnt">'+it.count+'×</span>':'');
        d.addEventListener('mousedown',function(e){e.preventDefault();choose(it.label);});box.appendChild(d);});place();}
    function upd(){if(!box)return;$$('.opt',box).forEach(function(x,j){x.classList.toggle('hi',j===hi)});var n=$$('.opt',box)[hi];if(n)n.scrollIntoView({block:'nearest'});}
    function close(){if(box){box.parentNode&&box.parentNode.removeChild(box);box=null;}hi=-1;}
    function choose(v){input.value=v;close();if(onPick)onPick(v);}
    input.addEventListener('input',function(){hi=-1;render();});
    input.addEventListener('focus',function(){hi=-1;render();});
    input.addEventListener('keydown',function(e){
      if(box&&cur.length){
        if(e.key==='ArrowDown'){e.preventDefault();hi=(hi<cur.length-1)?hi+1:0;upd();return;}
        if(e.key==='ArrowUp'){e.preventDefault();hi=(hi>0)?hi-1:cur.length-1;upd();return;}
        if(e.key==='Enter'&&hi>=0){e.preventDefault();e.stopImmediatePropagation();choose(cur[hi].label);return;}
        if(e.key==='Escape'){e.preventDefault();e.stopImmediatePropagation();close();return;}
      }
    });
    input.addEventListener('blur',function(){setTimeout(close,120);});
    if(document.activeElement===input){hi=-1;render();} /* show at once for an already-focused (inline) field */
    return {close:close};
  };

  wire();
})();
