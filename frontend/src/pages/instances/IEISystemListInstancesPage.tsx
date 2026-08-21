import axios from "axios";
import {
  ArrowRight,
  Box,
  CheckCircle2,
  ChevronRight,
  LogOut,
  RefreshCw,
  Search,
  ShieldCheck,
  SlidersHorizontal,
  Sparkles,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { getIEIRuntimePresentation } from "../../lib/ieiRuntimeCatalog";
import {
  ieiSystemService,
  type IEISystemInstance,
  type IEISystemSession,
} from "../../services/ieiSystemService";

const PAGE_SIZE = 100;

function statusClass(status: string) {
  switch (status.toLowerCase()) {
    case "running":
      return "border-emerald-200 bg-emerald-50 text-emerald-700";
    case "creating":
      return "border-amber-200 bg-amber-50 text-amber-700";
    case "error":
      return "border-red-200 bg-red-50 text-red-700";
    default:
      return "border-slate-200 bg-slate-50 text-slate-600";
  }
}

function statusLabel(status: string) {
  switch (status.toLowerCase()) {
    case "running":
      return "运行中";
    case "creating":
      return "创建中";
    case "stopped":
      return "已停止";
    case "error":
      return "异常";
    default:
      return status;
  }
}

function InstanceTypeIcon({
  type,
  className = "h-full w-full",
}: {
  type: string;
  className?: string;
}) {
  const normalizedType = type.trim().toLowerCase();
  if (normalizedType === "openclaw") {
    return <img src="/openclaw.png" alt="OpenClaw" className={`${className} object-contain`} />;
  }
  if (normalizedType === "hermes") {
    return <img src="/hermes.png" alt="Hermes" className={`${className} object-contain`} />;
  }
  if (normalizedType === "opencode") {
    return <img src="/opencode.png" alt="OpenCode" className={`${className} object-contain`} />;
  }
  if (normalizedType === "deepseek-harness") {
    return (
      <img
        src="/deepseek-harness.svg"
        alt="DeepSeek Harness"
        className={`${className} object-contain`}
      />
    );
  }
  if (normalizedType === "workbuddy") {
    return <img src="/workbuddy.png" alt="WorkBuddy" className={`${className} object-contain`} />;
  }
  return <Box aria-label={type || "Instance"} className={`${className} text-slate-500`} />;
}

function errorMessage(error: unknown) {
  if (axios.isAxiosError(error) && typeof error.response?.data?.error === "string") {
    return error.response.data.error;
  }
  return "无法加载实例，请稍后重试。";
}

function installNoReferrerPolicy() {
  const existing = document.querySelector<HTMLMetaElement>('meta[name="referrer"]');
  const previous = existing?.content;
  const meta = existing ?? document.createElement("meta");
  if (!existing) {
    meta.name = "referrer";
    document.head.appendChild(meta);
  }
  meta.content = "no-referrer";
  return () => {
    if (existing) meta.content = previous ?? "";
    else meta.remove();
  };
}

function formatTime(value?: string) {
  if (!value) return "—";
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

export default function IEISystemListInstancesPage() {
  const initialized = useRef(false);
  const [session, setSession] = useState<IEISystemSession | null>(null);
  const [instances, setInstances] = useState<IEISystemInstance[]>([]);
  const [selectedID, setSelectedID] = useState<number | null>(null);
  const [query, setQuery] = useState("");
  const [runtimeFilter, setRuntimeFilter] = useState("all");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadInstances = useCallback(async () => {
    const first = await ieiSystemService.listInstances(1, PAGE_SIZE);
    const items = [...(first.instances ?? [])];
    const pages = Math.ceil(first.total / PAGE_SIZE);
    for (let page = 2; page <= pages; page += 1) {
      const next = await ieiSystemService.listInstances(page, PAGE_SIZE);
      items.push(...(next.instances ?? []));
    }
    setInstances(items);
    setSelectedID((current) =>
      current && items.some((instance) => instance.id === current)
        ? current
        : (items[0]?.id ?? null),
    );
  }, []);

  const initialize = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const params = new URLSearchParams(window.location.search);
      const token = params.get("token")?.trim();
      if (token) {
        window.history.replaceState({}, "", `${window.location.pathname}${window.location.hash}`);
      }
      const nextSession = token
        ? await ieiSystemService.exchangeSession(token)
        : await ieiSystemService.getSession();
      setSession(nextSession);
      await loadInstances();
    } catch (loadError) {
      setSession(null);
      setInstances([]);
      setSelectedID(null);
      setError(errorMessage(loadError));
    } finally {
      setLoading(false);
    }
  }, [loadInstances]);

  useEffect(() => {
    if (initialized.current) return;
    initialized.current = true;
    void initialize();
  }, [initialize]);

  useEffect(() => installNoReferrerPolicy(), []);

  const runtimeOptions = useMemo(() => {
    const types = [...new Set(instances.map((instance) => instance.type.trim().toLowerCase()))];
    return types.map((type) => ({ type, label: getIEIRuntimePresentation(type).name }));
  }, [instances]);

  const visibleInstances = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase();
    return instances.filter((instance) => {
      const matchesRuntime = runtimeFilter === "all" || instance.type.toLowerCase() === runtimeFilter;
      const matchesQuery =
        !normalizedQuery ||
        instance.name.toLowerCase().includes(normalizedQuery) ||
        String(instance.id).includes(normalizedQuery) ||
        getIEIRuntimePresentation(instance.type).name.toLowerCase().includes(normalizedQuery);
      return matchesRuntime && matchesQuery;
    });
  }, [instances, query, runtimeFilter]);

  const selectedInstance =
    instances.find((instance) => instance.id === selectedID) ??
    visibleInstances[0] ??
    instances[0] ??
    null;
  const selectedRuntime = selectedInstance
    ? getIEIRuntimePresentation(selectedInstance.type)
    : null;
  const handleLogout = async () => {
    await ieiSystemService.clearSession().catch(() => undefined);
    setSession(null);
    setInstances([]);
    setSelectedID(null);
    setError("访问会话已退出，请从智慧协作平台重新进入。");
  };

  return (
    <main className="flex min-h-screen flex-col bg-[#f4f7fb] text-slate-900">
      <header className="shrink-0 border-b border-slate-200 bg-white/95 shadow-[0_1px_16px_rgba(15,23,42,0.025)] backdrop-blur">
        <div className="flex min-h-[92px] items-center justify-between gap-6 px-7 py-4 lg:px-9">
          <div className="flex min-w-0 items-center gap-7">
            <div className="flex shrink-0 items-center gap-3">
              <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-gradient-to-br from-blue-600 to-cyan-500 text-white shadow-[0_8px_22px_rgba(37,99,235,0.22)]">
                <ShieldCheck className="h-7 w-7" strokeWidth={2.2} />
              </div>
              <div>
                <div className="text-[25px] font-bold leading-6 tracking-tight text-[#12203a]">IEI</div>
                <div className="mt-1 text-[10px] font-semibold tracking-[0.14em] text-slate-400">
                  OWNER PORTAL
                </div>
              </div>
            </div>
            <div className="hidden h-10 w-px bg-slate-200 sm:block" />
            {session ? (
              <div className="hidden min-w-0 sm:block">
                <div className="flex items-center gap-2 text-sm font-semibold text-slate-800">
                  <CheckCircle2 className="h-4 w-4 text-emerald-500" />
                  已验证所有者
                </div>
                <p className="mt-1 truncate text-sm text-slate-500">{session.owner}</p>
              </div>
            ) : null}
          </div>
          {session ? (
            <div className="flex items-center gap-3">
              <button
                type="button"
                className="inline-flex h-11 items-center gap-2 rounded-lg border border-slate-200 bg-white px-4 text-sm font-semibold text-slate-700 transition hover:border-blue-300 hover:text-blue-700 disabled:opacity-60"
                onClick={() => void initialize()}
                disabled={loading}
              >
                <RefreshCw className={`h-4 w-4 ${loading ? "animate-spin" : ""}`} />
                刷新
              </button>
              <button
                type="button"
                className="inline-flex h-11 items-center gap-2 rounded-lg border border-slate-200 bg-white px-4 text-sm font-semibold text-slate-700 transition hover:border-red-200 hover:text-red-600"
                onClick={() => void handleLogout()}
              >
                <LogOut className="h-4 w-4" />
                退出
              </button>
            </div>
          ) : null}
        </div>
      </header>

      <section className="min-h-0 flex-1 p-4 lg:p-5">
        {error ? (
          <div className="mx-auto mt-16 max-w-lg rounded-2xl border border-red-200 bg-white p-7 text-center shadow-sm">
            <h2 className="text-lg font-semibold">访问验证失败</h2>
            <p className="mt-2 text-sm leading-6 text-slate-600">{error}</p>
          </div>
        ) : loading ? (
          <div className="flex h-[65vh] items-center justify-center text-sm text-slate-600">
            <RefreshCw className="mr-2 h-5 w-5 animate-spin" />
            正在验证身份并加载实例…
          </div>
        ) : instances.length === 0 ? (
          <div className="mx-auto mt-16 flex max-w-xl flex-col items-center rounded-2xl border border-slate-200 bg-white px-6 py-16 text-center shadow-sm">
            <Box className="h-10 w-10 text-slate-300" />
            <h2 className="mt-4 text-lg font-semibold">暂无实例</h2>
            <p className="mt-1 text-sm text-slate-500">当前 owner 下没有可展示的实例。</p>
          </div>
        ) : selectedInstance && selectedRuntime ? (
          <div className="grid gap-4 lg:h-[calc(100vh-132px)] lg:min-h-[640px] lg:grid-cols-[minmax(280px,330px)_minmax(460px,1fr)_minmax(280px,320px)] 2xl:grid-cols-[minmax(320px,380px)_minmax(560px,1fr)_minmax(320px,360px)]">
            <aside className="flex min-h-[680px] flex-col overflow-hidden rounded-xl border border-slate-200 bg-white shadow-[0_12px_35px_rgba(15,23,42,0.045)] lg:min-h-0">
              <div className="flex items-center justify-between border-b border-slate-100 px-5 py-5">
                <h2 className="text-base font-bold text-[#14213a]">我的实例 · {instances.length}</h2>
                <button
                  type="button"
                  className={`cm-icon-button h-9 w-9 ${showFilters ? "border-blue-200 bg-blue-50 text-blue-600" : ""}`}
                  title="筛选运行时"
                  onClick={() => setShowFilters((current) => !current)}
                >
                  <SlidersHorizontal className="h-4 w-4" />
                </button>
              </div>
              <div className="space-y-3 px-4 py-4">
                <label className="relative block">
                  <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-400" />
                  <input
                    value={query}
                    onChange={(event) => setQuery(event.target.value)}
                    placeholder="搜索实例名称或 ID…"
                    className="h-11 w-full rounded-lg border border-slate-200 bg-slate-50/60 pl-10 pr-3 text-sm outline-none transition placeholder:text-slate-400 focus:border-blue-300 focus:bg-white focus:ring-2 focus:ring-blue-100"
                  />
                </label>
                {showFilters ? (
                  <select
                    aria-label="筛选运行时"
                    value={runtimeFilter}
                    onChange={(event) => setRuntimeFilter(event.target.value)}
                    className="h-10 w-full rounded-lg border border-slate-200 bg-white px-3 text-sm text-slate-700 outline-none focus:border-blue-300"
                  >
                    <option value="all">全部 Runtime</option>
                    {runtimeOptions.map((option) => (
                      <option key={option.type} value={option.type}>
                        {option.label}
                      </option>
                    ))}
                  </select>
                ) : null}
              </div>
              <div className="min-h-0 flex-1 space-y-3 overflow-y-auto px-4 pb-4">
                {visibleInstances.length === 0 ? (
                  <div className="rounded-xl border border-dashed border-slate-200 px-4 py-10 text-center text-sm text-slate-500">
                    没有符合条件的实例
                  </div>
                ) : (
                  visibleInstances.map((instance) => {
                    const runtime = getIEIRuntimePresentation(instance.type);
                    const selected = instance.id === selectedInstance.id;
                    return (
                      <button
                        key={instance.id}
                        type="button"
                        onClick={() => setSelectedID(instance.id)}
                        className={`group w-full rounded-xl border p-4 text-left transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-200 ${
                          selected
                            ? runtime.theme.selection
                            : "border-slate-200 bg-white hover:border-slate-300 hover:bg-slate-50"
                        }`}
                      >
                        <div className="flex items-start gap-3">
                          <div
                            className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border ${runtime.theme.border} ${runtime.theme.accentSoft} p-1.5`}
                          >
                            <InstanceTypeIcon type={instance.type} />
                          </div>
                          <div className="min-w-0 flex-1">
                            <div className="flex items-start justify-between gap-2">
                              <h3 className="truncate text-[15px] font-bold text-slate-900">
                                {instance.name}
                              </h3>
                              <span
                                className={`shrink-0 rounded-full border px-2 py-0.5 text-[10px] font-semibold ${statusClass(instance.status)}`}
                              >
                                {statusLabel(instance.status)}
                              </span>
                            </div>
                            <p className="mt-1 text-xs text-slate-500">
                              {runtime.name} · #{instance.id}
                            </p>
                            <div className="mt-3 flex items-center gap-2 text-[11px] text-slate-400">
                              <span className={`h-2 w-2 rounded-full ${runtime.theme.dot}`} />
                              更新于 {formatTime(instance.updated_at)}
                            </div>
                          </div>
                        </div>
                      </button>
                    );
                  })
                )}
              </div>
            </aside>

            <section className="flex min-h-[680px] flex-col overflow-hidden rounded-xl border border-slate-200 bg-white shadow-[0_12px_35px_rgba(15,23,42,0.045)] lg:min-h-0">
              <div className="border-b border-slate-100 px-6 py-5">
                <h2 className="text-base font-bold text-[#14213a]">实例详情</h2>
              </div>
              <div className="flex-1 overflow-y-auto p-6">
                <div className="flex flex-col gap-5 border-b border-slate-100 pb-6 sm:flex-row sm:items-center">
                  <div
                    className={`flex h-24 w-24 shrink-0 items-center justify-center rounded-2xl border ${selectedRuntime.theme.border} ${selectedRuntime.theme.accentSoft} p-4`}
                  >
                    <InstanceTypeIcon type={selectedInstance.type} />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-3">
                      <h1 className="truncate text-2xl font-bold tracking-tight text-[#10203b]">
                        {selectedInstance.name}
                      </h1>
                      <span
                        className={`rounded-full border px-2.5 py-1 text-xs font-semibold ${statusClass(selectedInstance.status)}`}
                      >
                        {statusLabel(selectedInstance.status)}
                      </span>
                    </div>
                    <p className="mt-2 text-base text-slate-500">
                      {selectedRuntime.name} · #{selectedInstance.id}
                    </p>
                    <p
                      className={`mt-2 flex items-center gap-2 text-sm font-medium ${selectedRuntime.theme.accent}`}
                    >
                      <span className={`h-2 w-2 rounded-full ${selectedRuntime.theme.dot}`} />
                      {selectedRuntime.category}
                    </p>
                  </div>
                  <Link
                    to={`/ieisystem/instances/${selectedInstance.id}`}
                    className="inline-flex h-12 shrink-0 items-center justify-center gap-2 rounded-lg bg-gradient-to-r from-blue-700 to-blue-600 px-6 text-sm font-semibold text-white shadow-[0_10px_24px_rgba(37,99,235,0.22)] transition hover:-translate-y-0.5 hover:from-blue-800 hover:to-blue-700"
                  >
                    进入实例
                    <ArrowRight className="h-4 w-4" />
                  </Link>
                </div>

                <div className="mt-6 overflow-hidden rounded-xl border border-slate-200">
                  <div className="border-b border-slate-200 bg-slate-50/70 px-5 py-3 text-sm font-bold text-slate-800">
                    实例信息
                  </div>
                  <dl className="divide-y divide-slate-100 px-5">
                    {[
                      ["实例名称", selectedInstance.name],
                      ["实例 ID", `#${selectedInstance.id}`],
                      ["运行时", selectedRuntime.name],
                      ["运行时类型", selectedRuntime.category],
                      ["状态", statusLabel(selectedInstance.status)],
                      ["更新时间", formatTime(selectedInstance.updated_at)],
                    ].map(([label, value]) => (
                      <div
                        key={label}
                        className="grid grid-cols-[120px_minmax(0,1fr)] gap-5 py-3 text-sm"
                      >
                        <dt className="text-slate-500">{label}</dt>
                        <dd className="font-medium text-slate-700">{value}</dd>
                      </div>
                    ))}
                  </dl>
                </div>

                <div className="mt-6">
                  <h3 className="text-sm font-bold text-slate-900">实例描述</h3>
                  <p className="mt-2 text-sm leading-7 text-slate-600">
                    {selectedInstance.description?.trim() || selectedRuntime.positioning}
                  </p>
                </div>
                <div className="mt-6">
                  <h3 className="text-sm font-bold text-slate-900">能力标签</h3>
                  <div className="mt-3 flex flex-wrap gap-2">
                    {selectedRuntime.capabilities.map((capability) => (
                      <span
                        key={capability}
                        className={`rounded-md border ${selectedRuntime.theme.border} ${selectedRuntime.theme.accentSoft} px-3 py-1.5 text-xs font-medium ${selectedRuntime.theme.accent}`}
                      >
                        {capability}
                      </span>
                    ))}
                  </div>
                </div>
              </div>
            </section>

            <aside className="flex min-h-[680px] flex-col overflow-hidden rounded-xl border border-slate-200 bg-white shadow-[0_12px_35px_rgba(15,23,42,0.045)] lg:min-h-0">
              <div className="border-b border-slate-100 px-5 py-5">
                <h2 className="text-base font-bold text-[#14213a]">运行时说明</h2>
              </div>
              <div className="flex-1 overflow-y-auto px-5 py-6">
                <div className="text-center">
                  <div
                    className={`mx-auto flex h-20 w-20 items-center justify-center rounded-full ${selectedRuntime.theme.accentSoft} p-4`}
                  >
                    <InstanceTypeIcon type={selectedInstance.type} />
                  </div>
                  <div className="mt-4 flex flex-wrap items-center justify-center gap-2">
                    <h3 className="text-xl font-bold text-[#10203b]">{selectedRuntime.name}</h3>
                    {selectedRuntime.badge ? (
                      <span className="rounded-full bg-gradient-to-r from-teal-500 to-cyan-500 px-2 py-0.5 text-[10px] font-bold tracking-wider text-white">
                        {selectedRuntime.badge}
                      </span>
                    ) : null}
                  </div>
                  <p
                    className={`mt-2 flex items-center justify-center gap-2 text-sm font-medium ${selectedRuntime.theme.accent}`}
                  >
                    <span className={`h-2 w-2 rounded-full ${selectedRuntime.theme.dot}`} />
                    {selectedRuntime.tagline}
                  </p>
                </div>

                <div className="mt-7">
                  <h4 className="text-sm font-bold text-slate-900">运行时定位</h4>
                  <p className="mt-2 text-sm leading-7 text-slate-600">
                    {selectedRuntime.positioning}
                  </p>
                </div>

                <div className="mt-7">
                  <h4 className="text-sm font-bold text-slate-900">核心能力</h4>
                  <ul className="mt-3 space-y-2.5">
                    {selectedRuntime.capabilities.map((capability) => (
                      <li key={capability} className="flex items-center gap-2 text-sm text-slate-600">
                        <ChevronRight className={`h-4 w-4 ${selectedRuntime.theme.accent}`} />
                        {capability}
                      </li>
                    ))}
                  </ul>
                </div>

                <div className="mt-7">
                  <h4 className="text-sm font-bold text-slate-900">适用场景</h4>
                  <p className="mt-2 text-sm leading-7 text-slate-600">{selectedRuntime.scenarios}</p>
                </div>

                {selectedRuntime.notice ? (
                  <div className="mt-7 flex gap-3 rounded-xl border border-teal-200 bg-teal-50 p-3.5 text-xs leading-5 text-teal-800">
                    <Sparkles className="mt-0.5 h-4 w-4 shrink-0" />
                    {selectedRuntime.notice}
                  </div>
                ) : null}
              </div>
            </aside>
          </div>
        ) : null}
      </section>
    </main>
  );
}
