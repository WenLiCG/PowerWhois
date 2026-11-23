import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
import crypto from 'node:crypto';

const source=fs.readFileSync(new URL('../src/components/BulkCheck.vue',import.meta.url),'utf8');
const code=source.slice(source.indexOf('async function createTask()'),source.indexOf('async function control('));
function fixture(text) {
  const calls=[];
  const ctx={file:{value:{size:Buffer.byteLength(text),text:async()=>text}},creating:{value:false},error:{value:''},mode:{value:'dnsQuery'},concurrency:{value:15},timeout:{value:30},retries:{value:0},selectedID:{value:'prior-task'},submissionBody:null,submissionKey:null,crypto,loadSelected:async()=>{},showError:e=>{ctx.error.value=e.response.data.error.message},api:{post:async(path,body,options)=>{calls.push({path,body,options});return {data:{id:'created'}}}}};
  vm.createContext(ctx);vm.runInContext(code,ctx);return {ctx,calls};
}
test('upload retains first row, trims BOM/whitespace and defaults to 15',async()=>{
  assert.match(source,/concurrency = ref\(15\)/);
  const {ctx,calls}=fixture('\ufeffEXAMPLE.com\r\n\n example.org \n例子.中国\n');
  await ctx.createTask();
  assert.deepEqual(Array.from(calls[0].body.domains),['EXAMPLE.com','example.org','例子.中国']);
  assert.equal(calls[0].body.concurrency,15);assert.equal(ctx.selectedID.value,'created');assert.equal(ctx.file.value,null);
});
test('rejected upload retains input, previous task and idempotency key',async()=>{
  const {ctx,calls}=fixture('example.com\nbad name.com');
  ctx.api.post=async(path,body,options)=>{calls.push({body,options});throw {response:{data:{error:{message:'invalid domain at line 2'}}}}};
  await ctx.createTask();await ctx.createTask();
  assert.equal(ctx.selectedID.value,'prior-task');assert.ok(ctx.file.value);assert.equal(ctx.creating.value,false);
  assert.equal(ctx.error.value,'invalid domain at line 2');assert.equal(calls[0].options.headers['Idempotency-Key'],calls[1].options.headers['Idempotency-Key']);
});
test('full attachment is parsed without truncation', {skip:!process.env.POWERWHOIS_ATTACHMENT_TXT},async()=>{
  const text=fs.readFileSync(process.env.POWERWHOIS_ATTACHMENT_TXT,'utf8');const {ctx,calls}=fixture(text);await ctx.createTask();
  const expected=text.replace(/^\uFEFF/,'').split(/\r?\n/).map(s=>s.trim()).filter(Boolean);
  assert.equal(calls[0].body.domains.length,563807);assert.deepEqual(Array.from(calls[0].body.domains),expected);
  assert.equal(calls[0].body.domains[0],'google.cf');assert.equal(calls[0].body.domains.at(-1),'southampton.ac.uk');
});
