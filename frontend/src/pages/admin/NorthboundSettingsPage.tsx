import React, { useEffect, useMemo, useState } from 'react';
import { Download, Network, RefreshCw, Save, ShieldCheck, TriangleAlert } from 'lucide-react';
import AdminLayout from '../../components/AdminLayout';
import { northboundSettingsService, type NorthboundCallerPolicy, type NorthboundOverview, type NorthboundSettings } from '../../services/northboundSettingsService';
import { userService } from '../../services/userService';
import type { User } from '../../types/user';

const runtimes = [
  { value: 'openclaw', label: 'OpenClaw' }, { value: 'hermes', label: 'Hermes' },
  { value: 'opencode', label: 'OpenCode' }, { value: 'deepseek-harness', label: 'DeepSeek Harness' },
  { value: 'workbuddy', label: 'WorkBuddy' },
];
const proRuntimes = runtimes.filter((item) => item.value !== 'deepseek-harness');
const scopes = ['lite-instances:create', 'lite-instances:read', 'pro-instances:create', 'pro-instances:read', 'lite-instances:share-link:manage', 'lite-instances:share-link:reset'];

function errorMessage(error: unknown) {
  const message = (error as { response?: { data?: { error?: string } } })?.response?.data?.error;
  return message || (error instanceof Error ? error.message : '操作失败');
}

const NumberField = ({ label, value, onChange, min = 1, step = 1, suffix }: { label: string; value: number; onChange: (value: number) => void; min?: number; step?: number; suffix?: string }) => (
  <label className="space-y-1 text-sm text-slate-700">
    <span>{label}</span>
    <div className="flex items-center rounded-lg border border-slate-200 bg-white px-3 focus-within:border-red-400">
      <input className="w-full bg-transparent py-2 outline-none" type="number" min={min} step={step} value={value} onChange={(event) => onChange(Number(event.target.value))} />
      {suffix && <span className="text-xs text-slate-400">{suffix}</span>}
    </div>
  </label>
);

const RuntimeSelector = ({ title, values, choices, onChange }: { title: string; values: string[]; choices: typeof runtimes; onChange: (values: string[]) => void }) => (
  <div>
    <div className="mb-2 text-sm font-medium text-slate-800">{title}</div>
    <div className="flex flex-wrap gap-2">
      {choices.map((runtime) => {
        const selected = values.includes(runtime.value);
        return <button type="button" key={runtime.value} onClick={() => onChange(selected ? values.filter((value) => value !== runtime.value) : [...values, runtime.value])} className={`rounded-full border px-3 py-1.5 text-sm ${selected ? 'border-red-300 bg-red-50 text-red-700' : 'border-slate-200 text-slate-500'}`}>{runtime.label}</button>;
      })}
    </div>
  </div>
);

