// Local browser acceptance of the real production bundle. All APIs are fixtures;
// no test mutates a live Team, workspace or model conversation.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import http from 'node:http';
import { chromium } from 'playwright';

const dist = process.env.TEAM_WORKSPACE_DIST || path.resolve(import.meta.dirname, '../frontend/dist');
const server = http.createServer((req, res) => {
  let file = path.join(dist, decodeURIComponent(new URL(req.url, 'http://local').pathname));
  if (!file.startsWith(dist + path.sep)) { res.writeHead(403).end(); return; }
  if (!fs.existsSync(file) || fs.statSync(file).isDirectory()) file = path.join(dist, 'index.html');
  res.setHeader('Content-Type', ({ '.js': 'text/javascript', '.css': 'text/css', '.html': 'text/html', '.png': 'image/png', '.svg': 'image/svg+xml' })[path.extname(file)] || 'application/octet-stream');
  fs.createReadStream(file).pipe(res);
});
await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
const origin = `http://127.0.0.1:${server.address().port}`;
const browser = await chromium.launch({ headless: true, channel: process.env.PLAYWRIGHT_CHANNEL || 'chrome' });
const page = await browser.newPage({ viewport: { width: 1920, height: 1080 } });
const errors = [];
page.on('pageerror', error => errors.push(String(error)));
await page.addInitScript(() => { localStorage.setItem('access_token', 'local-fixture-only'); localStorage.setItem('language', 'zh-CN'); });
const now = new Date().toISOString();
const members = ['leader', 'developer', 'reviewer'].map((key, i) => ({ id: i + 1, team_id: 144, user_id: 1, instance_id: 477 + i, member_key: key, display_name: key, role: key, runtime_type: i ? 'hermes' : 'openclaw', instance_mode: 'lite', status: i === 1 ? 'busy' : 'idle', availability: i === 1 ? 'busy' : 'idle', progress: 0, description: '成员职责与完整说明', last_seen_at: now, created_at: now, updated_at: now }));
const tasks = Array.from({ length: 8 }, (_, i) => ({ id: 300 + i, team_id: 144, target_member_id: 1, created_by: 1, message_id: `team-144-task-${300 + i}`, status: i === 7 ? 'running' : 'succeeded', workflow_state: i === 7 ? 'executing' : 'completed', payload: { title: `测试问题 ${i + 1}`, prompt: `测试问题 ${i + 1}：请协作开发并验收。\n${'长消息用于验证滚动定位和自动换行。'.repeat(20)}` }, result: i === 7 ? {} : { result: `已完成第 ${i + 1} 次任务。 /team/report.md` }, created_at: new Date(Date.now() - (8 - i) * 60000).toISOString(), updated_at: now }));
const data = { team: { id: 144, user_id: 1, name: 'workspace-acceptance', status: 'running', communication_mode: 'leader_mediated', shared_mount_path: '/team', created_at: now, updated_at: now }, members, leader_member_id: 'leader', tasks, events: [], work_items: [{ id: 1, team_id: 144, root_task_id: 307, work_id: 'build', owner_member_id: 2, title: '实现项目看板', status: 'running', required_for_root: true, created_at: now, updated_at: now, result: { summary: '详细交付说明 /team/report.md' } }] };
data.team.name = 'delivery-team';
tasks[6].payload = { title: '确认团队分工与交付标准', prompt: '请先明确成员职责、协作顺序和验收标准。开发与独立评审分开执行，最终交付由 Leader 汇总。' };
tasks[7].payload = { title: '个人项目看板 · 设计与交付', prompt: '请团队制作一个个人项目看板，参考 Trello 的操作体验。\n支持卡片拖拽、优先级标签与本地保存。请先制定计划，再由开发和评审分别完成实现与验收，最后交付页面和说明。' };
tasks[6].result = { result: '## 实施方案已确认\n本轮由 Developer 负责页面实现，Reviewer 负责独立验证。\n### 协作分工\n- Developer：实现卡片移动、标签管理和本地保存，提交自测结果。\n- Reviewer：核对产物、验证交互，记录测试边界与遗留项。\n### 交付约定\n1. 开发结果提交后，再开始独立评审。\n2. 静态检查和浏览器验证分别记录，不混为全部通过。\n3. 最终由 Leader 汇总交付与风险。\n方案说明：/team/report.md' };
const rosterTable = '\n\n| 成员 ID | 显示名称 | 角色 | Runtime | Mode | 状态 | 职责 |\n| --- | --- | --- | --- | --- | --- | --- |\n' +
  members.map(m => `| ${m.member_key} | 团队${m.role} | ${m.role} | ${m.runtime_type} | lite | idle/idle | ${'职责说明：负责协作分工、数据分析与交付验收，保留原始记录以及安全边界。'.repeat(16)} /team/report.md |`).join('\n');
