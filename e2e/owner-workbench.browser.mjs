// Targeted browser regression against the real application through a dev server
// or downloaded production bundle. Every API/native page below is a fixture.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { chromium } from 'playwright';

const origin = process.env.OWNER_WORKBENCH_ORIGIN || 'http://127.0.0.1:9178';
const screenshots = path.resolve(import.meta.dirname, 'reports/owner-workbench');
fs.mkdirSync(screenshots, {recursive:true});
const browser = await chromium.launch({channel:'chrome',headless:true});
const page = await browser.newPage({viewport:{width:1440,height:1000}});
page.setDefaultTimeout(45000);
const errors=[], calls=[], sessions=new Map();
page.on('pageerror',e=>errors.push(e.message));
await page.addInitScript(()=>{localStorage.setItem('clawmanager_locale','zh');localStorage.setItem('access_token','cm-fixture-never-send-to-owner');});
await page.routeWebSocket(/\/api\/v1\/ws/,()=>{});
await page.route('**/fixture-agent**',route=>route.fulfill({contentType:'text/html',body:'<p>Fixture native instance</p>'}));
await page.route('**/fixture-preview**',route=>route.fulfill({contentType:'text/html',body:'<h1>Fixture local web service</h1>'}));
const owner='portal-accept-20260923@example.com',now=new Date().toISOString();
const members=['openclaw','hermes','opencode','deepseek-harness'].map((runtime,i)=>({id:i+1,team_id:184,instance_id:616+i,user_id:1,member_key:i?'worker-'+i:'leader',display_name:i?'成员 '+i:'团队负责人',role:i?'worker':'leader',runtime_type:runtime,instance_mode:'lite',status:'idle',availability:'idle',runtime_status:'idle',progress:0,created_at:now,updated_at:now}));
const instances = new Map([
 [609,{id:609,name:'Hermes long instance name to verify narrow layouts',type:'hermes'}],
 [590,{id:590,name:'DeepSeek personal instance',type:'deepseek-harness'}],
 ...members.map(m=>[m.instance_id,{id:m.instance_id,name:m.display_name,type:m.runtime_type,team_id:184}]),
].map(([id,i])=>[id,{...i,owner,runtime_type:'gateway',runtime_generation:3,instance_mode:'lite',status:'running',workspace_available:true,workspace_root:'Workspace',created_at:now,updated_at:now}]));
const team={id:184,user_id:1,name:'四种实例协作团队',status:'running',communication_mode:'leader_mediated',shared_mount_path:'/team',created_at:now,updated_at:now};
const detail={team,members,leader_member_id:'leader',tasks:[{id:402,team_id:184,target_member_id:1,message_id:'task-402',status:'succeeded',payload:{title:'论文精读',prompt:'只精读两篇论文'},result:{result:'笔记见 /team/report.md'},created_at:now,updated_at:now}],events:[],work_items:[]};
await page.route('**/api/v1/**',async route=>{
 const r=route.request(),u=new URL(r.url()),p=u.pathname;calls.push({method:r.method(),path:p,query:u.search,headers:r.headers(),body:r.postData()});
 const send=data=>route.fulfill({json:{success:true,data}});
 if(p.endsWith('/auth/me'))return send({id:1,username:'admin',role:'admin',status:'active'});
 if(p.endsWith('/version'))return send({version:'fixture'});
 if(p.endsWith('/ieisystem/session')||p.endsWith('/session/refresh'))return send({owner,expires_at:new Date(Date.now()+3600000).toISOString()});
 if(p==='/api/v1/ieisystem/instances'){const index=Number(u.searchParams.get('page')||1);return send({instances:index===1?[instances.get(609)]:[instances.get(590)],total:2,page:index,limit:1});}
 if(p.endsWith('/teams')&&r.method()==='GET')return send({teams:[{...team,members,member_count:4,available_count:4}],total:1,page:1,limit:100});
 if(p.endsWith('/teams/184'))return send(detail);
 if(p.endsWith('/teams/184/tasks'))return send({tasks:detail.tasks,has_more:false});
 if(p.endsWith('/teams/184/events'))return send({events:[],has_more:false});
 if(p.endsWith('/workspace/files'))return send({entries:[{name:'report.md',path:'report.md',type:'file',is_dir:false,previewable:true,size:123,modified_at:now}]});
 if(p.endsWith('/workspace/preview'))return send({preview:{kind:'text',text:'# Notebook',path:'report.md'},kind:'text',text:'# Notebook',path:'report.md'});
 const m=/\/instances\/(\d+)(.*)$/.exec(p);
 if(m){const id=Number(m[1]),suffix=m[2];
  if(!suffix)return send({instance:instances.get(id)});
  if(suffix==='/access')return send({access_url:origin+'/fixture-agent/'+id,expires_at:new Date(Date.now()+3600000).toISOString(),workspace_available:true,workspace_root:'Workspace'});
  if(suffix==='/workbench/capabilities')return send({generation:3,workspace:'/workspace',git:false,terminal:true,preview:true,processes:false});
  if(suffix==='/workbench/terminals'){
   if(r.method()==='POST'){const value={id:'session_'+id+'_'+(sessions.size+1),name:'Terminal '+id,generation:3,cwd:'/workspace',exited:false,created_at:now};sessions.set(value.id,{...value,instance:id});return send(value);}
   return send({sessions:[...sessions.values()].filter(s=>s.instance===id)});
  }
  if(suffix.includes('/workbench/terminals/')&&suffix.endsWith('/output'))return send({data:Buffer.from('workspace ready\r\n$ ').toString('base64'),cursor:1,truncated:false,exited:false});
  if(suffix.includes('/workbench/terminals/'))return send({});
  if(suffix==='/workbench/previews')return send({url:origin+'/fixture-preview/'+id,generation:3});
 }
 return send({});
});
const root=()=>page.locator('.owner-instance-workbench .wb-root');
const toolPane=()=>page.locator('.team-workspace-tools');
const checkOverflow=async()=>assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'page overflows');
try{
 await page.goto(origin+'/ieisystem/instances/609');
 await root().getByRole('tab',{name:'终端',exact:true}).click();
 await root().getByRole('button',{name:'新建终端',exact:true}).click();
 await root().locator('.xterm').waitFor();
 assert.equal([...sessions.values()].filter(s=>s.instance===609).length,1);
 await page.getByRole('button',{name:'切换实例',exact:true}).click();
 const drawer=page.getByRole('dialog',{name:'切换实例',exact:true});await drawer.waitFor();
 await drawer.getByRole('button').filter({hasText:'DeepSeek personal instance'}).click();
 await page.waitForURL('**/ieisystem/instances/590');await root().waitFor();
 assert.ok(calls.some(c=>c.path==='/api/v1/ieisystem/instances'&&c.query.includes('page=2')),'missing second instance page');
 await page.getByRole('button',{name:'切换实例',exact:true}).click();
 await drawer.getByRole('button',{name:/^Team/}).click();
 assert.equal(await drawer.locator('.owner-switch-card').count(),4,'Team members must be directly visible');
 await drawer.getByRole('searchbox').fill('618');assert.equal(await drawer.locator('.owner-switch-card').count(),1);
 await drawer.locator('.owner-switch-card').click();await page.waitForURL('**/ieisystem/instances/618');await root().waitFor();
 await root().getByRole('tab',{name:'网页',exact:true}).click();
 await root().getByRole('button',{name:'新建网页',exact:true}).click();
 await root().locator('.wb-browser input').fill('http://localhost:3000');
 await root().locator('.wb-browser input').press('Enter');
 await root().locator('.wb-preview-frame').waitFor();
 for(const width of [1440,1280,768,390,320]){await page.setViewportSize({width,height:1000});await checkOverflow();await page.getByRole('button',{name:'切换实例',exact:true}).click();await drawer.waitFor();assert.ok(await drawer.evaluate(e=>e.getBoundingClientRect().width<=innerWidth));await page.screenshot({path:path.join(screenshots,'switch-'+width+'.png')});await page.keyboard.press('Escape');assert.equal(await drawer.count(),0);assert.equal(await page.getByRole('button',{name:'切换实例',exact:true}).evaluate(e=>e===document.activeElement),true);}
 for(const route of ['/ieisystem/teams/184','/teams/184']){
  await page.setViewportSize({width:1440,height:1000});await page.goto(origin+route);const w=page.locator('.team-chat-workspace');await w.waitFor();
  await w.locator('.team-header-files-trigger').click();await w.locator('.team-file-row').first().waitFor();
  await w.locator('.team-tools-toolbar').getByRole('button',{name:'终端',exact:true}).click();
  const selector=toolPane().getByRole('combobox',{name:'工具运行成员'});
  for(const id of route.includes('ieisystem')?[616,617,618,619]:[616]){
   console.log('Checking Team terminal',route,id);
   await selector.selectOption(String(id));await toolPane().getByRole('button',{name:'新建终端',exact:true}).click();await toolPane().locator('.xterm').waitFor();
   assert.ok([...sessions.values()].some(s=>s.instance===id));
  }
  const before=sessions.size;
  await w.locator('.team-tools-toolbar').getByRole('button',{name:'文件',exact:true}).click();await w.locator('.team-file-row').first().waitFor();
  await w.locator('.team-tools-toolbar').getByRole('button',{name:'终端',exact:true}).click();await toolPane().locator('.xterm').waitFor();assert.equal(sessions.size,before,'restoring a terminal created a duplicate session');
  for(const width of [1440,1280,768,390,320]){await page.setViewportSize({width,height:1000});await checkOverflow();await page.screenshot({path:path.join(screenshots,(route.includes('ieisystem')?'owner':'cm')+'-team-'+width+'.png')});}
 }
 const ownerCalls=calls.filter(c=>c.path.startsWith('/api/v1/ieisystem/')&&c.path.includes('/workbench/'));
 assert.ok(ownerCalls.length>0);assert.ok(ownerCalls.every(c=>!c.headers.authorization),'owner workbench leaked CM JWT');
 assert.ok(ownerCalls.filter(c=>c.method!=='GET').every(c=>c.headers['x-owner-action']==='1'),'missing explicit owner action');
 assert.equal(calls.filter(c=>/\/teams\/184\/tasks/.test(c.path)&&c.method==='POST').length,0,'tool selection dispatched a model task');
 assert.deepEqual(errors,[]);
 console.log(JSON.stringify({result:'PASS',checks:'owner-only auth, paginated switch, all members, browser, four Runtime terminals, session restore, CM/owner Team, widths, no dispatch',sessions:sessions.size}));
}catch(error){await page.screenshot({path:path.join(screenshots,'failure.png')});console.log((await page.locator('.team-workspace-tools').innerText().catch(()=>page.locator('body').innerText())).slice(0,2200));throw error;}finally{await browser.close();}