const NorthboundSettingsPage: React.FC = () => {
  const [overview, setOverview] = useState<NorthboundOverview | null>(null);
  const [settings, setSettings] = useState<NorthboundSettings | null>(null);
  const [users, setUsers] = useState<User[]>([]);
  const [selectedUserID, setSelectedUserID] = useState(0);
  const [caller, setCaller] = useState<NorthboundCallerPolicy>({ user_id: 0, enabled: true, scopes: [...scopes] });
  const [nodePort, setNodePort] = useState(32343);
  const [certificateDNS, setCertificateDNS] = useState('');
  const [certificateIPs, setCertificateIPs] = useState('');
  const [certificateDays, setCertificateDays] = useState(825);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');

  const load = async () => {
    setBusy(true); setError('');
    try {
      const [nextOverview, userResult] = await Promise.all([northboundSettingsService.getOverview(), userService.getUsers(1, 1000)]);
      setOverview(nextOverview); setSettings(nextOverview.settings); setUsers(userResult.users);
      setNodePort(nextOverview.cluster.external_node_port || nextOverview.settings.external_node_port || 32343);
      if (!certificateDNS) setCertificateDNS(nextOverview.cluster.certificate.dns_names?.join(', ') || '');
      if (!certificateIPs) setCertificateIPs(nextOverview.cluster.certificate.ip_addresses?.join(', ') || '');
    } catch (loadError) { setError(errorMessage(loadError)); } finally { setBusy(false); }
  };

  useEffect(() => {
    let active = true;
    Promise.all([northboundSettingsService.getOverview(), userService.getUsers(1, 1000)])
      .then(([nextOverview, userResult]) => {
        if (!active) return;
        setOverview(nextOverview); setSettings(nextOverview.settings); setUsers(userResult.users);
        setNodePort(nextOverview.cluster.external_node_port || nextOverview.settings.external_node_port || 32343);
        setCertificateDNS(nextOverview.cluster.certificate.dns_names?.join(', ') || '');
        setCertificateIPs(nextOverview.cluster.certificate.ip_addresses?.join(', ') || '');
      })
      .catch((loadError) => { if (active) setError(errorMessage(loadError)); })
      .finally(() => { if (active) setBusy(false); });
    return () => { active = false; };
  }, []);

  const selectedUser = useMemo(() => users.find((user) => user.id === selectedUserID), [users, selectedUserID]);
  const selectCaller = (userID: number) => {
    setSelectedUserID(userID);
    const existing = overview?.callers.find((item) => item.user_id === userID);
    setCaller(existing ? { ...existing } : { user_id: userID, enabled: true, scopes: [...scopes] });
  };
  const update = <K extends keyof NorthboundSettings>(key: K, value: NorthboundSettings[K]) => setSettings((current) => current ? { ...current, [key]: value } : current);

  const saveSettings = async () => {
    if (!settings) return; setBusy(true); setError(''); setMessage('');
    try { const saved = await northboundSettingsService.saveSettings(settings); setSettings(saved); setOverview((current) => current ? { ...current, settings: saved } : current); setMessage('北向配置已保存，动态策略会在数秒内生效。'); }
    catch (saveError) { setError(errorMessage(saveError)); } finally { setBusy(false); }
  };

  const savePort = async () => {
    setBusy(true); setError(''); setMessage('');
    try { const saved = await northboundSettingsService.setExternalNodePort(nodePort); setOverview(saved); setSettings(saved.settings); setMessage('外部 NodePort 已更新；内部 9443/9002 未改变。'); }
    catch (saveError) { setError(errorMessage(saveError)); } finally { setBusy(false); }
  };

  const saveCaller = async () => {
    if (!selectedUserID) return; setBusy(true); setError(''); setMessage('');
    try { await northboundSettingsService.saveCaller({ ...caller, user_id: selectedUserID }); setMessage('调用方权限已保存，旧北向会话已撤销。'); await load(); }
    catch (saveError) { setError(errorMessage(saveError)); } finally { setBusy(false); }
  };

  const prepareCertificate = async () => {
    setBusy(true); setError(''); setMessage('');
    try { const saved = await northboundSettingsService.prepareCertificate({ dns_names: certificateDNS.split(',').map((v) => v.trim()).filter(Boolean), ip_addresses: certificateIPs.split(',').map((v) => v.trim()).filter(Boolean), valid_days: certificateDays }); setOverview(saved); setMessage('新托管 CA 与 Gateway 证书已暂存，当前证书未改变。请先下载并分发新的公共 CA，再执行激活。'); }
    catch (saveError) { setError(errorMessage(saveError)); } finally { setBusy(false); }
  };

  const activateCertificate = async () => {
    if (!window.confirm('确认调用方已经信任新公共 CA？激活会替换 Gateway 证书并触发滚动。')) return;
    setBusy(true); setError(''); setMessage('');
    try { const saved = await northboundSettingsService.activateCertificate(); setOverview(saved); setMessage('托管证书已激活并触发 Gateway 滚动。'); }
    catch (saveError) { setError(errorMessage(saveError)); } finally { setBusy(false); }
  };

  if (!settings || !overview) return <AdminLayout title="北向接口管理"><div className="p-8 text-slate-500">{error || '正在读取北向配置…'}</div></AdminLayout>;
  const cert = overview.cluster.certificate;

  return (
    <AdminLayout title="北向接口管理">
      <div className="space-y-5 p-5">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div><h1 className="text-2xl font-semibold text-slate-950">北向接口管理</h1><p className="mt-1 text-sm text-slate-500">集中管理公开入口、调用权限、Token、限流和 Runtime 创建策略。普通门户与已有实例不受北向开关影响。</p></div>
          <button onClick={() => void load()} disabled={busy} className="flex items-center gap-2 rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm"><RefreshCw size={16} />刷新</button>
        </div>
        {error && <div className="rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700">{error}</div>}
        {message && <div className="rounded-xl border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-700">{message}</div>}

        <section className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">{[
          ['有效会话', overview.stats.active_sessions], ['排队任务', overview.stats.queued_operations],
          ['处理中任务', overview.stats.processing_operations], ['失败任务', overview.stats.failed_operations],
        ].map(([label,value]) => <div key={label} className="rounded-2xl border border-slate-200 bg-white p-4"><div className="text-sm text-slate-500">{label}</div><div className="mt-2 text-2xl font-semibold text-slate-950">{value}</div></div>)}</section>

        <section className="grid gap-4 lg:grid-cols-3">
          <div className="rounded-2xl border border-slate-200 bg-white p-5 lg:col-span-2">
            <div className="flex items-center justify-between gap-4"><div><h2 className="font-semibold">北向 API 状态</h2><p className="mt-1 text-sm text-slate-500">关闭后仅北向 API 返回 503，Gateway 仍保持健康，普通门户和已有 Runtime 不停止。</p></div><button type="button" onClick={() => update('api_enabled', !settings.api_enabled)} className={`relative h-7 w-12 rounded-full transition ${settings.api_enabled ? 'bg-emerald-500' : 'bg-slate-300'}`}><span className={`absolute top-1 h-5 w-5 rounded-full bg-white transition ${settings.api_enabled ? 'left-6' : 'left-1'}`} /></button></div>
            <label className="mt-5 flex items-center gap-3 text-sm"><input type="checkbox" checked={settings.require_explicit_callers} onChange={(event) => update('require_explicit_callers', event.target.checked)} />仅允许下方明确授权的调用方登录</label>
          </div>
          <div className={`rounded-2xl border p-5 ${overview.cluster.available ? 'border-emerald-200 bg-emerald-50' : 'border-amber-200 bg-amber-50'}`}><div className="flex items-center gap-2 font-semibold"><Network size={18} />集群控制</div><div className="mt-3 text-sm">{overview.cluster.available ? `${overview.cluster.namespace}/${overview.cluster.service_name}` : '当前环境不可连接 Kubernetes'}</div><div className="mt-1 text-xs text-slate-500">内部端口固定：Gateway {overview.cluster.gateway_port} / Core {overview.cluster.core_port}</div></div>
        </section>

        <section className="grid gap-4 lg:grid-cols-2">
          <div className="rounded-2xl border border-slate-200 bg-white p-5"><h2 className="font-semibold">外部访问端口</h2><p className="mt-1 text-sm text-slate-500">只修改北向 Gateway 的外部 NodePort。保存前检查集群端口冲突，不改内部监听端口。</p><div className="mt-4 flex gap-3"><input type="number" min={30000} max={32767} value={nodePort} onChange={(event) => setNodePort(Number(event.target.value))} className="w-40 rounded-lg border border-slate-200 px-3 py-2" /><button onClick={() => void savePort()} disabled={busy || !overview.cluster.available} className="rounded-lg bg-slate-900 px-4 py-2 text-sm text-white disabled:opacity-40">检查并修改</button></div></div>
          <div className="rounded-2xl border border-slate-200 bg-white p-5"><div className="flex items-center justify-between"><h2 className="flex items-center gap-2 font-semibold"><ShieldCheck size={18} />现有内部 CA</h2>{cert.available && <button onClick={() => void northboundSettingsService.downloadPublicCA()} className="flex items-center gap-2 rounded-lg border px-3 py-2 text-sm"><Download size={15} />下载当前公共 CA</button>}</div><div className="mt-3 text-sm text-slate-600">{cert.available ? `有效至 ${cert.not_after ? new Date(cert.not_after).toLocaleString() : '-'}，剩余 ${cert.days_remaining} 天` : cert.error || '证书状态不可用'}</div><div className="mt-2 break-all text-xs text-slate-400">{cert.sha256}</div><div className={`mt-3 flex gap-2 rounded-lg p-3 text-xs ${cert.managed ? 'bg-emerald-50 text-emerald-700' : 'bg-amber-50 text-amber-800'}`}><TriangleAlert size={16} className="shrink-0" />{cert.management_note}</div></div>
        </section>

        <section className="rounded-2xl border border-slate-200 bg-white p-5"><h2 className="font-semibold">内部 CA 纳管与续签</h2><p className="mt-1 text-sm text-slate-500">两阶段操作：准备只创建暂存 Secret，不影响当前入口；调用方信任新公共 CA 后才允许激活。私钥始终只保存在 Kubernetes Secret，页面无法读取。</p><div className="mt-4 grid gap-3 lg:grid-cols-[2fr_1fr_150px_auto]"><label className="text-sm">DNS SAN（逗号分隔）<input value={certificateDNS} onChange={(event) => setCertificateDNS(event.target.value)} className="mt-1 w-full rounded-lg border border-slate-200 px-3 py-2" placeholder="northbound.example.internal" /></label><label className="text-sm">IP SAN（逗号分隔）<input value={certificateIPs} onChange={(event) => setCertificateIPs(event.target.value)} className="mt-1 w-full rounded-lg border border-slate-200 px-3 py-2" placeholder="10.130.15.40" /></label><NumberField label="证书有效期" value={certificateDays} onChange={setCertificateDays} suffix="天" /><button onClick={() => void prepareCertificate()} disabled={busy || !overview.cluster.available} className="self-end rounded-lg bg-slate-900 px-4 py-2 text-sm text-white disabled:opacity-40">准备托管证书</button></div>{cert.prepared && <div className="mt-4 flex flex-wrap items-center gap-3 rounded-xl border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900"><span>已有暂存证书，有效至 {cert.prepared_not_after ? new Date(cert.prepared_not_after).toLocaleString() : '-'}</span><button onClick={() => void northboundSettingsService.downloadPreparedCA()} className="rounded-lg border border-amber-300 bg-white px-3 py-1.5">下载新公共 CA</button><button onClick={() => void activateCertificate()} disabled={busy} className="rounded-lg bg-red-600 px-3 py-1.5 text-white">已分发 CA，确认激活</button></div>}</section>

        <section className="rounded-2xl border border-slate-200 bg-white p-5"><h2 className="font-semibold">Token、限流与异步任务</h2><div className="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-4"><NumberField label="Challenge 有效期" value={settings.challenge_ttl_seconds} onChange={(v) => update('challenge_ttl_seconds', v)} suffix="秒" /><NumberField label="Access Token" value={settings.access_token_ttl_seconds} onChange={(v) => update('access_token_ttl_seconds', v)} suffix="秒" /><NumberField label="Refresh Token" value={settings.refresh_token_ttl_seconds} onChange={(v) => update('refresh_token_ttl_seconds', v)} suffix="秒" /><NumberField label="Core 请求超时" value={settings.core_request_timeout_seconds} onChange={(v) => update('core_request_timeout_seconds', v)} suffix="秒" /><NumberField label="Challenge / IP" value={settings.challenge_rate_per_minute} onChange={(v) => update('challenge_rate_per_minute', v)} suffix="次/分" /><NumberField label="登录 / IP" value={settings.login_rate_per_minute} onChange={(v) => update('login_rate_per_minute', v)} suffix="次/分" /><NumberField label="创建 / 用户" value={settings.create_rate_per_minute} onChange={(v) => update('create_rate_per_minute', v)} suffix="次/分" /><NumberField label="查询 / 用户" value={settings.query_rate_per_minute} onChange={(v) => update('query_rate_per_minute', v)} suffix="次/分" /><NumberField label="ShareLink / 用户" value={settings.share_rate_per_minute} onChange={(v) => update('share_rate_per_minute', v)} suffix="次/分" /><NumberField label="每用户待处理任务" value={settings.max_pending_operations} onChange={(v) => update('max_pending_operations', v)} /><NumberField label="任务租约" value={settings.operation_lease_seconds} onChange={(v) => update('operation_lease_seconds', v)} suffix="秒" /><NumberField label="最大重试" value={settings.operation_max_attempts} onChange={(v) => update('operation_max_attempts', v)} suffix="次" /></div></section>

        <section className="rounded-2xl border border-slate-200 bg-white p-5"><h2 className="font-semibold">Runtime 创建策略</h2><p className="mt-1 text-sm text-slate-500">镜像地址继续复用“设置”中的系统镜像，不在这里保存第二份。</p><div className="mt-4 grid gap-5 lg:grid-cols-2"><RuntimeSelector title="允许通过 Lite 接口创建" values={settings.allowed_lite_types} choices={runtimes} onChange={(v) => update('allowed_lite_types', v)} /><RuntimeSelector title="允许通过 Pro 接口创建" values={settings.allowed_pro_types} choices={proRuntimes} onChange={(v) => update('allowed_pro_types', v)} /></div><div className="mt-5 grid gap-4 lg:grid-cols-3">{([['Lite','lite'],['普通 Pro','pro'],['WorkBuddy Pro','workbuddy_pro']] as const).map(([title,prefix]) => <div key={prefix} className="rounded-xl bg-slate-50 p-4"><div className="mb-3 font-medium">{title}</div><div className="grid grid-cols-3 gap-2"><NumberField label="CPU" value={settings[`${prefix}_cpu_cores`]} onChange={(v) => update(`${prefix}_cpu_cores`,v)} step={0.5} /><NumberField label="内存" value={settings[`${prefix}_memory_gb`]} onChange={(v) => update(`${prefix}_memory_gb`,v)} suffix="GiB" /><NumberField label="磁盘" value={settings[`${prefix}_disk_gb`]} onChange={(v) => update(`${prefix}_disk_gb`,v)} suffix="GiB" /></div></div>)}</div></section>

        <section className="rounded-2xl border border-slate-200 bg-white p-5"><h2 className="font-semibold">调用方授权</h2><p className="mt-1 text-sm text-slate-500">权限变更会立即撤销该用户已有北向会话，避免旧 Token 继续保留过期权限。</p><div className="mt-4 grid gap-4 md:grid-cols-[minmax(220px,1fr)_2fr_auto]"><select value={selectedUserID} onChange={(event) => selectCaller(Number(event.target.value))} className="rounded-lg border border-slate-200 px-3 py-2"><option value={0}>选择 ClawManager 用户</option>{users.map((user) => <option key={user.id} value={user.id}>{user.username} · {user.email}</option>)}</select><div className="flex flex-wrap gap-2">{scopes.map((scope) => <label key={scope} className="flex items-center gap-1.5 rounded-full border border-slate-200 px-3 py-1.5 text-xs"><input type="checkbox" disabled={!selectedUserID} checked={caller.scopes.includes(scope)} onChange={(event) => setCaller((current) => ({ ...current, scopes: event.target.checked ? [...current.scopes,scope] : current.scopes.filter((value) => value!==scope) }))} />{scope}</label>)}</div><div className="flex items-center gap-3"><label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={caller.enabled} disabled={!selectedUserID} onChange={(event) => setCaller((current) => ({...current,enabled:event.target.checked}))} />启用</label><button onClick={() => void saveCaller()} disabled={busy || !selectedUserID} className="rounded-lg bg-slate-900 px-4 py-2 text-sm text-white disabled:opacity-40">保存权限</button></div></div>{selectedUser && <div className="mt-3 text-xs text-slate-400">正在编辑：{selectedUser.username}（{selectedUser.email}）</div>}</section>

        <section className="rounded-2xl border border-slate-200 bg-white p-5"><h2 className="font-semibold">最近配置审计</h2><div className="mt-3 divide-y divide-slate-100">{overview.audit.length === 0 ? <div className="py-3 text-sm text-slate-400">暂无修改记录</div> : overview.audit.slice(0, 10).map((item) => <div key={item.id} className="flex items-center justify-between gap-4 py-3 text-sm"><span><span className="font-medium">{item.actor || '系统'}</span> · {item.action}</span><span className="text-xs text-slate-400">{new Date(item.created_at).toLocaleString()}</span></div>)}</div></section>

        <div className="sticky bottom-4 flex justify-end"><button onClick={() => void saveSettings()} disabled={busy} className="flex items-center gap-2 rounded-xl bg-red-600 px-5 py-3 font-medium text-white shadow-lg disabled:opacity-50"><Save size={17} />保存北向配置</button></div>
      </div>
    </AdminLayout>
  );
};

export default NorthboundSettingsPage;