tasks[6].result.result += rosterTable + '\n\n| 项目 | 说明 |\n| --- | --- |\n| 简短表格 | 不应折叠 |\n\n' +
  '| 单行长表格 | 内容 |\n| --- | --- |\n| 详细报告 | ' + '这是只有一行但内容非常长的表格，应该按实际高度折叠。'.repeat(100) + ' |';
data.work_items.push(
  { ...data.work_items[0], id: 2, work_id: 'requirements-review', owner_member_id: 3, title: '核对需求与验收清单', status: 'succeeded', result: { summary: '需求与测试清单已确认。 /team/report.md' } },
  { ...data.work_items[0], id: 3, work_id: 'leader-final-synthesis', owner_member_id: 1, title: '汇总交付与风险', status: 'pending', required_for_root: false, result: {} },
);
data.events.push({ id: 1, team_id: 144, task_id: 307, member_id: 1, event_type: 'outbound', sequence_no: 1, created_at: now, payload: { from: 'leader', to: 'developer', message: '开始实现项目看板', sent: true } });
const calls = [];
let failRefresh = false;
await page.route('**/api/v1/**', async route => {
  const request = route.request(), url = new URL(request.url()); calls.push([request.method(), url.pathname, url.search]);
  const send = value => route.fulfill({ json: { data: value } });
  if (url.pathname.endsWith('/auth/me')) return send({ id: 1, username: 'admin', role: 'user', status: 'active' });
  if (url.pathname.endsWith('/version')) return send({ version: 'ui-acceptance' });
  if (url.pathname.endsWith('/workspace/preview')) return send({ path: 'report.md', name: 'report.md', content: '# 验收报告\n可点击链接仍可打开预览。\n' + '## 验收章节\n预览长文档正文，确保标题与操作按钮始终可见。\n'.repeat(35) });
  if (url.pathname.endsWith('/workspace/download')) return route.fulfill({ body: 'test download', contentType: 'application/octet-stream' });
  if (url.pathname.endsWith('/workspace/files') && request.method() === 'GET') return send({ entries: url.searchParams.get('path') ? [{ name: 'report.md', path: 'artifacts/report.md', type: 'file', previewable: true, size: 123, modified_at: now }] : [{ name: 'artifacts', path: 'artifacts', type: 'directory', size: 0, modified_at: now }, { name: 'report.md', path: 'report.md', type: 'file', previewable: true, size: 123, modified_at: now }] });
  if (url.pathname === '/api/v1/teams/144') return failRefresh ? route.fulfill({ status: 503, json: { error: 'fixture transient failure' } }) : send(data);
  return send({});
});
const workspace = () => page.locator('.team-chat-workspace:visible');
const shot = async name => { await page.screenshot({ path: path.resolve(import.meta.dirname, `reports/team-workspace-${name}.png`), fullPage: true }); };
try {
  await page.goto(`${origin}/teams/144`);
  await workspace().waitFor();
  await page.waitForFunction(() => document.querySelector('.team-chat-workspace[data-mode=dual]'));
  assert.equal(await workspace().getAttribute('data-right'), 'closed');
  assert.equal(calls.filter(c => c[1].endsWith('/workspace/files')).length, 0, 'collapsed files must not fetch');
  await shot('collapsed');
  assert.equal(await workspace().locator('.team-history-popover').isVisible(), false);
  await workspace().locator('.team-history-menu summary').click();
  await workspace().locator('.team-history-options button').first().waitFor();
  assert.ok(await workspace().locator('.team-history-popover').evaluate(el => {
    const r = el.getBoundingClientRect();
    return el.contains(document.elementFromPoint(r.right - 8, r.bottom - 8));
  }), 'history popover must remain above floating workflow');
  await shot('history');
  await workspace().locator('.team-history-options button').nth(1).click();
  assert.equal(await workspace().locator('.team-history-popover').isVisible(), false);
  const roster = workspace().locator('.team-collapsible-table').filter({ hasText: '成员 ID' });
  await roster.getByRole('button', { name: '显示全部', exact: true }).waitFor();
  assert.equal(await roster.getAttribute('data-collapsed'), 'true');
  assert.ok((await roster.boundingBox()).height < 390, 'long table should not occupy the whole screen');
  assert.ok(await roster.locator('th').evaluateAll(cells => cells.every(cell => cell.getBoundingClientRect().width >= 95)), 'metadata columns must retain readable widths');
  await roster.scrollIntoViewIfNeeded();
  await shot('table-collapsed');
  await roster.getByRole('button', { name: '显示全部', exact: true }).click();
  assert.equal(await roster.getAttribute('data-collapsed'), 'false');
  assert.ok((await roster.boundingBox()).height > 500);
  await shot('table-expanded');
  await roster.getByRole('button', { name: '收起', exact: true }).click();
  await page.waitForTimeout(100);
  assert.equal(await roster.getAttribute('data-collapsed'), 'true');
  assert.ok(await roster.evaluate(el => {
    const feed = el.closest('.team-chat-feed').getBoundingClientRect(), rect = el.getBoundingClientRect();
    return rect.bottom > feed.top && rect.top < feed.bottom;
  }), 'collapse keeps the table in the reader viewport');
  const shortTable = workspace().locator('.team-collapsible-table').filter({ hasText: '不应折叠' });
  assert.equal(await shortTable.getByRole('button').count(), 0, 'short table must not gain a redundant toggle');
  const singleRowTable = workspace().locator('.team-collapsible-table').filter({ hasText: '单行长表格' });
  assert.equal(await singleRowTable.getAttribute('data-collapsed'), 'true', 'even one tall row must collapse');
  await roster.getByRole('button', { name: '显示全部', exact: true }).click();
  await roster.getByRole('button', { name: '/team/report.md', exact: true }).first().click();
  await page.locator('[data-workspace-preview]:visible').waitFor();
  assert.equal(await page.locator('[data-workspace-preview]:visible').count(), 1, 'responsive layout must not create duplicate body portals');
  await page.keyboard.press('Escape');
  await roster.getByRole('button', { name: '收起', exact: true }).click();
  await workspace().locator('.team-history-menu summary').click();
  await workspace().locator('.team-history-options button').first().click();
  await workspace().locator('.team-history-menu summary').click();
  await page.keyboard.press('Escape');
  assert.equal(await workspace().locator('.team-history-popover').isVisible(), false);
  const workflowNode = workspace().locator('.team-workflow-node').filter({ hasText: 'developer' }).first();
  await workflowNode.hover();
  await page.getByRole('tooltip').waitFor();
  assert.match(await page.getByRole('tooltip').innerText(), /实现项目看板/);
  await shot('workflow-hover');
  await page.emulateMedia({ reducedMotion: 'reduce' });
  assert.equal(await workspace().locator('li[data-active=true] .team-workflow-marker svg').first().evaluate(el => getComputedStyle(el).animationName), 'none');
  await page.emulateMedia({ reducedMotion: 'no-preference' });
  await workflowNode.click();
  assert.equal(await workspace().getAttribute('data-right'), 'kanban');
  await workspace().getByRole('button', { name: '收起右侧面板' }).click();
  await workspace().locator('.team-tools-toolbar').getByRole('button', { name: '文件', exact: true }).click();
  assert.equal(await workspace().getAttribute('data-right'), 'files');
  await workspace().getByRole('button', { name: '收起右侧面板' }).click();
  const feed = workspace().locator('.team-chat-feed');
  await feed.evaluate(el => { el.scrollTop = el.scrollHeight / 2; el.dispatchEvent(new Event('scroll')); });
  await page.waitForTimeout(150);
  const anchor = await feed.evaluate(el => { const top = el.getBoundingClientRect().top; const n = [...el.querySelectorAll('[data-chat-message]')].find(n => n.getBoundingClientRect().bottom > top + 1); return { id: n.dataset.chatMessage, offset: n.getBoundingClientRect().top - top }; });
  await workspace().getByRole('button', { name: '展开成员与团队配置' }).click();
  await workspace().getByRole('button', { name: '展开任务看板', exact: true }).click();
  assert.equal(await workspace().getAttribute('data-left'), 'true');
  assert.equal(await workspace().getAttribute('data-right'), 'kanban');
  assert.ok(await workspace().locator('.team-chat-main').evaluate(el => el.clientWidth) >= 560);
  await page.waitForTimeout(200);
  const offset = await feed.evaluate((el, id) => [...el.querySelectorAll('[data-chat-message]')].find(n => n.dataset.chatMessage === id).getBoundingClientRect().top - el.getBoundingClientRect().top, anchor.id);
  assert.ok(Math.abs(offset - anchor.offset) < 8, `reading anchor moved ${offset - anchor.offset}px`);
  await feed.evaluate(el => { el.scrollTop = el.scrollHeight; el.dispatchEvent(new Event('scroll')); });
  await shot('both');
  await workspace().locator('.team-board-columns').getByRole('button', { name: /实现项目看板/ }).click();
  await workspace().getByRole('button', { name: '← 返回看板' }).waitFor();
  await workspace().locator('.team-kanban-host').getByRole('button', { name: '/team/report.md', exact: true }).click();
  await page.locator('[data-workspace-preview]:visible').waitFor();
  await page.keyboard.press('Escape');
  await shot('card-detail');
  await workspace().getByRole('button', { name: '← 返回看板' }).click();
  await workspace().locator('textarea').fill('保留未发送的草稿');
  await workspace().locator('.team-tools-toolbar').getByRole('button', { name: '文件', exact: true }).click();
  assert.equal(await workspace().locator('.team-kanban-host').isVisible(), false);
  await workspace().getByRole('button', { name: 'artifacts', exact: true }).click();
  await workspace().getByRole('button', { name: 'report.md', exact: true }).waitFor();
  await workspace().locator('.team-tools-toolbar').getByRole('button', { name: '看板', exact: true }).click();
  await workspace().locator('.team-tools-toolbar').getByRole('button', { name: '文件', exact: true }).click();
  assert.ok((await workspace().locator('.team-files-host').innerText()).includes('artifacts'), 'folder must survive switching');
  assert.equal(await workspace().locator('textarea').inputValue(), '保留未发送的草稿');
  await workspace().getByRole('button', { name: 'report.md', exact: true }).click();
  await page.locator('[data-workspace-preview]:visible').waitFor();
  assert.ok(await page.locator('[data-workspace-preview]:visible').evaluate(modal => {
    const topbar = [...document.querySelectorAll('.team-chat-topbar')].find(el => el.getClientRects().length);
    const r = topbar.getBoundingClientRect();
    return modal.contains(document.elementFromPoint(innerWidth / 2, r.top + r.height / 2));
  }), 'file preview must cover the chat header, not sit behind its stacking context');
  const previewDialog = page.locator('[data-workspace-preview]:visible');
  assert.ok(await previewDialog.evaluate(el => el.matches(':modal')), 'preview must use the browser modal layer');
  await previewDialog.locator('.team-preview-body').evaluate(el => { el.scrollTop = el.scrollHeight; });
  assert.ok(await previewDialog.locator('.team-preview-header').evaluate(el => {
    const r = el.getBoundingClientRect();
    return r.top >= 0 && el.contains(document.elementFromPoint(r.left + 20, r.top + 20));
  }), 'preview title remains visible and hit-testable after scrolling');
  await shot('preview-layer');
  await previewDialog.getByRole('button', { name: '关闭', exact: true }).focus();
  await page.keyboard.press('Tab');
  assert.ok(await previewDialog.evaluate(el => el.contains(document.activeElement)), 'Tab stays within preview');
  await page.keyboard.press('Escape');
  assert.equal(await page.locator('[data-workspace-preview]:visible').count(), 0);
  assert.equal(await workspace().getAttribute('data-right'), 'files');
  await shot('files');
  for (const width of [1600, 1440, 1366, 1024, 768, 390]) {
    await page.setViewportSize({ width, height: 900 });
    await page.waitForTimeout(200);
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), `page overflows at ${width}`);
    if (await workspace().getAttribute('data-mode') === 'drawer') {
      await page.keyboard.press('Escape');
      await workspace().getByRole('button', { name: '展开任务看板', exact: true }).click();
      await workspace().getByRole('dialog').waitFor();
      await page.keyboard.press('Escape');
      assert.equal(await workspace().getAttribute('data-right'), 'closed');
    }
    await shot(String(width));
    if (width === 390) {
      await workspace().locator('.team-tools-toolbar').getByRole('button', { name: '文件', exact: true }).click();
      await workspace().getByRole('button', { name: 'report.md', exact: true }).click();
      const mobilePreview = page.locator('[data-workspace-preview]:visible');
      await mobilePreview.waitFor();
      assert.equal(await page.locator('[data-workspace-preview]:visible').count(), 1);
      assert.ok(await mobilePreview.evaluate(el => el.matches(':modal')));
      await mobilePreview.locator('.team-preview-body').evaluate(el => { el.scrollTop = el.scrollHeight; });
      await mobilePreview.getByRole('button', { name: '关闭', exact: true }).waitFor();
      await shot('preview-mobile');
      await page.keyboard.press('Escape');
      assert.equal(await workspace().getAttribute('data-right'), 'files', 'closing preview must not close underlying drawer');
      await page.keyboard.press('Escape');
    }
  }
  await page.setViewportSize({ width: 1920, height: 1080 });
  // New, successful transfer animates once; re-polling the same event does not
  // reset the effect. Terminal tasks must stop execution animation.
  data.events.push({ ...data.events[0], id: 2, sequence_no: 2, created_at: new Date().toISOString(), payload: { from: 'developer', to: 'reviewer', sent: true, messageDelivered: true } });
  await page.getByRole('button', { name: '刷新', exact: true }).filter({ visible: true }).first().click();
  await workspace().locator('.team-workflow-handoff[data-fresh=true]').waitFor();
  assert.match(await workspace().locator('.team-workflow-handoff').innerText(), /developer[\s\S]*reviewer/);
  tasks[7].status = 'succeeded'; tasks[7].workflow_state = 'completed'; tasks[7].updated_at = new Date(Date.now() + 1000).toISOString();
  await page.getByRole('button', { name: '刷新', exact: true }).filter({ visible: true }).first().click();
  await page.waitForFunction(() => [...document.querySelectorAll('.team-workflow-nodes li')].every(el => el.dataset.active !== 'true'));
  assert.match(await workspace().locator('.team-workflow-status').innerText(), /已完成/);
  await workspace().getByRole('button', { name: '展开任务看板', exact: true }).click();
  failRefresh = true;
  await page.getByRole('button', { name: '刷新', exact: true }).filter({ visible: true }).first().click();
  await page.getByRole('alert').filter({ visible: true }).waitFor();
  assert.equal(await workspace().locator('textarea').inputValue(), '保留未发送的草稿');
  assert.deepEqual(errors, []);
  console.log('PASS: floating workspace, table widths/collapse/links, native preview layering and mobile controls, reading anchor, panel state, transfer/terminal animation, responsive widths and transient refresh.');
} finally { await browser.close(); server.close(); }
