import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';

const source = fs.readFileSync(new URL('../src/components/BulkCheck.vue', import.meta.url), 'utf8');
const code = source.slice(source.indexOf('async function exportResults()'), source.indexOf('onMounted('));

async function exportPages(pages) {
  const offsets = [], errors = [];
  let downloaded = false, blob;
  const ctx = {
    exporting: {value: false}, task: {value: {id: 'large-task', state: 'canceled'}},
    showError: error => errors.push(error), Blob, setTimeout: () => {},
    URL: {createObjectURL: value => {blob = value; return 'blob:test';}, revokeObjectURL() {}},
    document: {createElement: () => ({click: () => {downloaded = true;}})},
    api: {get: async (_, {params}) => {
      offsets.push(params.offset);
      assert.ok(pages.length, 'export scanned the unprocessed tail');
      return pages.shift();
    }}
  };
  vm.createContext(ctx); vm.runInContext(code, ctx); await ctx.exportResults();
  assert.deepEqual(errors, []); assert.ok(downloaded); assert.equal(ctx.exporting.value, false);
  return {offsets, csv: await blob.text()};
}
const page = (count, completed, next, state = 'canceled', data = 'Domain,Message\n') => ({
  data, headers: {'x-result-count': String(count), 'x-task-completed': String(completed),
    'x-next-offset': String(next), 'x-task-state': state}
});

test('partial export ends after existing results, including multiline CSV fields', async () => {
  const result = await exportPages([page(853, 853, 1000, 'canceled', 'Domain,Message\na.com,"line one\nline two"\n')]);
  assert.deepEqual(result.offsets, [0]); assert.match(result.csv, /line one\nline two/);
});

test('sparse out-of-order results are not skipped on an empty page', async () => {
  const result = await exportPages([page(1, 2, 1000), page(0, 2, 2000), page(1, 2, 3000)]);
  assert.deepEqual(result.offsets, [0, 1000, 2000]);
});

test('running tasks still follow the cursor after reaching the reported count', async () => {
  const result = await exportPages([page(1, 1, 1000, 'running'), page(1, 2, -1, 'running')]);
  assert.deepEqual(result.offsets, [0, 1000]);
});

test('older servers without count headers retain cursor pagination', async () => {
  const result = await exportPages([
    {data: 'Domain\na.com\n', headers: {'x-next-offset': '1000'}},
    {data: 'Domain\nb.com\n', headers: {'x-next-offset': '-1'}}
  ]);
  assert.deepEqual(result.offsets, [0, 1000]); assert.equal(result.csv, 'Domain\na.com\nb.com\n');
});
