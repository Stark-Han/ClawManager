// Synthetic fixtures only; lifecycle actions must remain cancelled.
const {chromium}=require('playwright');const assert=require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch({channel:'chrome',headless:true});const context=await browser.newContext({viewport:{width:1440,height:900},reducedMotion:'reduce'});const page=await context.newPage();page.setDefaultTimeout(10000);const errors=[];page.on('pageerror',e=>errors.push(e.message));
 const types=['openclaw','hermes','opencode','deepseek-harness'];
 const instances=types.map((type,i)=>({id:i+1,name:'长名称实例 · '+type+' · 协作工作空间',type,status:i===3?'error':'running',instance_mode:'lite',created_at:'2026-09-01T00:00:00Z',updated_at:'2026-09-21T04:00:00Z'}));
 const tasks=[['login',1,10],['questions',3,20],['questions',5,30],['tokens',5000,10],['tokens',20000,10],['tokens',50000,20]].map(([metric,target,xp],i)=>({code:'task-'+i,name:['今日启航','深入交流','持续推进','轻量协作','深度协作','充分协作'][i],metric,target,xp,period:'day',progress:i===0?1:0,awarded:false}));
 const growth={xp:27,level:1,level_start:0,next_level:120,started_at:'2026-09-01T00:00:00Z',as_of:'2026-09-21T04:00:00Z',tasks,ledger:[],days:[],rule:'growth-v3',note:'规则',daily_limit:100};
 let writes=0;
 let reportFailure=false;
 const reportRequests=[];
 await context.route('**/api/**',async route=>{const p=new URL(route.request().url()).pathname;let data={};if(p.endsWith('/session'))data={owner:'accept123@example.com',expires_at:'2030-01-01T00:00:00Z'};else if(p.endsWith('/instances'))data={instances,total:5};else if(p.includes('/growth'))data=growth;else if(p.endsWith('/dashboard'))data={summary:{questions:5,total_tokens:50000,sessions:2,workday_streak:1},trend:[],last_activity:{},coverage:{questions_supported:true,partial:true},insights:{as_of:growth.as_of,days:[],comparison:{tokens:{status:'new',current:5,previous:0,delta:5,percent:null}},window:{kind:'day'}}};else if(p.includes('lifecycle-operation')){await route.fulfill({status:404,json:{error:'none'}});return}else if(route.request().method()==='POST'&&/\/instances\/\d+\/(restart|reset)$/.test(p)){writes++}await route.fulfill({json:{data}})});
 await context.route('**/ieisystem/reports?**',route=>{
  const {searchParams:q}=new URL(route.request().url());
  reportRequests.push({kind:q.get('kind'),date:q.get('date'),revision:q.get('revision'),method:route.request().method()});
  if(reportFailure)return route.fulfill({status:503,json:{error:'unavailable'}});
  const ongoing=q.get('date')!=='2026-10-14';
  return route.fulfill({json:{data:{start:ongoing?'2026-11-16':'2026-10-01',end:ongoing?'2026-11-22':'2026-10-31',ongoing,counts:{tokens:50000,sessions:4},active_days:2,previous:{},runtime:[],days:[],saved_revisions:ongoing?[]:[1,2,3],revision:route.request().method()==='POST'?3:Number(q.get('revision'))}}});
 });
 const origin=process.env.OWNER_PREVIEW_URL||'http://127.0.0.1:38479';
 await page.goto(origin+'/ieisystem/list-instances?view=overview');await page.locator('.portal-runtime-card').first().waitFor();
 await page.getByText('已达标，奖励待入账',{exact:true}).waitFor();assert.match(await page.getByTestId('task-stat').innerText(),/1 \/ 6/);assert.match(await page.getByTestId('task-stat').innerText(),/已入账 0 \/ 100 XP/);
 await page.getByRole('button',{name:'全部任务'}).click();
 const taskDialog=page.getByRole('dialog',{name:'全部成长任务'});
 await taskDialog.waitFor();
 assert.equal(await taskDialog.locator('li').count(),6);
 await page.keyboard.press('Escape');
 const growthTrigger=page.getByTestId('owner-overview-summary').getByRole('button',{name:'成长路径',exact:true});
 await growthTrigger.click();
 const growthDialog=page.getByRole('dialog',{name:'成长详情'});
 await growthDialog.getByText('当前 Lv.1',{exact:true}).waitFor();
 await growthDialog.getByRole('button',{name:/^Lv\.50 /}).click();
 await growthDialog.getByRole('status').getByText(/当前开放上限/).waitFor();
 growth.upcoming_tasks=[{...tasks[0],name:'未来任务预告',target:9}];await growthDialog.getByRole('button',{name:'更新进度',exact:true}).click();
 await page.keyboard.press('Escape');
 await growthDialog.waitFor({state:'hidden'});
 assert.ok(await growthTrigger.evaluate(el=>el===document.activeElement));
 await page.getByRole('button',{name:'全部任务'}).click();await taskDialog.getByRole('button',{name:'新任务预告'}).click();
 await taskDialog.getByText('未来任务预告',{exact:true}).waitFor();await page.keyboard.press('Escape');
 const reportCard=page.getByRole('region',{name:'周期报告'});
 await reportCard.getByText('进行中',{exact:true}).waitFor();
 reportFailure=true;await reportCard.getByRole('button',{name:'月报',exact:true}).click();
 await reportCard.getByRole('alert').waitFor();assert.ok(await page.getByTestId('metric-tokens').isVisible());
 reportFailure=false;
 await page.goto(origin+'/ieisystem/list-instances?view=overview&dashboard_tab=reports&report_kind=month&report_date=2026-10-14&report_revision=2');
 const reportDialog=page.getByRole('dialog',{name:'周期报告'});
 await reportDialog.locator('.owner-report-summary').getByText('快照 v2',{exact:true}).waitFor();
 assert.equal(await reportDialog.getByLabel('报告日期').inputValue(),'2026-10-14');
 assert.ok(reportRequests.some(request=>request.kind==='month'&&request.date==='2026-10-14'&&request.revision==='2'));
 await reportDialog.getByRole('button',{name:'保存 / 更新快照'}).click();
 await page.waitForURL(url=>url.searchParams.get('report_revision')==='3');
 await page.keyboard.press('Escape');
 await reportDialog.waitFor({state:'hidden'});
 assert.equal(new URL(page.url()).searchParams.get('dashboard_tab'),'overview');
 assert.match(await page.getByTestId('metric-tokens').innerText(),/上期 0/);
 const intersects=(a,b)=>a.x<b.x+b.width&&a.x+a.width>b.x&&a.y<b.y+b.height&&a.y+a.height>b.y;
 for(const width of [1920,1440,1366,390]){
  console.log('Checking cards at',width);
  await page.setViewportSize({width,height:width===1366?768:900});
  if(width===390)await page.getByRole('button',{name:'工作区列表',exact:true}).click();
  for(const type of types){
   const card=page.locator(`.portal-runtime-card[data-runtime="${type}"]`);await card.scrollIntoViewIfNeeded();await card.hover();await page.waitForTimeout(280);
   const entry=await card.locator('.instance-hover-entry').boundingBox();
   for(const selector of ['.instance-runtime-label','.instance-heading strong','.instance-meta']) assert(!intersects(entry,await card.locator(selector).boundingBox()),type+' collapsed overlap '+width+' '+selector);
   assert.equal(await card.locator('.instance-card-expand').count(),0);
   assert.ok((await card.locator('.instance-avatar').boundingBox()).width>=44);
   assert(await card.locator('.instance-avatar img').evaluate(e=>e.complete&&e.naturalWidth>0));
  }
  assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'horizontal overflow '+width);
 }
 await page.setViewportSize({width:1440,height:900});const card=page.locator('.portal-runtime-card[data-runtime="deepseek-harness"]');await card.locator('.instance-more').click();
 let confirmations=0;page.on('dialog',async d=>{confirmations++;await d.dismiss()});await card.getByRole('menuitem',{name:'重启实例',exact:true}).click();assert.equal(confirmations,1);await card.getByRole('menuitem',{name:'重置实例',exact:true}).click();assert.equal(confirmations,2);assert.equal(writes,0);
 await card.getByRole('menuitem',{name:'实例详情',exact:true}).click();await page.getByRole('heading',{name:'运行时说明',exact:true}).waitFor();
 await page.getByRole('tab',{name:'总览',exact:true}).click();await page.locator('.portal-runtime-card[data-runtime="opencode"]').hover();await page.getByTestId('instance-hover-details').waitFor();await page.screenshot({path:'.tmp/r14-cards.png',fullPage:true});
 await context.route('**/instances/*/access',route=>route.fulfill({status:409,json:{error:'Runtime temporarily unavailable'}}));
 await page.getByTestId('instance-hover-details').locator('.instance-entry').click();
 await page.getByRole('tab',{name:'总览',exact:true}).click();await page.locator('.owner-dashboard').waitFor();
 assert.equal(new URL(page.url()).searchParams.get('view'),'overview');
 assert.equal(await page.getByTestId('instance-hover-details').count(),0);
 await page.reload();await page.locator('.owner-dashboard').waitFor();assert.equal(await page.getByTestId('instance-hover-details').count(),0);
 for(const [width,height] of [[1920,1080],[1920,944],[1440,900],[1366,768]]){
  await page.setViewportSize({width,height});await page.waitForTimeout(200);
  assert(await page.evaluate(()=>document.documentElement.scrollHeight<=innerHeight+1),'vertical overflow '+width+'x'+height);
  const trend=await page.getByRole('heading',{name:'Token 使用趋势'}).boundingBox();
  const tasks=await page.getByRole('heading',{name:'成长任务'}).boundingBox();
  assert.ok(trend.x<trend.x+trend.width&&trend.x+trend.width<=tasks.x&&Math.abs(trend.y-tasks.y)<8,'trend and tasks should share the first row '+width+'x'+height);
  assert.ok(await page.getByRole('heading',{name:'继续工作'}).isVisible());
  assert(await page.locator('.owner-summary .owner-welcome-line').evaluate(e=>parseFloat(getComputedStyle(e).fontSize)>=19));
 }
 await page.goto(origin+'/ieisystem/list-instances?dashboard_tab=growth');
 await growthDialog.getByText('成长路径 Lv.1–50').waitFor();assert.doesNotMatch(await growthDialog.innerText(),/Lv\.100|100\s*级/);
 await page.keyboard.press('Escape');await page.screenshot({path:'.tmp/r15-overview-layout.png',fullPage:true});
 assert.deepEqual(errors,[]);await browser.close();console.log('PASS: runtime card logos; 4 widths; no overlaps; hover details link; overview navigation and reload; no future level announcement; overview fills height; pending awards and top count');
})().catch(e=>{console.error(e);process.exit(1)});
