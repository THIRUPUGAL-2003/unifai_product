const __vite__mapDeps=(i,m=__vite__mapDeps,d=(m.f||(m.f=["assets/xlsx-JZtJaz5q.js","assets/chunk-CMxvf4Kt.js","assets/jspdf.es.min-DEjlmLyM.js","assets/index-C31c0DXa.js","assets/useStore-C3Xgsdq2.js","assets/react-CoHAqsLe.js","assets/with-selector-BA01i0Uz.js","assets/clsx-CjueKrWZ.js","assets/jsx-runtime-sLPvdpSW.js","assets/bundle-mjs-_luiSGPg.js","assets/index-e-OaeqdM.css","assets/slicedToArray-CJym0LHi.js"])))=>i.map(i=>d[i]);
import{N as e,V as t,i as n,q as r}from"./index-C31c0DXa.js";var i=e(`file-spreadsheet`,[[`path`,{d:`M6 22a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h8a2.4 2.4 0 0 1 1.704.706l3.588 3.588A2.4 2.4 0 0 1 20 8v12a2 2 0 0 1-2 2z`,key:`1oefj6`}],[`path`,{d:`M14 2v5a1 1 0 0 0 1 1h5`,key:`wfsgrz`}],[`path`,{d:`M8 13h2`,key:`yr2amv`}],[`path`,{d:`M14 13h2`,key:`un5t4a`}],[`path`,{d:`M8 17h2`,key:`2yhykz`}],[`path`,{d:`M14 17h2`,key:`10kma7`}]]),a=t||`/yes-panchi-logo.png`;function o(){let e=new Date;return`${e.getFullYear()}-${String(e.getMonth()+1).padStart(2,`0`)}-${String(e.getDate()).padStart(2,`0`)}`}function s(e,t){let n=URL.createObjectURL(e),r=document.createElement(`a`);r.href=n,r.download=t,document.body.appendChild(r),r.click(),r.remove(),setTimeout(()=>URL.revokeObjectURL(n),0)}function c(e){return String(e??``).replace(/&/g,`&amp;`).replace(/</g,`&lt;`).replace(/>/g,`&gt;`).replace(/"/g,`&quot;`)}async function l(e=a){try{let t=await fetch(e);if(!t.ok)return null;let n=await t.blob();return await new Promise(e=>{let t=new FileReader;t.onload=()=>{let n=String(t.result||``);if(!n){e(null);return}if(typeof Image<`u`){let t=new Image;t.onload=()=>{e({dataUrl:n,width:t.naturalWidth||1024,height:t.naturalHeight||334})},t.onerror=()=>e({dataUrl:n,width:1024,height:334}),t.src=n}else e({dataUrl:n,width:1024,height:334})},t.onerror=()=>e(null),t.readAsDataURL(n)})}catch{return null}}function u(e,t){let n=e[t];return n==null?``:String(n)}function d(e){let{title:t,subtitle:n,columns:r,rows:i,logoAsset:a}=e,o=r.map(e=>`<th style="border:1px solid #d1d5db;padding:6pt 8pt;background:#f3f4f6;text-align:left;font-size:9pt;font-weight:bold;color:#1f2937;">${c(e.header)}</th>`).join(``),s=i.map((e,t)=>{let n=t%2==1?`background:#f9fafb;`:`background:#ffffff;`;return`<tr>${r.map(t=>`<td style="border:1px solid #e5e7eb;padding:5pt 7pt;font-size:8.5pt;vertical-align:top;color:#111827;${n}">${c(u(e,t.key))}</td>`).join(``)}</tr>`}).join(``),l=``;if(a){let e=(a.width||1)/(a.height||1),t=Math.min(160,Math.max(70,Math.round(32*e)));l=`<p style="margin:0 0 10pt 0;"><img src="${a.dataUrl}" alt="Logo" width="${t}" height="32" style="width:${t}px;height:32px;max-width:${t}px;max-height:32px;display:block;margin:0 0 8pt 0;" /></p>`}return`${l}
<h1 style="font-size:16pt;margin:0 0 4pt 0;color:#111827;font-family:Calibri,'Segoe UI',Arial,sans-serif;">${c(t)}</h1>
${n?`<p class="subtitle" style="font-size:9.5pt;color:#4b5563;margin:0 0 6pt 0;font-family:Calibri,'Segoe UI',Arial,sans-serif;">${c(n)}</p>`:``}
<p class="meta" style="font-size:8.5pt;color:#6b7280;margin:0 0 12pt 0;font-family:Calibri,'Segoe UI',Arial,sans-serif;">Exported ${new Date().toLocaleString()} · ${i.length} row(s)</p>
<table style="border-collapse:collapse;width:100%;mso-table-layout-alt:fixed;">
<thead><tr>${o}</tr></thead>
<tbody>${s||`<tr><td colspan="${r.length}" style="padding:8pt;color:#6b7280;">No data</td></tr>`}</tbody>
</table>`}async function f(e){let t=await n(()=>import(`./xlsx-JZtJaz5q.js`).then(e=>e.i),__vite__mapDeps([0,1])),r=t.default??t,i=[e.columns.map(e=>e.header),...e.rows.map(t=>e.columns.map(e=>u(t,e.key)))],a=r.utils.aoa_to_sheet(i),c=r.utils.book_new();r.utils.book_append_sheet(c,a,(e.sheetName||`Export`).slice(0,31));let l=r.write(c,{bookType:`xlsx`,type:`array`});s(new Blob([l],{type:`application/vnd.openxmlformats-officedocument.spreadsheetml.sheet`}),`${e.filename}-${o()}.xlsx`)}async function p(e){let t=await l(e.logoSrc||a),n=d({title:e.title,subtitle:e.subtitle,columns:e.columns,rows:e.rows,logoAsset:t}),r=`\uFEFF<html xmlns:o="urn:schemas-microsoft-com:office:office"
 xmlns:w="urn:schemas-microsoft-com:office:word"
 xmlns="http://www.w3.org/TR/REC-html40">
<head>
<meta charset="utf-8">
<title>${c(e.title)}</title>
<!--[if gte mso 9]>
<xml>
 <w:WordDocument>
  <w:View>Print</w:View>
  <w:Zoom>100</w:Zoom>
  <w:DoNotOptimizeForBrowser/>
 </w:WordDocument>
</xml>
<![endif]-->
<style>
@page Section1 {
    size: 297mm 210mm;
    mso-page-orientation: landscape;
    margin: 15mm 15mm 15mm 15mm;
    mso-header-margin: 10mm;
    mso-footer-margin: 10mm;
}
div.Section1 {
    page: Section1;
}
body {
    font-family: Calibri, 'Segoe UI', Arial, sans-serif;
    color: #111827;
    margin: 0;
    padding: 24px;
    background-color: #ffffff;
}
h1 {
    font-family: Calibri, 'Segoe UI', Arial, sans-serif;
    font-size: 16pt;
    font-weight: bold;
    color: #111827;
    margin: 0 0 4pt 0;
}
p.subtitle {
    font-size: 9.5pt;
    color: #4b5563;
    margin: 0 0 8pt 0;
}
p.meta {
    font-size: 8.5pt;
    color: #6b7280;
    margin: 0 0 12pt 0;
}
table {
    border-collapse: collapse;
    width: 100%;
    mso-table-layout-alt: fixed;
}
th {
    background-color: #f3f4f6;
    border: 1px solid #d1d5db;
    padding: 6pt 8pt;
    font-family: Calibri, 'Segoe UI', Arial, sans-serif;
    font-size: 9pt;
    font-weight: bold;
    color: #1f2937;
    text-align: left;
}
td {
    border: 1px solid #e5e7eb;
    padding: 5pt 7pt;
    font-family: Calibri, 'Segoe UI', Arial, sans-serif;
    font-size: 8.5pt;
    color: #111827;
    vertical-align: top;
}
tr:nth-child(even) td {
    background-color: #f9fafb;
}
</style>
</head>
<body>
<div class="Section1">
${n}
</div>
</body>
</html>`;s(new Blob([r],{type:`application/msword;charset=utf-8`}),`${e.filename}-${o()}.doc`)}function m(e,t){let n=String(e??``).replace(/\s+/g,` `).trim();return n.length<=t?n:n.slice(0,Math.max(0,t-1))+`…`}async function h(e){let{jsPDF:t}=await n(async()=>{let{jsPDF:e}=await import(`./jspdf.es.min-DEjlmLyM.js`);return{jsPDF:e}},__vite__mapDeps([2,1,3,4,5,6,7,8,9,10,11])),i=await l(e.logoSrc||a),s=new t({orientation:`landscape`,unit:`mm`,format:`a4`}),c=s.internal.pageSize.getWidth(),d=s.internal.pageSize.getHeight(),f=c-20,p=e.columns,h=Math.max(1,p.length),g=p.map(e=>{let t=e.key.toLowerCase();return t.includes(`pattern`)||t.includes(`prompt`)||t.includes(`description`)||t.includes(`policy`)?2.4:t.includes(`name`)||t.includes(`domain`)||t.includes(`platform`)||t.includes(`query`)?1.4:t.includes(`active`)||t.includes(`action`)||t.includes(`severity`)||t.includes(`type`)?.7:1}),_=g.reduce((e,t)=>e+t,0)||1,v=g.map(e=>e/_*f),y=v.map(e=>Math.max(12,Math.floor(e/1.35))),b=3.6,x=1.2,S=e=>{s.setFillColor(243,244,246),s.rect(10,e-3.2,f,5.7,`F`),s.setFont(`helvetica`,`bold`),s.setFontSize(8),s.setTextColor(30,30,30);let t=10;for(let n=0;n<h;n++){let r=m(p[n].header,y[n]);s.text(r,t+x,e),t+=v[n]}return s.setDrawColor(200,200,200),s.setLineWidth(.2),s.line(10,e+1.8,10+f,e+1.8),e+4.2+1.2},C=(e,t)=>{s.setFont(`helvetica`,`normal`),s.setFontSize(7),s.setTextColor(140,140,140),s.text(`Page ${e} · ${t}`,10,d-4),s.text(`Powered by ${r}`,c-10,d-4,{align:`right`})},w=10;if(i)try{let e=(i.width||1)/(i.height||1),t=6.5,n=Math.min(26,Math.max(10,t*e));s.addImage(i.dataUrl,`PNG`,10,w-2,n,t),w+=8.5}catch{}s.setFont(`helvetica`,`bold`),s.setFontSize(14),s.setTextColor(17,17,17),s.text(e.title||`Export`,10,w),w+=6,s.setFont(`helvetica`,`normal`),s.setFontSize(9),s.setTextColor(100,100,100);let T=[e.subtitle,`Exported ${new Date().toLocaleString()}`,`${e.rows.length} row(s)`].filter(Boolean).join(` · `);s.text(T,10,w),w+=7,w=S(w);let E=1,D=`${e.rows.length} row(s)`;s.setFont(`helvetica`,`normal`),s.setFontSize(7.5),s.setTextColor(20,20,20);for(let t=0;t<e.rows.length;t++){let n=e.rows[t],r=p.map((e,t)=>m(u(n,e.key),y[t])).map((e,t)=>{let n=s.splitTextToSize(e||`—`,Math.max(8,v[t]-x*2));return Array.isArray(n)?n.slice(0,4):[String(n)]}),i=Math.max(1,...r.map(e=>e.length))*b+1.2;w+i>d-10-8&&(C(E,D),s.addPage(),E+=1,w=10,w=S(w),s.setFont(`helvetica`,`normal`),s.setFontSize(7.5),s.setTextColor(20,20,20)),t%2==1&&(s.setFillColor(249,250,251),s.rect(10,w-2.6,f,i,`F`));let a=10;for(let e=0;e<h;e++){let t=r[e];for(let e=0;e<t.length;e++)s.text(t[e],a+x,w+e*b);a+=v[e]}w+=i}C(E,D),s.save(`${e.filename}-${o()}.pdf`)}export{i,f as n,h as r,p as t};