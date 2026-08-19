import axios from "axios";
import { ArrowRight, Box, LogOut, RefreshCw, ShieldCheck } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
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

function InstanceTypeIcon({ type }: { type: string }) {
  const normalizedType = type.trim().toLowerCase();
  if (normalizedType === "openclaw") {
    return <img src="/openclaw.png" alt="OpenClaw" className="h-full w-full object-contain" />;
  }
  if (normalizedType === "hermes") {
    return <img src="/hermes.png" alt="Hermes" className="h-full w-full object-contain" />;
  }
  return <Box aria-label={type || "Instance"} className="h-6 w-6 text-slate-500" />;
}

function errorMessage(error: unknown) {
  if (axios.isAxiosError(error) && typeof error.response?.data?.error === "string") {
    return error.response.data.error;
  }
  return "无法加载 Lite 实例，请稍后重试。";
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
    if (existing) {
      existing.content = previous ?? "";
    } else {
      meta.remove();
    }
  };
}

export default function IEISystemListInstancesPage() {
  const initialized = useRef(false);
  const [session, setSession] = useState<IEISystemSession | null>(null);
  const [instances, setInstances] = useState<IEISystemInstance[]>([]);
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
  }, []);

  const initialize = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const params = new URLSearchParams(window.location.search);
      const token = params.get("token")?.trim();
      if (token) {
        // Remove the one-time AES token before any subsequent navigation or
        // subresource can receive it as a referrer.
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

  const handleLogout = async () => {
    await ieiSystemService.clearSession().catch(() => undefined);
    setSession(null);
    setInstances([]);
    setError("访问会话已退出，请从智慧协作平台重新进入。 ");
  };

  return (
    <main className="min-h-screen bg-slate-100">
      <header className="border-b border-slate-200 bg-white">
        <div className="mx-auto flex max-w-7xl flex-wrap items-center justify-between gap-4 px-5 py-5 sm:px-8">
          <div>
            <div className="flex items-center gap-2 text-sm font-semibold text-sky-700">
              <ShieldCheck className="h-4 w-4" />
              智慧协作平台安全访问
            </div>
            <h1 className="mt-1 text-2xl font-semibold text-slate-950">Lite 实例</h1>
            {session ? <p className="mt-1 text-sm text-slate-500">当前 owner：{session.owner}</p> : null}
          </div>
          {session ? (
            <div className="flex items-center gap-2">
              <button type="button" className="app-button-secondary" onClick={() => void initialize()} disabled={loading}>
                <RefreshCw className={`h-4 w-4 ${loading ? "animate-spin" : ""}`} />
                刷新
              </button>
              <button type="button" className="app-button-secondary" onClick={() => void handleLogout()}>
                <LogOut className="h-4 w-4" />
                退出
              </button>
            </div>
          ) : null}
        </div>
      </header>

      <section className="mx-auto max-w-7xl px-5 py-8 sm:px-8">
        {error ? (
          <div className="mx-auto max-w-lg rounded-xl border border-red-200 bg-white p-6 text-center shadow-sm">
            <h2 className="text-lg font-semibold text-slate-950">访问验证失败</h2>
            <p className="mt-2 text-sm leading-6 text-slate-600">{error}</p>
          </div>
        ) : loading ? (
          <div className="flex items-center justify-center py-24 text-sm text-slate-600">
            <RefreshCw className="mr-2 h-5 w-5 animate-spin" />
            正在验证身份并加载实例…
          </div>
        ) : instances.length === 0 ? (
          <div className="app-panel flex flex-col items-center px-6 py-16 text-center">
            <Box className="h-10 w-10 text-slate-300" />
            <h2 className="mt-4 text-lg font-semibold text-slate-900">暂无 Lite 实例</h2>
            <p className="mt-1 text-sm text-slate-500">当前 owner 下没有可展示的 Lite 实例。</p>
          </div>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
            {instances.map((instance) => (
              <Link
                key={instance.id}
                to={`/ieisystem/instances/${instance.id}`}
                className="group app-panel block p-5 transition hover:-translate-y-0.5 hover:border-sky-300 hover:shadow-md"
              >
                <div className="flex items-start justify-between gap-4">
                  <div className="flex min-w-0 items-center gap-3">
                    <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-slate-200 bg-slate-50 p-1.5">
                      <InstanceTypeIcon type={instance.type} />
                    </div>
                    <div className="min-w-0">
                      <h2 className="truncate text-lg font-semibold text-slate-950 group-hover:text-sky-700">{instance.name}</h2>
                      <p className="mt-1 text-sm text-slate-500">
                        {instance.type.toLowerCase() === "hermes" ? "Hermes" : "OpenClaw"} Lite · #{instance.id}
                      </p>
                    </div>
                  </div>
                  <span className={`rounded-full border px-2.5 py-1 text-xs font-semibold ${statusClass(instance.status)}`}>
                    {instance.status}
                  </span>
                </div>
                <div className="mt-5 flex items-center justify-between border-t border-slate-100 pt-4 text-xs text-slate-500">
                  <span>{new Date(instance.created_at).toLocaleString()}</span>
                  <span className="inline-flex items-center gap-1 font-semibold text-sky-700">
                    进入实例
                    <ArrowRight className="h-4 w-4 transition-transform group-hover:translate-x-0.5" />
                  </span>
                </div>
              </Link>
            ))}
          </div>
        )}
      </section>
    </main>
  );
}
