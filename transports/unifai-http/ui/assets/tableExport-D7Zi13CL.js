const __vite__mapDeps=(i,m=__vite__mapDeps,d=(m.f||(m.f=["assets/xlsx-JZtJaz5q.js","assets/chunk-CMxvf4Kt.js","assets/jspdf.es.min-DNKSVJ2R.js","assets/index-JIy7qbXG.js","assets/useStore-C3Xgsdq2.js","assets/react-CoHAqsLe.js","assets/with-selector-BA01i0Uz.js","assets/clsx-CjueKrWZ.js","assets/createLucideIcon-D3tpt5UX.js","assets/jsx-runtime-sLPvdpSW.js","assets/bundle-mjs-_luiSGPg.js","assets/index-DVxR0POk.css","assets/slicedToArray-CJym0LHi.js"])))=>i.map(i=>d[i]);
import{t as e}from"./createLucideIcon-D3tpt5UX.js";import{f as t}from"./index-JIy7qbXG.js";var n=e(`file-spreadsheet`,[[`path`,{d:`M6 22a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h8a2.4 2.4 0 0 1 1.704.706l3.588 3.588A2.4 2.4 0 0 1 20 8v12a2 2 0 0 1-2 2z`,key:`1oefj6`}],[`path`,{d:`M14 2v5a1 1 0 0 0 1 1h5`,key:`wfsgrz`}],[`path`,{d:`M8 13h2`,key:`yr2amv`}],[`path`,{d:`M14 13h2`,key:`un5t4a`}],[`path`,{d:`M8 17h2`,key:`2yhykz`}],[`path`,{d:`M14 17h2`,key:`10kma7`}]]),r=`/yes-panchi-logo.png`;function i(){let e=new Date;return`${e.getFullYear()}-${String(e.getMonth()+1).padStart(2,`0`)}-${String(e.getDate()).padStart(2,`0`)}`}function a(e,t){let n=URL.createObjectURL(e),r=document.createElement(`a`);r.href=n,r.download=t,document.body.appendChild(r),r.click(),r.remove(),setTimeout(()=>URL.revokeObjectURL(n),0)}function o(e){return String(e??``).replace(/&/g,`&amp;`).replace(/</g,`&lt;`).replace(/>/g,`&gt;`).replace(/"/g,`&quot;`)}async function s(e=r){try{let t=await fetch(e);if(!t.ok)return null;let n=await t.blob();return await new Promise(e=>{let t=new FileReader;t.onload=()=>{let n=String(t.result||``);if(!n){e(null);return}if(typeof Image<`u`){let t=new Image;t.onload=()=>{e({dataUrl:n,width:t.naturalWidth||1024,height:t.naturalHeight||334})},t.onerror=()=>e({dataUrl:n,width:1024,height:334}),t.src=n}else e({dataUrl:n,width:1024,height:334})},t.onerror=()=>e(null),t.readAsDataURL(n)})}catch{return null}}function c(e,t){let n=e[t];return n==null?``:String(n)}function l(e){let{title:t,subtitle:n,columns:r,rows:i,logoAsset:a}=e,s=r.map(e=>`<th style="border:1px solid #d1d5db;padding:6pt 8pt;background:#f3f4f6;text-align:left;font-size:9pt;font-weight:bold;color:#1f2937;">${o(e.header)}</th>`).join(``),l=i.map((e,t)=>{let n=t%2==1?`background:#f9fafb;`:`background:#ffffff;`;return`<tr>${r.map(t=>`<td style="border:1px solid #e5e7eb;padding:5pt 7pt;font-size:8.5pt;vertical-align:top;color:#111827;${n}">${o(c(e,t.key))}</td>`).join(``)}</tr>`}).join(``),u=``;if(a){let e=(a.width||1)/(a.height||1),t=Math.min(160,Math.max(70,Math.round(32*e)));u=`<p style="margin:0 0 10pt 0;"><img src="${a.dataUrl}" alt="Logo" width="${t}" height="32" style="width:${t}px;height:32px;max-width:${t}px;max-height:32px;display:block;margin:0 0 8pt 0;" /></p>`}return`${u}
<h1 style="font-size:16pt;margin:0 0 4pt 0;color:#111827;font-family:Calibri,'Segoe UI',Arial,sans-serif;">${o(t)}</h1>
${n?`<p class="subtitle" style="font-size:9.5pt;color:#4b5563;margin:0 0 6pt 0;font-family:Calibri,'Segoe UI',Arial,sans-serif;">${o(n)}</p>`:``}
<p class="meta" style="font-size:8.5pt;color:#6b7280;margin:0 0 12pt 0;font-family:Calibri,'Segoe UI',Arial,sans-serif;">Exported ${new Date().toLocaleString()} · ${i.length} row(s)</p>
<table style="border-collapse:collapse;width:100%;mso-table-layout-alt:fixed;">
<thead><tr>${s}</tr></thead>
<tbody>${l||`<tr><td colspan="${r.length}" style="padding:8pt;color:#6b7280;">No data</td></tr>`}</tbody>
</table>`}async function u(e){let n=await t(()=>import(`./xlsx-JZtJaz5q.js`).then(e=>e.i),__vite__mapDeps([0,1])),r=n.default??n,o=[e.columns.map(e=>e.header),...e.rows.map(t=>e.columns.map(e=>c(t,e.key)))],s=r.utils.aoa_to_sheet(o),l=r.utils.book_new();r.utils.book_append_sheet(l,s,(e.sheetName||`Export`).slice(0,31));let u=r.write(l,{bookType:`xlsx`,type:`array`});a(new Blob([u],{type:`application/vnd.openxmlformats-officedocument.spreadsheetml.sheet`}),`${e.filename}-${i()}.xlsx`)}async function d(e){let t=await s(e.logoSrc||r),n=l({title:e.title,subtitle:e.subtitle,columns:e.columns,rows:e.rows,logoAsset:t}),c=`\uFEFF<html xmlns:o="urn:schemas-microsoft-com:office:office"
 xmlns:w="urn:schemas-microsoft-com:office:word"
 xmlns="http://www.w3.org/TR/REC-html40">
<head>
<meta charset="utf-8">
<title>${o(e.title)}</title>
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
</html>`;a(new Blob([c],{type:`application/msword;charset=utf-8`}),`${e.filename}-${i()}.doc`)}function f(e,t){let n=String(e??``).replace(/\s+/g,` `).trim();return n.length<=t?n:n.slice(0,Math.max(0,t-1))+`…`}async function p(e){let{jsPDF:n}=await t(async()=>{let{jsPDF:e}=await import(`./jspdf.es.min-DNKSVJ2R.js`);return{jsPDF:e}},__vite__mapDeps([2,1,3,4,5,6,7,8,9,10,11,12])),a=await s(e.logoSrc||r),o=new n({orientation:`landscape`,unit:`mm`,format:`a4`}),l=o.internal.pageSize.getWidth(),u=o.internal.pageSize.getHeight(),d=l-20,p=e.columns,m=Math.max(1,p.length),h=p.map(e=>{let t=e.key.toLowerCase();return t.includes(`pattern`)||t.includes(`prompt`)||t.includes(`description`)||t.includes(`policy`)?2.4:t.includes(`name`)||t.includes(`domain`)||t.includes(`platform`)||t.includes(`query`)?1.4:t.includes(`active`)||t.includes(`action`)||t.includes(`severity`)||t.includes(`type`)?.7:1}),g=h.reduce((e,t)=>e+t,0)||1,_=h.map(e=>e/g*d),v=_.map(e=>Math.max(12,Math.floor(e/1.35))),y=3.6,b=1.2,x=e=>{o.setFillColor(243,244,246),o.rect(10,e-3.2,d,5.7,`F`),o.setFont(`helvetica`,`bold`),o.setFontSize(8),o.setTextColor(30,30,30);let t=10;for(let n=0;n<m;n++){let r=f(p[n].header,v[n]);o.text(r,t+b,e),t+=_[n]}return o.setDrawColor(200,200,200),o.setLineWidth(.2),o.line(10,e+1.8,10+d,e+1.8),e+4.2+1.2},S=(e,t)=>{o.setFont(`helvetica`,`normal`),o.setFontSize(7),o.setTextColor(140,140,140),o.text(`Page ${e} · ${t}`,10,u-4),o.text(`Powered by UnifAI`,l-10,u-4,{align:`right`})},C=10;if(a)try{let e=(a.width||1)/(a.height||1),t=6.5,n=Math.min(26,Math.max(10,t*e));o.addImage(a.dataUrl,`PNG`,10,C-2,n,t),C+=8.5}catch{}o.setFont(`helvetica`,`bold`),o.setFontSize(14),o.setTextColor(17,17,17),o.text(e.title||`Export`,10,C),C+=6,o.setFont(`helvetica`,`normal`),o.setFontSize(9),o.setTextColor(100,100,100);let w=[e.subtitle,`Exported ${new Date().toLocaleString()}`,`${e.rows.length} row(s)`].filter(Boolean).join(` · `);o.text(w,10,C),C+=7,C=x(C);let T=1,E=`${e.rows.length} row(s)`;o.setFont(`helvetica`,`normal`),o.setFontSize(7.5),o.setTextColor(20,20,20);for(let t=0;t<e.rows.length;t++){let n=e.rows[t],r=p.map((e,t)=>f(c(n,e.key),v[t])).map((e,t)=>{let n=o.splitTextToSize(e||`—`,Math.max(8,_[t]-b*2));return Array.isArray(n)?n.slice(0,4):[String(n)]}),i=Math.max(1,...r.map(e=>e.length))*y+1.2;C+i>u-10-8&&(S(T,E),o.addPage(),T+=1,C=10,C=x(C),o.setFont(`helvetica`,`normal`),o.setFontSize(7.5),o.setTextColor(20,20,20)),t%2==1&&(o.setFillColor(249,250,251),o.rect(10,C-2.6,d,i,`F`));let a=10;for(let e=0;e<m;e++){let t=r[e];for(let e=0;e<t.length;e++)o.text(t[e],a+b,C+e*y);a+=_[e]}C+=i}S(T,E),o.save(`${e.filename}-${i()}.pdf`)}export{n as i,u as n,p as r,d as t};