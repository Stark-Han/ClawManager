import React, { useCallback, useEffect, useState } from 'react';
import { CheckCircle2, ExternalLink, FlaskConical, RefreshCw, Rocket, Trash2, XCircle } from 'lucide-react';
import { Link } from 'react-router-dom';
import AdminLayout from '../../components/AdminLayout';
import { openClawUpgradeLabService } from '../../services/openClawUpgradeLabService';
import type { OpenClawUpgradeLabView } from '../../types/openClawUpgradeLab';

const terminalStatuses = new Set(['finished', 'verification_failed', 'failed', 'cleaned']);

function errorMessage(error: unknown): string {
  if (typeof error === 'object' && error !== null) {
    const candidate = error as { response?: { data?: { message?: string } }; message?: string };
    return candidate.response?.data?.message || candidate.message || '操作失败';
  }
  return String(error || '操作失败');
}

const OpenClawUpgradeLabPage: React.FC = () => {
  const [view, setView] = useState<OpenClawUpgradeLabView | null>(null);
  const [instanceCount, setInstanceCount] = useState(1);
  const [batchSize, setBatchSize] = useState(1);
  const [targetImage, setTargetImage] = useState('');
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadLatest = useCallback(async () => {
    try {
      setError(null);
      setView(await openClawUpgradeLabService.latest());
    } catch (loadError) {
      setError(errorMessage(loadError));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    const timer = window.setTimeout(() => void loadLatest(), 0);
    return () => window.clearTimeout(timer);
  }, [loadLatest]);

  const activeRunID = view?.run?.id;
  const activeRunStatus = view?.run?.status;
  useEffect(() => {
    if (!activeRunID || !activeRunStatus || terminalStatuses.has(activeRunStatus) || !['provisioning', 'preflighting', 'upgrading'].includes(activeRunStatus)) {
      return undefined;
    }
    const timer = window.setInterval(async () => {
      try {
        setView(await openClawUpgradeLabService.get(activeRunID));
      } catch (pollError) {
        setError(errorMessage(pollError));
      }
    }, 2000);
    return () => window.clearInterval(timer);
  }, [activeRunID, activeRunStatus]);

  const createBaseline = async () => {
    setBusy(true);
    setError(null);
    try {
      setView(await openClawUpgradeLabService.createBaseline(instanceCount));
    } catch (createError) {
      setError(errorMessage(createError));
      await loadLatest();
    } finally {
      setBusy(false);
    }
  };

  const resetBaseline = async () => {
    if (!view?.run) return;
    setBusy(true);
    setError(null);
    try {
      setView(await openClawUpgradeLabService.reset(view.run.id, instanceCount));
    } catch (resetError) {
      setError(errorMessage(resetError));
    } finally {
      setBusy(false);
    }
  };

  const cleanup = async () => {
    if (!view?.run) return;
    setBusy(true);
    setError(null);
    try {
      await openClawUpgradeLabService.cleanup(view.run.id);
      await loadLatest();
    } catch (cleanupError) {
      setError(errorMessage(cleanupError));
    } finally {
      setBusy(false);
    }
  };

  const startUpgrade = async () => {
    if (!view?.run || !targetImage.trim()) {
      setError('请输入要测试的OpenClaw 8.1镜像');
      return;
    }
    setBusy(true);
    setError(null);
    try {
      setView(await openClawUpgradeLabService.startUpgrade(view.run.id, targetImage.trim(), batchSize));
    } catch (upgradeError) {
      setError(errorMessage(upgradeError));
      await loadLatest();
    } finally {
      setBusy(false);
    }
  };

  const captureBaseline = async () => {
    if (!view?.run) return;
    setBusy(true);
    setError(null);
    try {
      setView(await openClawUpgradeLabService.captureBaseline(view.run.id));
    } catch (captureError) {
      setError(errorMessage(captureError));
    } finally {
      setBusy(false);
    }
  };

  const run = view?.run;
  const canReset = run && terminalStatuses.has(run.status);
  const canUpgrade = run?.status === 'ready' && run.phase === 'baseline_captured';

  return (
    <AdminLayout title="OpenClaw升级实验室">
      <div className="space-y-6">
        <section className="app-panel p-6">
          <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
            <div>
              <div className="flex items-center gap-2">
                <FlaskConical className="h-6 w-6 text-orange-600" />
                <h2 className="text-xl font-semibold text-gray-900">隔离的7.1 → 8.1升级验证</h2>
              </div>
              <p className="mt-2 max-w-3xl text-sm text-slate-600">
                测试实例只运行在带有upgrade-lab标签的独立Runtime池。普通调度、正式升级候选和其他Lite运行时不会使用该池；迁移阶段调用与正式8.1升级相同的执行核心。
              </p>
            </div>
            <Link to="/admin/settings" className="app-button-secondary inline-flex items-center gap-2">
              返回系统设置
            </Link>
          </div>
        </section>

        {error && <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div>}

        <section className="app-panel p-6">
          <h2 className="text-lg font-semibold text-gray-900">安全基线</h2>
          <div className="mt-4 grid gap-4 lg:grid-cols-2">
            <div>
              <div className="text-sm font-medium text-slate-700">固定基线Tag</div>
              <div className="mt-1 break-all rounded-md bg-slate-50 p-3 font-mono text-sm">{view?.baseline_tag || 'master-20260824-737ad4c'}</div>
            </div>
            <div>
              <div className="text-sm font-medium text-slate-700">锁定Digest</div>
              <div className="mt-1 break-all rounded-md bg-slate-50 p-3 font-mono text-sm">{run?.baseline_image_digest || 'sha256:c0905d813cdf22f5ed357d9bd6f61a6798020f1d099022f0aaec4a96c83df125'}</div>
            </div>
          </div>
          <div className="mt-4 flex flex-wrap items-end gap-3">
            <label className="text-sm font-medium text-slate-700">
              测试实例数量
              <input type="number" min={1} max={8} value={instanceCount} onChange={(event) => setInstanceCount(Math.min(8, Math.max(1, Number(event.target.value) || 1)))} className="app-input mt-1 block w-32" />
            </label>
            {!run || run.status === 'cleaned' ? (
              <button type="button" disabled={busy || loading} onClick={() => void createBaseline()} className="app-button-primary inline-flex items-center gap-2 disabled:opacity-50">
                <FlaskConical className="h-4 w-4" />创建7.1基线环境
              </button>
            ) : (
              <>
                <button type="button" disabled={busy || !canReset} onClick={() => void resetBaseline()} className="app-button-secondary inline-flex items-center gap-2 disabled:opacity-50">
                  <RefreshCw className="h-4 w-4" />重新创建7.1基线
                </button>
                <button type="button" disabled={busy || ['provisioning', 'preflighting', 'upgrading'].includes(run.status)} onClick={() => void cleanup()} className="app-button-secondary inline-flex items-center gap-2 text-red-700 disabled:opacity-50">
                  <Trash2 className="h-4 w-4" />清理测试环境
                </button>
              </>
            )}
          </div>
        </section>

        {run && run.status !== 'cleaned' && (
          <section className="app-panel p-6">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <h2 className="text-lg font-semibold text-gray-900">测试批次 #{run.id}</h2>
                <p className="mt-1 text-sm text-slate-600">状态：{run.status} / 阶段：{run.phase}</p>
              </div>
              <div className="rounded-full bg-slate-100 px-3 py-1 text-xs font-medium text-slate-700">{run.source_deployment}</div>
            </div>
            {run.error_code && (
              <div className="mt-4 rounded-md border border-red-200 bg-red-50 p-4 text-sm text-red-800">
                <div className="font-semibold">{run.error_code}</div>
                <div className="mt-1 whitespace-pre-wrap">{run.error_message}</div>
              </div>
            )}
            <div className="mt-5 grid gap-4 md:grid-cols-2 xl:grid-cols-3">
              {(view?.instances || []).map((instance) => (
                <div key={instance.id} className="rounded-lg border border-slate-200 p-4">
                  <div className="font-medium text-gray-900">{instance.name}</div>
                  <div className="mt-1 text-sm text-slate-600">实例 #{instance.id} · {instance.status}</div>
                  <div className="mt-3 text-xs text-slate-500">先进入实例写入测试对话，并在工作区创建或上传测试文件，然后再执行升级。</div>
                  <Link to={`/instances/${instance.id}`} className="app-button-secondary mt-4 inline-flex items-center gap-2">
                    <ExternalLink className="h-4 w-4" />打开会话和工作区
                  </Link>
                </div>
              ))}
            </div>
            {run.status === 'ready' && (
              <div className="mt-5 flex flex-wrap items-center gap-3">
                <button type="button" disabled={busy} onClick={() => void captureBaseline()} className="app-button-primary inline-flex items-center gap-2 disabled:opacity-50">
                  <CheckCircle2 className="h-4 w-4" />检查并锁定升级前数据
                </button>
                <span className="text-sm text-slate-600">每个实例必须已有至少一轮人工问答和一个自行创建/上传的项目文件；系统心跳与内置标记文件不计入。</span>
              </div>
            )}
            {(view?.baseline_evidence || []).length > 0 && (
              <div className="mt-5 grid gap-3 md:grid-cols-2 xl:grid-cols-3">
                {view!.baseline_evidence.map((evidence) => (
                  <div key={evidence.instance_id} className="rounded-lg border border-emerald-200 bg-emerald-50 p-4 text-sm text-emerald-900">
                    <div className="font-semibold">实例 #{evidence.instance_id} 基线已锁定</div>
                    <div className="mt-2">会话 {evidence.session_count} 个 · 人工提问 {evidence.interactive_user_message_count} 条 · 助手回复 {evidence.assistant_message_count} 条</div>
                    <div className="mt-1">项目文件 {evidence.project_file_count} 个（人工文件 {evidence.manual_project_file_count} 个）</div>
                    <div className="mt-2 break-all font-mono text-xs">Session {evidence.session_catalog_sha256}</div>
                  </div>
                ))}
              </div>
            )}
          </section>
        )}

        {run && run.status !== 'cleaned' && (
          <section className="app-panel p-6">
            <h2 className="text-lg font-semibold text-gray-900">执行8.1专用升级</h2>
            <p className="mt-1 text-sm text-slate-600">升级仅在7.1人工问答与项目文件基线锁定后开放；完成后同时核对项目文件、原始Session归档和8.1官方Session目录。</p>
            <div className="mt-4 grid gap-4 lg:grid-cols-[minmax(320px,1fr)_160px_auto] lg:items-end">
              <label className="text-sm font-medium text-slate-700">
                新测试镜像
                <input value={targetImage} onChange={(event) => setTargetImage(event.target.value)} placeholder="registry/repository:tag 或 @sha256:..." className="app-input mt-1 block w-full font-mono" />
              </label>
              <label className="text-sm font-medium text-slate-700">
                实例迁移并发
                <input type="number" min={1} max={8} value={batchSize} onChange={(event) => setBatchSize(Math.min(8, Math.max(1, Number(event.target.value) || 1)))} className="app-input mt-1 block w-full" />
              </label>
              <button type="button" disabled={busy || !canUpgrade} onClick={() => void startUpgrade()} className="app-button-primary inline-flex items-center justify-center gap-2 disabled:opacity-50">
                <Rocket className="h-4 w-4" />开始隔离升级测试
              </button>
            </div>
          </section>
        )}

        {view?.rollout && (
          <section className="app-panel p-6">
            <h2 className="text-lg font-semibold text-gray-900">升级过程</h2>
            <div className="mt-3 rounded-md bg-blue-50 p-4 text-sm text-blue-900">
              升级 #{view.rollout.rollout.id}：{view.rollout.rollout.status} / {view.rollout.rollout.phase}
            </div>
            <div className="mt-4 grid gap-2">
              {view.rollout.items.map((item) => (
                <div key={item.id} className="flex items-center justify-between rounded-md border border-slate-200 px-3 py-2 text-sm">
                  <span>实例 #{item.instance_id}</span><span>{item.state}</span>
                </div>
              ))}
            </div>
          </section>
        )}

        {(view?.checks || []).length > 0 && (
          <section className="app-panel p-6">
            <h2 className="text-lg font-semibold text-gray-900">升级前后验收</h2>
            <div className="mt-4 space-y-3">
              {view!.checks.map((check) => (
                <div key={check.name} className={`rounded-md border p-4 ${check.passed ? 'border-emerald-200 bg-emerald-50' : 'border-red-200 bg-red-50'}`}>
                  <div className="flex items-center gap-2 font-medium">
                    {check.passed ? <CheckCircle2 className="h-5 w-5 text-emerald-600" /> : <XCircle className="h-5 w-5 text-red-600" />}
                    {check.name}
                  </div>
                  {check.message && <div className="mt-1 text-sm text-slate-700">{check.message}</div>}
                  {check.expected && <div className="mt-2 break-all font-mono text-xs">升级前：{check.expected}</div>}
                  {check.actual && <div className="mt-1 break-all font-mono text-xs">升级后：{check.actual}</div>}
                </div>
              ))}
            </div>
          </section>
        )}
      </div>
    </AdminLayout>
  );
};

export default OpenClawUpgradeLabPage;
