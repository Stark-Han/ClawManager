import { useCallback, useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { ArrowRight, ChevronUp, Plus, RefreshCw, Search, Users, X } from 'lucide-react';
import { OwnerTeamOperationError, withOwnerTeamIntent, pendingOwnerTeamIntent, recoverOwnerTeamIntent, ownerTeamService, ownerTeamWorkspaceAPI, waitForOwnerTeamOperation, type OwnerTeamSummary, type OwnerTeamCatalogEntry, type TeamOperation } from '../../services/ownerTeamService';
import type { OwnerTeamsState } from '../../hooks/useOwnerTeams';
import type { TeamDetails, TeamMember } from '../../types/team';
import type { CustomTeamTemplate } from '../../types/customTeamTemplate';
import TeamBadge, { TeamIconPicker } from './TeamBadge';
import { memberIcon } from '../../lib/teamIcons';
import { memberRailState } from './teamMemberPresentation';
import './owner-team-workspace.css';

const message = (error: unknown) => error instanceof Error ? error.message : '操作失败';
const teamURL = (id: number) => `/ieisystem/teams/${id}`;
const teamLabel = (team: OwnerTeamSummary) => team.status === 'creating' ? '创建中' : team.status !== 'running' || team.available_count < team.member_count ? '需处理' : team.members.some(m => m.status === 'busy' || m.availability === 'busy') ? '执行中' : '空闲';
const memberState = (m: TeamMember) => memberRailState(m);

function MemberRow({ member, ownerTeamID }: { member: TeamMember; ownerTeamID: number }) {
  const state = memberState(member);
  return <div className="ot-member"><TeamBadge iconKey={memberIcon(member)} size="chat" /><div><strong>{member.display_name || member.member_key}</strong><p>{member.runtime_type} · {member.role} · <span data-state={state.tone}>{state.label}</span></p></div>{member.instance_id ? <Link to={`/ieisystem/instances/${member.instance_id}`} state={{ portalReturnSearch: window.location.search, portalReturnTeamID: ownerTeamID }}>进入 ↗</Link> : <span className="ot-muted">待创建</span>}</div>;
}

export function OwnerTeamSidebar({ state, onSelect }: { state: OwnerTeamsState; onSelect?: () => void }) {
  return <div className="ot-sidebar"><label className="ot-search"><Search size={15}/><input value={state.query} onChange={e => state.setQuery(e.target.value)} placeholder="搜索团队名称…" aria-label="搜索团队"/></label>{state.error && <p role="alert" className="ot-error">{state.error}<button onClick={() => void state.refresh()}>重试</button></p>}
    {!state.teams.length && !state.loading && <p className="ot-empty">还没有 Team，可在右侧创建。</p>}
    {state.teams.map(team => {
      const expanded = state.expandedID === team.id;
      const toggle = () => { state.expand(team.id); onSelect?.(); };
      const entry = <Link aria-label={`进入团队 ${team.name}`} className="ot-card-entry" to={teamURL(team.id)} state={{ portalReturnSearch: window.location.search }}><span>进入团队</span><ArrowRight size={15}/></Link>;
      return <article key={team.id} data-team-id={team.id} data-expanded={expanded} className={`ot-team-card ${state.selectedID === team.id ? 'is-selected' : ''}`} onClick={e => { if ((e.target as HTMLElement).closest('a,button,input')) return; toggle(); }}>
        <div className="ot-card-watermark" aria-hidden="true"><img src="/team-expanded.png" alt=""/></div>
        <button className="ot-card-title" onClick={toggle} aria-expanded={expanded} aria-controls={`ot-members-${team.id}`}><span className="ot-card-avatar"><img src="/team-square.png" alt=""/></span><span className="ot-card-heading"><strong title={team.name}>{team.name}</strong><small>Team #{team.id} · {team.member_count} 名成员</small></span></button>
        <div className="ot-card-status"><span data-state={teamLabel(team) === '需处理' ? 'error' : teamLabel(team) === '创建中' ? 'waiting' : 'ready'}>● {teamLabel(team)}</span><span>{team.available_count}/{team.member_count} 可用</span>{!expanded && entry}</div>
        <div id={`ot-members-${team.id}`} hidden={!expanded} className="ot-card-members">{expanded && team.members.map(member => <MemberRow key={member.id} member={member} ownerTeamID={team.id}/>)}<footer><button onClick={toggle}>收起 <ChevronUp size={13}/></button>{entry}</footer></div>
      </article>;
    })}{state.hasMore && <button className="ot-secondary" onClick={state.loadMore}>加载更多</button>}{state.loading && !state.teams.length && <p role="status" className="ot-empty">正在读取 Team…</p>}
  </div>;
}

function useTeamOperation(owner: string, onDone: () => Promise<void>) {
  const [operation, setOperation] = useState<TeamOperation | null>(() => { try { const raw = sessionStorage.getItem(`owner-team-operation:${owner}`); return raw ? JSON.parse(raw) : null; } catch { return null; } });
  const operationOwner = useRef(owner);
  useEffect(() => { if (operationOwner.current !== owner) { operationOwner.current = owner; setOperation(null); setError(''); } }, [owner]);
  const [error, setError] = useState('');
  const [watch, setWatch] = useState(0);
  const done = useRef(onDone); done.current = onDone;
  useEffect(() => {
    if (!operation?.operation_id || !owner || operationOwner.current !== owner) return;
    sessionStorage.setItem(`owner-team-operation:${owner}`, JSON.stringify(operation));
    const controller = new AbortController(); setError('');
    void waitForOwnerTeamOperation(operation.operation_id, controller.signal).then(async result => {
      if (controller.signal.aborted) return;
      setOperation(result); sessionStorage.removeItem(`owner-team-operation:${owner}`); await done.current();
    }).catch(error => { if (!controller.signal.aborted) { if (error instanceof OwnerTeamOperationError) setOperation(error.operation); setError(message(error)); } });
    return () => controller.abort();
  }, [operation?.operation_id, owner, watch]);
  return { operation: operationOwner.current === owner ? operation : null, error, retryFailed: async () => { if (!operation) return; try { const next = await ownerTeamService.retryOperation(operation.operation_id, operation.attempt_count || 0); setError(''); setOperation(next); setWatch(n => n + 1); } catch (error) { setError(message(error)); } }, begin: (op: TeamOperation) => { setError(''); setOperation(op); }, retryRead: () => setWatch(n => n + 1), dismiss: () => { setOperation(null); setError(''); sessionStorage.removeItem(`owner-team-operation:${owner}`); } };
}

export function OwnerTeamDashboard({ state }: { state: OwnerTeamsState }) {
  const [section, setSection] = useState<'teams' | 'templates'>('teams');
  const [loadedDetails, setDetails] = useState<TeamDetails | null>(null);
  const details = loadedDetails?.team.id === state.selectedID ? loadedDetails : null;
  const [detailError, setDetailError] = useState('');
  const [create, setCreate] = useState(false);
  const [manage, setManage] = useState(false);
  const [narrowDetail, setNarrowDetail] = useState(false);
  const operation = useTeamOperation(state.owner, state.refresh);
  const pendingIntent = pendingOwnerTeamIntent(state.owner);
  const [recoveryError, setRecoveryError] = useState('');
  useEffect(() => {
    let active = true; setDetails(null); setDetailError('');
    if (!state.selectedID) return;
    const refresh = async () => { try { const details = await ownerTeamWorkspaceAPI.getTeam(state.selectedID!); if (active) { setDetails(details); setDetailError(''); } } catch (error) { if (active) setDetailError(message(error)); } };
    void refresh(); const timer = window.setInterval(() => { if (!document.hidden) void refresh(); }, 5000);
    return () => { active = false; clearInterval(timer); };
  }, [state.selectedID, state.owner]);
  const selected = state.teams.find(t => t.id === state.selectedID) || (details && details.team.id === state.selectedID ? { ...details.team, members: details.members, member_count: details.members.length, available_count: details.members.filter(m => ['idle','busy'].includes(m.availability || '')).length } : undefined);
  return <section className="ot-workspace"><header className="ot-heading"><div><h2>团队空间</h2><p>{state.total} 个团队 · 创建、管理与协作</p></div><div className="ot-actions"><button className="ot-secondary" onClick={() => setSection('templates')}>模板管理</button><button className="ot-primary" disabled={!state.capabilities?.can_create} title={state.capabilities?.create_disabled_reason} onClick={() => setCreate(true)}><Plus size={16}/> 新建 Team</button></div></header>
    {pendingIntent && <p className="ot-note" role="status">上次提交结果尚未确认。<button onClick={() => { setRecoveryError(''); void recoverOwnerTeamIntent(state.owner, pendingIntent).then(operation.begin).catch(error => setRecoveryError(message(error))); }}>恢复上次提交并查看进度</button>{recoveryError}</p>}
    {!state.capabilities?.can_create && <p className="ot-note">{state.capabilities?.create_disabled_reason || '当前暂不能创建团队；已有团队不受影响。'}</p>}
    <div className="ot-tabs" role="tablist" aria-label="团队空间"><button role="tab" aria-selected={section === 'teams'} onClick={() => setSection('teams')}>我的 Team</button><button role="tab" aria-selected={section === 'templates'} onClick={() => setSection('templates')}>团队模板</button></div>
    {operation.operation && <div className={operation.error ? 'ot-note' : 'ot-operation'} role="status"><span>{operation.operation.status === 'succeeded' ? '操作已完成' : operation.error || '正在创建团队并等待成员就绪…'}</span>{operation.error ? <><button onClick={operation.retryRead}>重新检查进度</button>{operation.operation.status === 'failed' && operation.operation.team_id && <button onClick={() => void operation.retryFailed()}>重试原操作</button>}<button onClick={operation.dismiss}>收起记录</button></> : operation.operation.status === 'succeeded' && <button onClick={operation.dismiss}>关闭</button>}</div>}
    {section === 'templates' ? <OwnerTeamTemplates key={state.owner} owner={state.owner} canGenerate={state.capabilities?.can_create === true} onOperation={operation.begin}/> : <div className={`ot-master-detail ${narrowDetail ? 'show-detail' : ''}`}>
      <div className="ot-master"><label className="ot-search"><Search size={14}/><input aria-label="筛选团队" placeholder="搜索团队…" value={state.query} onChange={e => state.setQuery(e.target.value)}/></label>{state.error && <p role="alert" className="ot-error">{state.error}</p>}{state.teams.map(t => <button key={t.id} aria-pressed={state.selectedID === t.id} className="ot-master-item" onClick={() => { state.select(t.id); setNarrowDetail(true); }}><strong>{t.name}</strong><span>#{t.id} · {t.available_count}/{t.member_count} 可用</span><small>{teamLabel(t)}</small></button>)}{state.hasMore && <button onClick={state.loadMore}>加载更多</button>}</div>
      <div className="ot-detail"><button className="ot-mobile-back" onClick={() => setNarrowDetail(false)}>← 团队列表</button>{detailError ? <p role="alert" className="ot-error">{detailError}</p> : selected ? <>
        <div className="ot-detail-title"><div className="ot-team-icon"><Users size={22}/></div><div><h3>{selected.name}</h3><p>Team #{selected.id}</p></div></div><p className="ot-description">{selected.description || '协作完成团队任务，沉淀共享产物。'}</p><p className="ot-muted">{selected.member_count} 名成员 · {selected.available_count}/{selected.member_count} 可用 · Leader 统筹</p>
        <div className="ot-actions"><Link className="ot-primary" to={teamURL(selected.id)} state={{ portalReturnSearch: window.location.search }}>进入团队对话 <ArrowRight size={15}/></Link><button className="ot-secondary" onClick={() => setManage(v => !v)}>管理</button><button className="ot-icon" aria-label="刷新团队" onClick={() => void state.refresh()}><RefreshCw size={15}/></button></div>
        <div className="ot-task-summary"><small>当前任务</small><p>{details?.tasks?.find(t => ['pending', 'dispatched', 'running', 'waiting'].includes(t.status))?.payload?.prompt as string || '暂无执行中的任务'}</p></div>
        <h4>团队成员</h4>{(details?.members || selected.members).map(m => <MemberRow key={m.id} member={m} ownerTeamID={selected.id}/>)}
        {manage && <OwnerTeamManage key={selected.id} details={details} onChange={state.refresh} onOperation={operation.begin}/>}
      </> : <div className="ot-empty"><Users size={32}/><h3>创建你的第一个 Team</h3><p>选择模板，或先生成自定义团队。</p><button className="ot-primary" disabled={!state.capabilities?.can_create} onClick={() => setCreate(true)}>新建 Team</button></div>}</div>
    </div>}
    {create && <OwnerTeamCreate owner={state.owner} onClose={() => setCreate(false)} onOperation={op => { operation.begin(op); setCreate(false); void state.refresh(); }}/>}
  </section>;
}

function OwnerTeamCreate({ owner, onClose, onOperation }: { owner: string; onClose: () => void; onOperation: (op: TeamOperation) => void }) {
  const [catalog, setCatalog] = useState<OwnerTeamCatalogEntry[]>([]); const [custom, setCustom] = useState<CustomTeamTemplate[]>([]);
  const [selected, setSelected] = useState(''); const [name, setName] = useState(''); const [description, setDescription] = useState(''); const [busy, setBusy] = useState(false); const [error, setError] = useState('');
  const [memberRuntimes, setMemberRuntimes] = useState<Record<string, string>>({});
  const [memberModes, setMemberModes] = useState<Record<string, string>>({});
  const dialog = useRef<HTMLDialogElement>(null);
  useEffect(() => { const element = dialog.current; element?.showModal(); return () => element?.close(); }, []);
  useEffect(() => { const controller = new AbortController(); void Promise.all([ownerTeamService.catalog(controller.signal), ownerTeamService.templates(controller.signal)]).then(([catalog, custom]) => { if (controller.signal.aborted) return; setCatalog(catalog.templates); setCustom(custom.templates); setSelected(catalog.templates[0] ? `builtin:${catalog.templates[0].key}` : ''); }).catch(error => { if (!controller.signal.aborted) setError(message(error)); }); return () => controller.abort(); }, []);
  const builtin = catalog.find(t => selected === `builtin:${t.key}`); const template = custom.find(t => selected === `custom:${t.id}`);
  const members = builtin?.members || template?.spec.members.map(m => ({ member_id: m.memberId, name: m.displayName, role: m.role, is_leader: m.isLeader, runtime_type: 'openclaw', instance_mode: 'lite' })) || [];
  return <dialog ref={dialog} aria-labelledby="ot-create-title" className="ot-modal" onCancel={e => { if (busy) e.preventDefault(); else onClose(); }}><header><h3 id="ot-create-title">新建 Team</h3><button aria-label="关闭创建" disabled={busy} onClick={onClose}><X size={18}/></button></header><form onSubmit={e => { e.preventDefault(); if (busy || !members.length) return; setBusy(true); setError(''); void withOwnerTeamIntent(owner, 'create', { name, description, template_key: builtin?.key, template_id: template?.id, revision: builtin?.revision || template!.revision, members: members.map(m => ({ member_id: m.member_id, runtime_type: m.is_leader ? 'openclaw' : memberRuntimes[m.member_id] || m.runtime_type, instance_mode: memberModes[m.member_id] || m.instance_mode })) }, (data, key) => ownerTeamService.create(data, key)).then(onOperation).catch(error => setError(message(error))).finally(() => setBusy(false)); }}>
    <label>团队名称<input required maxLength={100} value={name} onChange={e => setName(e.target.value)}/></label><label>团队模板<select value={selected} onChange={e => { setSelected(e.target.value); setMemberRuntimes({}); setMemberModes({}); }}><optgroup label="内置模板">{catalog.map(t => <option key={t.key} value={`builtin:${t.key}`}>{t.name} · {t.members.length} 人</option>)}</optgroup><optgroup label="我的自定义模板">{custom.map(t => <option key={t.id} value={`custom:${t.id}`}>{t.name} · {t.resolved_member_count} 人</option>)}</optgroup></select></label><label>说明<textarea value={description} maxLength={2000} onChange={e => setDescription(e.target.value)}/></label>
    <div className="ot-create-members">{members.map(m => <div key={m.member_id}><strong>{m.name}</strong><span>{m.role}</span><select aria-label={`${m.name} Runtime`} disabled={m.is_leader} value={m.is_leader ? 'openclaw' : memberRuntimes[m.member_id] || m.runtime_type} onChange={e => setMemberRuntimes(v => ({ ...v, [m.member_id]: e.target.value }))}><option value="openclaw">OpenClaw</option><option value="hermes">Hermes</option></select><select aria-label={`${m.name}模式`} value={memberModes[m.member_id] || m.instance_mode} onChange={e => setMemberModes(v => ({ ...v, [m.member_id]: e.target.value }))}><option value="lite">Lite</option><option value="pro">Pro</option></select></div>)}</div>{error && <p role="alert" className="ot-error">{error}</p>}<footer><button type="button" disabled={busy} className="ot-secondary" onClick={onClose}>取消</button><button className="ot-primary" disabled={busy || !members.length}>{busy ? '正在提交…' : '创建团队'}</button></footer></form></dialog>;
}

function OwnerTeamManage({ details, onChange, onOperation }: { details: TeamDetails | null; onChange: () => Promise<void>; onOperation: (op: TeamOperation) => void }) {
  const [name, setName] = useState(details?.team.name || ''); const [description, setDescription] = useState(details?.team.description || ''); const [error, setError] = useState(''); const [busy, setBusy] = useState(false);
  useEffect(() => { if (details) { setName(details.team.name); setDescription(details.team.description || ''); } }, [details?.team.id]);
  if (!details) return <p className="ot-muted">正在读取管理信息…</p>;
  const run = async (action: () => Promise<void>) => { if (busy) return; setBusy(true); setError(''); try { await action(); } catch (error) { setError(message(error)); } finally { setBusy(false); } };
  return <section className="ot-manage"><h4>管理 Team</h4><form onSubmit={e => { e.preventDefault(); void run(async () => { await ownerTeamService.update(details.team.id, { name, description }); await onChange(); }); }}><label>名称<input required maxLength={100} value={name} onChange={e => setName(e.target.value)}/></label><label>说明<textarea value={description} onChange={e => setDescription(e.target.value)}/></label><button className="ot-secondary" disabled={busy}>保存</button></form>
    {details.members.filter(m => m.member_key !== details.leader_member_id).map(m => <div key={m.id} className="ot-remove-member"><span>{m.display_name}</span><button disabled={busy} onClick={() => { if (window.confirm(`移除“${m.display_name}”会删除该成员实例及数据，请先保全需要的文件。确认继续？`)) void run(async () => { const op = await ownerTeamService.removeMember(details.team.id, m.member_key); onOperation(op); await onChange(); }); }}>移除成员</button></div>)}
    <button className="ot-danger" disabled={busy} onClick={() => { if (window.confirm('删除整个 Team 会删除成员实例和共享数据，无法撤销。确认继续？')) void run(async () => { const op = await ownerTeamService.removeTeam(details.team.id); onOperation(op); await onChange(); }); }}>删除 Team 与数据</button>{error && <p role="alert" className="ot-error">{error}</p>}
  </section>;
}

function OwnerTeamTemplates({ owner, canGenerate, onOperation }: { owner: string; canGenerate: boolean; onOperation: (op: TeamOperation) => void }) {
  const [templates, setTemplates] = useState<CustomTeamTemplate[]>([]); const [selected, setSelected] = useState<CustomTeamTemplate | null>(null); const [intent, setIntent] = useState(''); const [name, setName] = useState(''); const [count, setCount] = useState(3); const [error, setError] = useState(''); const [busy, setBusy] = useState(false);
  const refresh = useCallback(async () => { const result = await ownerTeamService.templates(); setTemplates(result.templates); }, []);
  useEffect(() => { let active = true; void ownerTeamService.templates().then(result => { if (active) setTemplates(result.templates); }).catch(error => { if (active) setError(message(error)); }); return () => { active = false; }; }, [owner]);
  const run = async (action: () => Promise<void>) => { if (busy) return; setBusy(true); setError(''); try { await action(); } catch (error) { setError(message(error)); } finally { setBusy(false); } };
  const generate = async (action: string, data: object) => { const op = await withOwnerTeamIntent(owner, `template:${selected?.id || 'new'}:${action}`, data, (payload, key) => selected ? ownerTeamService.templateAction(selected.id, action, payload, key) : ownerTeamService.generateTemplate(payload as { intent: string; member_count?: number }, key)); onOperation(op); const done = await waitForOwnerTeamOperation(op.operation_id); await refresh(); if (done.template_id) setSelected(await ownerTeamService.template(done.template_id)); };
  return <div className="ot-template-manager"><div className="ot-template-list"><button className="ot-secondary" onClick={() => { setSelected(null); setName(''); setIntent(''); }}>＋ 自定义团队模板</button>{templates.map(t => <button className="ot-master-item" aria-pressed={selected?.id === t.id} key={t.id} onClick={() => { setSelected(t); setName(t.name); setIntent(t.intent); setCount(t.resolved_member_count); }}><strong>{t.name}</strong><span>{t.resolved_member_count} 人 · v{t.revision}</span></button>)}<button onClick={() => void run(refresh)}>刷新模板</button></div><section className="ot-template-editor"><h3>{selected ? '管理自定义模板' : '生成自定义团队模板'}</h3><form onSubmit={e => { e.preventDefault(); void run(() => generate(selected ? 'revise' : '', { intent, member_count: count, ...(selected ? { name, expected_revision: selected.revision } : {}) })); }}>
    {selected && <label>模板名称<input value={name} required onChange={e => setName(e.target.value)}/></label>}<label>团队目标与分工<textarea required maxLength={4000} rows={5} value={intent} onChange={e => setIntent(e.target.value)}/></label><label>成员数量<input type="number" min={2} max={6} value={count} onChange={e => setCount(Number(e.target.value))}/></label><div className="ot-actions"><button className="ot-primary" disabled={!canGenerate || busy}>{busy ? '处理中…' : selected ? '按说明修订模板' : '生成模板'}</button>{selected && <><button type="button" className="ot-secondary" disabled={busy} onClick={() => void run(async () => { const t = await ownerTeamService.updateTemplate(selected.id, name, selected.revision); setSelected(t); await refresh(); })}>仅保存名称</button><button type="button" className="ot-secondary" disabled={!canGenerate || busy} onClick={() => void run(() => generate('regenerate', { expected_revision: selected.revision }))}>重新生成</button></>}</div></form>
    {selected?.spec.members.map(m => <article className="ot-template-member" key={m.memberId}><strong>{m.displayName} · {m.isLeader ? 'Leader' : m.role}</strong><p>{m.summary}</p><details><summary>查看职责与交付</summary><p>{m.mission}</p><ul>{m.responsibilities.map((r, i) => <li key={i}>{r}</li>)}</ul></details><div className="ot-actions"><button disabled={!canGenerate || busy} onClick={() => { const instruction = window.prompt(`如何调整“${m.displayName}”的职责？`); if (instruction?.trim()) void run(() => generate(`members/${encodeURIComponent(m.memberId)}/adjust`, { instruction, expected_revision: selected.revision })); }}>调整职责</button><button disabled={!canGenerate || busy} onClick={() => void run(() => generate(`members/${encodeURIComponent(m.memberId)}/regenerate`, { expected_revision: selected.revision }))}>重新生成成员</button><TeamIconPicker label={`${m.displayName}图标`} value={m.iconOverride} autoKey={m.isLeader ? 'leader' : m.autoIconKey || m.iconKey || 'worker'} disabled={busy || m.isLeader} onChange={value => void run(async () => { const t = await ownerTeamService.templateIcon(selected.id, m.memberId, { icon_override: value, expected_revision: selected.revision }); setSelected(t); await refresh(); })}/></div></article>)}
    {selected && <button className="ot-danger" disabled={busy} onClick={() => { if (window.confirm('删除模板不会改变已创建的团队，确认删除？')) void run(async () => { await ownerTeamService.deleteTemplate(selected.id); setSelected(null); await refresh(); }); }}>删除模板</button>}{error && <p role="alert" className="ot-error">{error}</p>}
  </section></div>;
}
