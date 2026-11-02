<template>
  <q-layout class="bg-grey-1"><q-page-container><q-page class="q-pa-lg">
    <main class="workspace">
      <h1>PowerWhois 本地作业</h1>
      <p>Excel 导入、typo 生成和持久化扫描。not_found 表示注册局未找到记录，不保证可购买。</p>
      <q-banner v-if="error" class="bg-red-1 text-negative q-mb-md" role="alert">{{ error }}</q-banner>
      <q-card flat bordered class="q-pa-md q-mb-md">
        <h2>1. 导入原始域名</h2>
        <input type="file" accept=".xlsx" aria-label="XLSX 文件" @change="file = $event.target.files[0]" />
        <div class="row q-gutter-md q-mt-sm">
          <q-input v-model="sheet" label="工作表（默认第一张）" dense outlined />
          <q-input v-model="column" label="域名列" dense outlined style="width:100px" />
          <q-checkbox v-model="header" label="首行是标题" />
          <q-btn color="primary" label="导入 XLSX" :disable="!file || busy" @click="importFile" />
        </div>
      </q-card>
      <div class="row q-gutter-md q-mb-md items-center">
        <q-input v-model="jobID" label="作业 ID" dense outlined style="width:350px" />
        <q-btn label="查看作业" :disable="busy" @click="load" />
        <q-btn label="列出作业" :disable="busy" @click="list" />
      </div>
      <q-card v-if="job" flat bordered class="q-pa-md q-mb-md">
        <h2>2. 预览后启动扫描</h2>
        <p>状态：{{ job.state }}，输入 {{ job.inputRows }} 行，有效唯一 seed {{ job.uniqueSeeds }}，无效 {{ job.invalidRows }} 行。</p>
        <p>原始候选 {{ job.rawCandidates }}，合法候选 {{ job.validCandidates }}，去重扫描域名 {{ job.candidates }}，来源关联 {{ job.origins }}。</p>
        <p>已检查 {{ job.completed }} / {{ job.candidates }}。{{ job.partial ? '部分结果' : '结果完整' }}{{ job.truncated ? '；候选数量上限已到达' : '' }}</p>
        <p v-if="job.error" class="text-negative">{{ job.error }}</p>
        <div class="row q-gutter-md q-mb-md">
          <q-select v-model="rules" :options="ruleOptions" multiple outlined dense label="Typo 规则" style="min-width:320px" />
          <q-input v-model.number="cap" type="number" min="1" max="2000000" outlined dense label="唯一候选上限" />
          <q-select v-model="denominator" :options="['all','known']" outlined dense label="not_found 比例分母" />
        </div>
        <div class="row q-gutter-sm">
          <q-btn v-for="action in actions" :key="action" :label="action" :disable="busy" @click="act(action)" />
          <q-btn label="候选与结果（分页）" :disable="busy" @click="results('')" />
          <q-btn label="来源行与无效输入" :disable="busy" @click="seeds" />
          <q-btn label="导出 XLSX" color="primary" :href="exportURL" />
        </div>
      </q-card>
      <q-linear-progress v-if="busy" indeterminate class="q-mb-md" aria-label="正在处理" />
      <q-markup-table v-if="items.length" flat bordered wrap-cells>
        <thead><tr><th v-for="key in keys" :key="key" class="text-left">{{ labels[key] || key }}</th></tr></thead>
        <tbody><tr v-for="(item,index) in items" :key="index"><td v-for="key in keys" :key="key"><q-btn v-if="key === 'id'" flat dense no-caps :label="item.id" @click="selectJob(item.id)" /><span v-else>{{ display(item[key]) }}</span></td></tr></tbody>
      </q-markup-table>
      <q-btn v-if="next" label="下一页" class="q-mt-md" :disable="busy" @click="page(pageKind, next)" />
    </main>
  </q-page></q-page-container></q-layout>
</template>
<script setup>
import { ref, computed, onMounted } from 'vue';
const file=ref(null), sheet=ref(''), column=ref('B'), header=ref(true), jobID=ref(''), job=ref(null), busy=ref(false), error=ref(''), items=ref([]), next=ref('');
const rules=ref(['omission','repetition','transposition']), ruleOptions=['omission','repetition','transposition','insertion','horizontal','vertical','www','tld'], cap=ref(500000), denominator=ref('all');
const actions=['generate','start','pause','resume','cancel'];
const pageKind=ref('jobs');
const labels={id:'作业 ID',state:'状态',inputRows:'输入行数',candidates:'唯一扫描数',completed:'已检查',counts:'结果统计',domain:'域名',status:'查询状态',checkedAt:'检查时间 UTC',httpStatus:'HTTP 状态',error:'错误',sheet:'工作表',row:'来源行',raw:'原始值',duplicate:'重复来源'};
const keys=computed(()=>items.value.length?Object.keys(items.value[0]):[]);
const exportURL=computed(()=>`/api/v1/jobs/${jobID.value}/export?denominator=${denominator.value}`);
const display=value=>typeof value==='object'?JSON.stringify(value):String(value ?? '');
async function request(path,options){const r=await fetch('/api/v1/'+path,options);const d=await r.json();if(!r.ok)throw new Error(d.error || r.statusText);return d;}
async function run(fn){busy.value=true;error.value='';try{await fn();}catch(e){error.value=e.message;}finally{busy.value=false;}}
async function page(kind,cursor='') { await run(async()=>{ const path=kind==='jobs'?'jobs':`jobs/${jobID.value}/${kind}`; const d=await request(path+'?limit=100&after='+encodeURIComponent(cursor)); pageKind.value=kind; next.value=d.next; if(kind==='jobs'){items.value=d.items.map(({id,state,inputRows,candidates,completed,counts})=>({id,state,inputRows,candidates,completed,counts}));}else if(kind==='items'){items.value=d.items.map(({domain,result})=>({domain,status:result?.status ?? 'pending',checkedAt:result?.checkedAt ?? '',httpStatus:result?.httpStatus ?? '',error:result?.error ?? ''}));}else{items.value=d.items;} if(kind!=='jobs')job.value=await request('jobs/'+jobID.value); }); }
async function load(){await run(async()=>{job.value=await request('jobs/'+jobID.value);items.value=[];next.value='';});}
async function selectJob(id){jobID.value=id;await load();await page('items');}
async function list(){await page('jobs');}
async function importFile(){await run(async()=>{const form=new FormData();form.append('file',file.value);form.append('sheet',sheet.value);form.append('column',column.value);form.append('header',String(header.value));job.value=await request('jobs/import',{method:'POST',body:form});jobID.value=job.value.id;items.value=[];next.value='';});}
async function act(action){await run(async()=>{job.value=await request('jobs/'+jobID.value+'/'+action,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(action==='generate'?{rules:rules.value,maxCandidates:cap.value}:{})});});}
async function results(cursor){await page('items',cursor);}
async function seeds(){await page('seeds');}
onMounted(list);
</script>
<style scoped>
.workspace{max-width:1200px;margin:auto;color:#182c43}h1{font-size:30px;line-height:1.3;margin:8px 0 16px}h2{font-size:19px;line-height:1.4;margin:4px 0 16px}td{max-width:500px;word-break:break-word}p{line-height:1.6}
</style>