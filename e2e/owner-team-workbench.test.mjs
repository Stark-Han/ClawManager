// Real React/Vite/browser; API fixtures exercise UI races and never call a model.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { chromium } from 'playwright';
import { createServer } from '../frontend/node_modules/vite/dist/node/index.js';

const frontend = path.resolve('../frontend'), temp = fs.mkdtempSync(path.join(os.tmpdir(), 'team-workbench-'));
let server, browser;
const spec = { members: Array.from({ length: 6 }, (_, i) => ({ memberId: `m${i}`, displayName: i ? `调研与资料分析成员${i}` : '团队负责人', role: i ? 'worker' : 'leader', isLeader: i === 0, summary: '负责可追溯的数据搜集与核对。', mission: '保持原始来源，按任务要求交付。', responsibilities: Array(8).fill('核实来源、时间、内容与任务目标的一致性。'), boundaries: ['保留原始引用'], expectedInputs: ['用户目标'], deliverables: ['报告'], acceptanceCriteria: ['可核对'], collaborationNotes: ['遵循 Leader 分工'] })) };
let templates = [1, 2].map(id => ({ id, name: `模板${id}`, intent: `目标${id}`, resolved_member_count: 6, revision: 1, spec }));
let mutations = 0, generationRequests = 0, failGeneration = false, holdRefresh = null;
try {
  const component = '/@fs/' + path.join(frontend, 'src/components/teams/OwnerTeamTemplates.tsx').replaceAll('\\', '/');
  const styles = '/@fs/' + path.join(frontend, 'src/components/teams/owner-team-workspace.css').replaceAll('\\', '/');
  fs.writeFileSync(path.join(temp, 'fixture.tsx'), `import React,{useState}from'react';import{createRoot}from'react-dom/client';import{OwnerTeamTemplates}from${JSON.stringify(component)};import${JSON.stringify(styles)};
function App(){const[visible,setVisible]=useState(true),[owner,setOwner]=useState('alice@example.com');return <><button onClick={()=>setVisible(v=>!v)}>切换标签</button><button onClick={()=>setOwner(v=>v==='alice@example.com'?'bob@example.com':'alice@example.com')}>切换所有者</button><main style={{height:650,maxWidth:1050,margin:'12px auto',display:'flex',minHeight:0}}><section className="ot-workspace">{visible&&<OwnerTeamTemplates key={owner} owner={owner} canGenerate onOperation={()=>{}}/>}</section></main></>};createRoot(document.getElementById('root')).render(<App/>);`);
  server = await createServer({ root: frontend, configFile: path.join(frontend, 'vite.config.ts'), server: { host: '127.0.0.1', port: 0, fs: { allow: [frontend, temp] } }, plugins: [{ name: 'owner-team-layout-test', configureServer(s) { s.middlewares.use('/__team_workbench__', async (req, res) => { res.setHeader('Content-Type', 'text/html'); res.end(await s.transformIndexHtml('/__team_workbench__', `<html><body style="margin:0"><div id="root"></div><script type="module" src="/@fs/${path.join(temp, 'fixture.tsx').replaceAll('\\', '/')}"></script></body></html>`)); }); } }] });
  await server.listen(); const address = server.httpServer.address();
  browser = await chromium.launch({ channel: 'chrome', headless: true });
  const page = await browser.newPage({ viewport: { width: 1200, height: 900 } }), errors = [];
  page.on('pageerror', e => errors.push(e.message));
  await page.route('**/api/v1/ieisystem/**', async route => {
    const request = route.request(), url = new URL(request.url()), method = request.method(), body = request.postDataJSON();
    const send = (data, status = 200) => route.fulfill({ status, json: { success: status < 400, data, message: status === 409 ? '模板版本冲突，请重新读取' : undefined } });
    if (url.pathname.endsWith('/custom-team-templates') && method === 'GET') {
      if (holdRefresh) await new Promise(resolve => { holdRefresh.release = resolve; });
      return send({ templates });
    }
    if (/\/team-operations\//.test(url.pathname)) return send({ operation_id: 'op-ui-fixture', status: failGeneration ? 'failed' : 'succeeded', template_id: 1, error_message: failGeneration ? '生成暂不可用，已保留模板' : undefined });
    const match = url.pathname.match(/\/custom-team-templates\/(\d+)/), t = templates.find(t => t.id === Number(match?.[1]));
    if (match && method === 'GET') return send(t);
    if (match && method === 'PATCH') { mutations++; if (body.expected_revision !== t.revision) return send(null, 409); Object.assign(t, { name: body.name, revision: t.revision + 1 }); return send(t); }
    if (match && method === 'POST') { generationRequests++; return send({ operation_id: 'op-ui-fixture', status: 'queued' }, 202); }
    if (match && method === 'PUT') { if (body.expected_revision !== t.revision) return send(null, 409); t.revision++; return send(t); }
    return send(null, 404);
  });
  holdRefresh = {};
  await page.goto(`http://127.0.0.1:${address.port}/__team_workbench__`);
  for(let i=0;i<40&&!holdRefresh.release;i++) await page.waitForTimeout(50);
  assert.equal(await page.getByRole('button',{name:'自定义团队模板',exact:true}).isDisabled(),true);
  assert.equal(await page.getByLabel('团队目标与分工',{exact:true}).isVisible(),false,'empty editor exposed before draft restore');
  const releaseInitial=holdRefresh.release; holdRefresh=null; releaseInitial();
  const name = page.getByLabel('模板名称', { exact: true }), intent = page.getByLabel('团队目标与分工', { exact: true });
  await name.waitFor(); await name.fill('保留名称草稿'); await intent.fill('保留目标草稿');
  await page.getByRole('button', { name: '刷新模板', exact: true }).click(); await page.getByText('模板列表已刷新，编辑内容已保留。').waitFor(); assert.equal(await name.inputValue(), '保留名称草稿');
  await page.getByRole('button', { name: '切换标签', exact: true }).click(); await page.getByRole('button', { name: '切换标签', exact: true }).click(); await name.waitFor(); assert.equal(await intent.inputValue(), '保留目标草稿');
  await page.getByRole('button', { name: '模板2', exact: false }).first().click(); await name.fill('另一个模板的草稿'); await page.getByRole('button', { name: /模板1.*6 人/ }).click(); assert.equal(await name.inputValue(), '保留名称草稿');
  assert.equal(await page.evaluate(() => window.dispatchEvent(new Event('clawmanager:before-page-refresh', { cancelable: true }))), false);
  await page.getByRole('button', { name: '仅保存名称', exact: true }).click(); await page.getByText('模板名称已保存。').waitFor(); assert.equal(mutations, 1); assert.equal(generationRequests, 0, 'rename must never generate');
  templates[0].revision++; await name.fill('冲突时保留的名称'); await page.getByRole('button', { name: '仅保存名称', exact: true }).click(); await page.getByRole('alert').waitFor(); assert.equal(await name.inputValue(), '冲突时保留的名称');
  await page.getByRole('button', { name: '读取最新版本并保留输入' }).click(); await page.getByText(/已读取最新版本/).waitFor(); assert.equal(await intent.inputValue(), '保留目标草稿');
  await page.locator('.ot-template-member summary').first().click(); await page.getByRole('button', { name: '调整职责', exact: true }).first().click(); const dialog = page.getByRole('dialog', { name: '调整成员职责' });
  await dialog.getByLabel('调整说明').fill('核实每条引用'); page.once('dialog', d => d.dismiss()); await page.keyboard.press('Escape'); assert.equal(await dialog.count(), 1); page.once('dialog', d => d.accept()); await page.keyboard.press('Escape'); await dialog.waitFor({ state: 'detached' });
  await page.locator('.ot-template-member').evaluateAll(elements => elements.forEach(e => { e.open = true; }));
  for (const width of [1920, 1440, 1024, 768, 390, 320]) {
    await page.setViewportSize({ width, height: 900 });
    const result = await page.locator('.ot-template-editor').evaluate(e => { const body = e.getBoundingClientRect(), footer = e.closest('.ot-template-area').querySelector('footer').getBoundingClientRect(), work = e.closest('.ot-workspace').getBoundingClientRect(); return { overflow: document.documentElement.scrollWidth > innerWidth + 1, scroll: e.scrollHeight > e.clientHeight, body: body.bottom, footer: footer.top, end: footer.bottom, outer: work.bottom }; });
    assert.equal(result.overflow, false, `overflow ${width}`); assert.equal(result.scroll, true, `missing internal scroll ${width}`); assert.ok(result.body <= result.footer + 1, `footer overlap ${width}`); assert.ok(result.end <= result.outer + 1, `footer escaped ${width}`);
  }
  await page.setViewportSize({ width: 1200, height: 900 }); failGeneration = true;
  await page.getByRole('button', { name: '按说明修订模板', exact: true }).dblclick(); await page.getByRole('alert').waitFor(); assert.equal(generationRequests, 1, 'double click created duplicate generation'); assert.equal(await name.inputValue(), '冲突时保留的名称');
  failGeneration = false; await page.getByRole('button', { name: '按说明修订模板', exact: true }).click(); await page.getByText('模板已更新，已创建的团队保持不变。').waitFor(); assert.equal(generationRequests, 2); assert.equal(await name.inputValue(), templates[0].name);
  await page.locator('.ot-template-member').first().getByRole('button', { name: '重新生成成员', exact: true }).click(); await page.getByText('模板已更新，已创建的团队保持不变。').waitFor(); assert.equal(generationRequests, 3);
  // Old responses must not replace a different owner's editor after unmount.
  holdRefresh = {}; await page.getByRole('button', { name: '刷新模板', exact: true }).click();
  await page.waitForTimeout(50); const release = holdRefresh.release; holdRefresh = null;
  await page.getByRole('button', { name: '切换所有者', exact: true }).click(); await name.waitFor(); if (release) release(); await page.waitForTimeout(80); assert.equal(await name.inputValue(), templates[0].name, 'foreign-owner draft leaked');
  assert.deepEqual(errors, []); console.log('PASS actual Team template UI: scoped drafts, rename without model, CAS conflict/input retention, cancel guard, six widths/internal scroll, duplicate-submit fence, failed generation retention, owner/unmount fencing. API/model boundary simulated.');
} finally { await browser?.close(); await server?.close(); assert.equal(path.dirname(path.resolve(temp)), path.resolve(os.tmpdir())); assert.ok(path.basename(temp).startsWith('team-workbench-')); fs.rmSync(temp, { recursive: true, force: true }); }
