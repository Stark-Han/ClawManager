import { useEffect, useRef, useState } from 'react';
import { Check, ChevronDown, MoreHorizontal, PencilLine, Plus, RefreshCw, Sparkles, Trash2 } from 'lucide-react';
import { ownerTeamService, waitForOwnerTeamOperation, withOwnerTeamIntent, type TeamOperation } from '../../services/ownerTeamService';
import type { CustomTeamTemplate } from '../../types/customTeamTemplate';
import TeamBadge, { TeamIconPicker } from './TeamBadge';
import { OwnerTeamDialog, useOwnerTeamDraftGuard } from './OwnerTeamDialog';

type Draft = { name: string; intent: string; count: number; revision?: number };
const initialDraft = (template: CustomTeamTemplate | null): Draft => ({ name: template?.name || '', intent: template?.intent || '', count: template?.resolved_member_count || 3, revision: template?.revision });
const errorMessage = (error: unknown) => error instanceof Error ? error.message : '操作失败，请重试';

export function OwnerTeamTemplates({ owner, canGenerate, onOperation }: { owner: string; canGenerate: boolean; onOperation: (op: TeamOperation) => void }) {
  const [templates, setTemplates] = useState<CustomTeamTemplate[]>([]), [selected, setSelected] = useState<CustomTeamTemplate | null>(null);
  const [draft, setDraft] = useState<Draft>(initialDraft(null));
  const [loading, setLoading] = useState(true), [busy, setBusy] = useState(false), [error, setError] = useState(''), [notice, setNotice] = useState('');
  const [adjustMember, setAdjustMember] = useState<string | null>(null), [instruction, setInstruction] = useState('');
  const controller = useRef<AbortController | null>(null), lock = useRef(false), sequence = useRef(0), selectedID = useRef<number | null>(null);
  const dirty = draft.name !== (selected?.name || '') || draft.intent !== (selected?.intent || '') || draft.count !== (selected?.resolved_member_count || 3);
  useOwnerTeamDraftGuard(dirty || (!!adjustMember && !!instruction) || busy);
  const key = (id: number | null) => `owner-team-template-draft:${owner}:${id ?? 'new'}`;
  const choose = (template: CustomTeamTemplate | null) => {
    let next = initialDraft(template);
    try {
      const saved = JSON.parse(sessionStorage.getItem(key(template?.id ?? null)) || 'null');
      if (saved && typeof saved.name === 'string' && typeof saved.intent === 'string' && Number.isInteger(saved.count) && saved.count >= 2 && saved.count <= 6) next = { ...next, ...saved };
      sessionStorage.setItem(`owner-team-template-selected:${owner}`, String(template?.id ?? 'new'));
    } catch { /* Storage unavailable: keep the visible form usable. */ }
    selectedID.current = template?.id ?? null; setSelected(template); setDraft(next); setError(''); setNotice('');
  };
  useEffect(() => {
    if (loading) return;
    try { sessionStorage.setItem(key(selected?.id ?? null), JSON.stringify(draft)); } catch { /* Refresh guard still protects the visible draft. */ }
  }, [owner, selected?.id, draft, loading]);
  useEffect(() => {
    const active = new AbortController(); controller.current = active;
    void ownerTeamService.templates(active.signal).then(result => {
      if (active.signal.aborted) return;
      setTemplates(result.templates);
      let remembered: string | null = null; try { remembered = sessionStorage.getItem(`owner-team-template-selected:${owner}`); } catch { /* optional */ }
      choose(remembered === 'new' ? null : result.templates.find(t => String(t.id) === remembered) || result.templates[0] || null);
    }).catch(error => { if (!active.signal.aborted) setError(errorMessage(error)); }).finally(() => { if (!active.signal.aborted) setLoading(false); });
    return () => active.abort();
  // The component is keyed by owner; initial loading must not replace edits.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [owner]);
  const refresh = async () => {
    const request = ++sequence.current;
    const result = await ownerTeamService.templates(controller.current?.signal);
    if (controller.current?.signal.aborted || request !== sequence.current) return result.templates;
    setTemplates(result.templates); return result.templates;
  };
  const run = async (action: () => Promise<void>) => {
    if (lock.current) return;
    lock.current = true; setBusy(true); setError(''); setNotice('');
    try { await action(); } catch (error) { if (!controller.current?.signal.aborted) setError(errorMessage(error)); }
    finally { lock.current = false; if (!controller.current?.signal.aborted) setBusy(false); }
  };
  const accept = (template: CustomTeamTemplate, replaceDraft: boolean) => {
    if (controller.current?.signal.aborted) return;
    selectedID.current = template.id; setSelected(template);
    setDraft(previous => replaceDraft ? initialDraft(template) : { ...previous, revision: template.revision });
    try { sessionStorage.setItem(`owner-team-template-selected:${owner}`, String(template.id)); } catch { /* optional */ }
  };
  const generate = async (action: string, data: object) => {
    const template = selected;
    const op = await withOwnerTeamIntent(owner, `template:${template?.id || 'new'}:${action}`, data, (payload, requestKey) => template ? ownerTeamService.templateAction(template.id, action, payload, requestKey) : ownerTeamService.generateTemplate(payload as { intent: string; member_count?: number }, requestKey));
    if (controller.current?.signal.aborted) return;
    onOperation(op);
    const done = await waitForOwnerTeamOperation(op.operation_id, controller.current?.signal);
    await refresh();
    if (done.template_id) {
      const updated = await ownerTeamService.template(done.template_id, controller.current?.signal);
      // Selection is locked during a mutation; unmount/owner changes are fenced.
      if (!controller.current?.signal.aborted) { accept(updated, true); setNotice('模板已更新，已创建的团队保持不变。'); }
    }
  };
  const reloadVersion = () => void run(async () => {
    if (!selected) return;
    const latest = await ownerTeamService.template(selected.id, controller.current?.signal);
    accept(latest, false); await refresh(); setNotice('已读取最新版本，保留了你的输入。请核对内容后再保存或修订。');
  });
  return <section className="ot-template-area" aria-busy={loading} aria-label="团队模板管理">
    <header className="ot-template-toolbar"><div><strong>我的模板</strong><span className="ot-muted">{templates.length} 个</span></div><div className="ot-actions">
      <button type="button" className="ot-secondary" disabled={busy || loading} onClick={() => void run(async () => { await refresh(); setNotice('模板列表已刷新，编辑内容已保留。'); })}><RefreshCw size={15}/>刷新模板</button>
      <button type="button" className="ot-secondary" disabled={busy || loading} onClick={() => choose(null)}><Plus size={15}/>自定义团队模板</button>
    </div></header>
    <div className="ot-template-manager">
      <aside className="ot-template-list" aria-label="模板列表">{loading && <p role="status" className="ot-muted">正在读取模板…</p>}{templates.map(t => <button type="button" disabled={busy} className="ot-master-item" aria-pressed={selected?.id === t.id} key={t.id} onClick={() => choose(t)}><strong>{t.name}</strong><span>{t.resolved_member_count} 人 · v{t.revision}</span></button>)}{!loading && !templates.length && <p className="ot-muted">还没有自定义模板，请描述目标生成。</p>}</aside>
      <section className="ot-template-editor" hidden={loading} aria-label="模板编辑滚动区域">
        <div className="ot-template-title"><h3>{selected ? selected.name : '生成自定义团队模板'}</h3>{selected && <span className="ot-tag">自定义模板 · v{selected.revision}</span>}</div>
        <form id="ot-template-form" onSubmit={event => { event.preventDefault(); void run(() => generate(selected ? 'revise' : '', { intent: draft.intent, member_count: draft.count, ...(selected ? { name: draft.name, expected_revision: draft.revision ?? selected.revision } : {}) })); }}>
          <div className="ot-template-fields">{selected && <label>模板名称<input value={draft.name} maxLength={100} required disabled={busy} onChange={e => setDraft(v => ({ ...v, name: e.target.value }))}/></label>}
          <label>成员数量<input type="number" min={2} max={6} required disabled={busy} value={draft.count} onChange={e => setDraft(v => ({ ...v, count: Number(e.target.value) }))}/></label></div>
          <label>团队目标与分工<textarea aria-label="团队目标与分工" required maxLength={4000} rows={3} disabled={busy} value={draft.intent} onChange={e => setDraft(v => ({ ...v, intent: e.target.value }))} placeholder="描述希望团队完成的目标、成员分工与交付要求…"/></label>
        </form>
        {selected && <><div className="ot-section-title"><h4>成员与分工</h4><span>{selected.spec.members.length} 名成员</span></div>{selected.spec.members.map(m => <details className="ot-template-member" key={`${selected.id}:${m.memberId}`}>
          <summary><TeamBadge iconKey={m.isLeader ? 'leader' : m.iconOverride || m.autoIconKey || m.iconKey || 'worker'} size="chat"/><span><strong>{m.displayName}</strong><small>{m.isLeader ? 'Leader' : m.role}</small></span><ChevronDown size={14}/></summary>
          <div className="ot-template-member-body"><p>{m.summary}</p><h5>职责与交付</h5><p>{m.mission}</p><ul>{m.responsibilities.map((r, i) => <li key={`r${i}`}>{r}</li>)}</ul>{[['交付物', m.deliverables], ['验收标准', m.acceptanceCriteria], ['工作边界', m.boundaries], ['所需输入', m.expectedInputs], ['协作说明', m.collaborationNotes]].map(([label, items]) => Array.isArray(items) && items.length > 0 && <div key={String(label)}><h5>{label}</h5><ul>{items.map((item, i) => <li key={i}>{item}</li>)}</ul></div>)}
          <div className="ot-actions"><button type="button" className="ot-secondary" disabled={!canGenerate || busy} onClick={() => { setAdjustMember(m.memberId); setInstruction(''); }}><PencilLine size={14}/>调整职责</button><button type="button" className="ot-secondary" disabled={!canGenerate || busy} onClick={() => void run(() => generate(`members/${encodeURIComponent(m.memberId)}/regenerate`, { expected_revision: draft.revision ?? selected.revision }))}><RefreshCw size={14}/>重新生成成员</button><TeamIconPicker label={`${m.displayName}图标`} value={m.iconOverride} autoKey={m.isLeader ? 'leader' : m.autoIconKey || m.iconKey || 'worker'} disabled={busy || m.isLeader} onChange={value => void run(async () => { const t = await ownerTeamService.templateIcon(selected.id, m.memberId, { icon_override: value, expected_revision: draft.revision ?? selected.revision }); accept(t, false); await refresh(); setNotice('成员图标已保存。'); })}/></div></div>
        </details>)}</>}

      </section>
    </div>
    <footer className="ot-template-footer">{(error || notice) && <div className="ot-template-feedback">        {error && <div role="alert" className="ot-error"><p>{error}</p>{selected && <button type="button" className="ot-secondary" disabled={busy} onClick={reloadVersion}>读取最新版本并保留输入</button>}</div>}
        {notice && <p role="status" className="ot-inline-status">{notice}</p>}</div>}<span className="ot-muted">{dirty ? '有尚未保存的修改' : '修改仅应用于模板'}</span><div className="ot-actions">
      {selected && <><button type="button" className="ot-secondary" disabled={busy || !draft.name.trim()} onClick={() => void run(async () => { const t = await ownerTeamService.updateTemplate(selected.id, draft.name, draft.revision ?? selected.revision); accept(t, false); await refresh(); setNotice('模板名称已保存。'); })}><Check size={15}/>仅保存名称</button>
      <details className="ot-more" onKeyDown={event => { if (event.key === 'Escape') { event.currentTarget.open = false; event.currentTarget.querySelector('summary')?.focus(); } }}><summary className="ot-secondary" aria-label="更多模板操作"><MoreHorizontal size={16}/>更多</summary><div className="ot-more-menu"><button type="button" disabled={!canGenerate || busy} onClick={() => void run(() => generate('regenerate', { expected_revision: draft.revision ?? selected.revision }))}><RefreshCw size={15}/>重新生成模板</button><button type="button" className="ot-danger-text" disabled={busy} onClick={() => { if (window.confirm('删除模板不会改变已创建的团队，确认删除？')) void run(async () => { await ownerTeamService.deleteTemplate(selected.id); try { sessionStorage.removeItem(key(selected.id)); } catch { /* optional */ } await refresh(); choose(null); setNotice('模板已删除，已有团队保持不变。'); }); }}><Trash2 size={15}/>删除模板</button></div></details></>}
      <button type="submit" form="ot-template-form" className="ot-primary" disabled={!canGenerate || busy || loading}><Sparkles size={15}/>{busy ? '处理中…' : selected ? '按说明修订模板' : '生成模板'}</button>
    </div></footer>
    {adjustMember && <OwnerTeamDialog title="调整成员职责" busy={busy} dirty={!!instruction} onClose={() => { setAdjustMember(null); setInstruction(''); }}><form onSubmit={event => { event.preventDefault(); void run(async () => { await generate(`members/${encodeURIComponent(adjustMember)}/adjust`, { instruction, expected_revision: draft.revision ?? selected!.revision }); if (!controller.current?.signal.aborted) { setAdjustMember(null); setInstruction(''); } }); }}><div className="ot-form-scroll"><label>调整说明<textarea required maxLength={4000} rows={5} disabled={busy} value={instruction} onChange={e => setInstruction(e.target.value)} placeholder="描述希望补充或调整的职责…"/></label>{error && <p role="alert" className="ot-error">{error}</p>}</div><footer><button type="button" className="ot-secondary" disabled={busy} onClick={() => { if (!instruction || window.confirm('放弃本次职责调整说明？')) { setAdjustMember(null); setInstruction(''); } }}>取消</button><button className="ot-primary" disabled={busy || !instruction.trim()}><Sparkles size={15}/>应用调整</button></footer></form></OwnerTeamDialog>}
  </section>;
}
