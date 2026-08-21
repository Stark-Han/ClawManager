import axios from "axios";
import { ArrowLeft, Maximize2, Minimize2, RefreshCw } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { WorkspaceFileManager } from "../../components/WorkspaceFileManager";
import { prepareOpenClawControlUIStorage } from "../../lib/openclawControlStorage";
import {
  ieiSystemService,
  ieiSystemWorkspaceService,
  type IEISystemInstance,
  type IEISystemInstanceAccess,
} from "../../services/ieiSystemService";

function resolveEmbedUrl(url: string) {
  if (/^https?:\/\//i.test(url)) return url;
  const explicitOrigin = import.meta.env.VITE_BACKEND_ORIGIN as string | undefined;
  if (explicitOrigin) return new URL(url, explicitOrigin).toString();
  if (window.location.port === "9002" && url.startsWith("/api/")) {
    return `${window.location.protocol}//${window.location.hostname}:9001${url}`;
  }
  return url;
}

function errorMessage(error: unknown) {
  if (axios.isAxiosError(error) && typeof error.response?.data?.error === "string") {
    return error.response.data.error;
  }
  return "无法进入该实例，请稍后重试。";
}

export default function IEISystemInstancePage() {
  const { id = "" } = useParams<{ id: string }>();
  const instanceID = Number(id);
  const frameContainerRef = useRef<HTMLElement | null>(null);
  const [instance, setInstance] = useState<IEISystemInstance | null>(null);
  const [access, setAccess] = useState<IEISystemInstanceAccess | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [frameVersion, setFrameVersion] = useState(0);
  const [isFullscreen, setIsFullscreen] = useState(false);

  const openInstance = useCallback(async () => {
    if (!Number.isInteger(instanceID) || instanceID <= 0) {
      setError("实例编号无效。");
      setLoading(false);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      // Both calls validate the dedicated IEI session and owner relationship.
      const nextInstance = await ieiSystemService.getInstance(instanceID);
      setInstance(nextInstance);
      const nextAccess = await ieiSystemService.generateAccess(instanceID);
      setAccess(nextAccess);
      setFrameVersion((version) => version + 1);
    } catch (openError) {
      setAccess(null);
      setError(errorMessage(openError));
    } finally {
      setLoading(false);
    }
  }, [instanceID]);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void openInstance();
    }, 0);
    return () => window.clearTimeout(timer);
  }, [openInstance]);

  useEffect(() => {
    const existing = document.querySelector<HTMLMetaElement>('meta[name="referrer"]');
    const previous = existing?.content;
    const meta = existing ?? document.createElement("meta");
    if (!existing) {
      meta.name = "referrer";
      document.head.appendChild(meta);
    }
    meta.content = "no-referrer";
    return () => {
      if (existing) existing.content = previous ?? "";
      else meta.remove();
    };
  }, []);

  useEffect(() => {
    const handleFullscreen = () => setIsFullscreen(document.fullscreenElement === frameContainerRef.current);
    document.addEventListener("fullscreenchange", handleFullscreen);
    return () => document.removeEventListener("fullscreenchange", handleFullscreen);
  }, []);

  const frameSrc = useMemo(() => {
    if (!instance || !access?.access_url) return "";
    const url = resolveEmbedUrl(access.access_url);
    return instance.type.toLowerCase() === "openclaw"
      ? prepareOpenClawControlUIStorage(instance.id, url)
      : url;
  }, [access, instance]);

  const handleFullscreen = () => {
    const element = frameContainerRef.current;
    if (!element) return;
    if (document.fullscreenElement === element) void document.exitFullscreen();
    else void element.requestFullscreen().catch(() => undefined);
  };

  if (loading && !access) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-slate-100 text-sm text-slate-600">
        <RefreshCw className="mr-2 h-5 w-5 animate-spin" />
        正在验证并进入实例…
      </main>
    );
  }

  if (!instance || !access || error) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-slate-100 p-6">
        <section className="w-full max-w-md rounded-xl border border-red-200 bg-white p-6 text-center shadow-sm">
          <h1 className="text-lg font-semibold text-slate-950">实例不可访问</h1>
          <p className="mt-2 text-sm leading-6 text-slate-600">{error ?? "访问验证未通过。"}</p>
          <div className="mt-5 flex justify-center gap-2">
            <Link className="app-button-secondary" to="/ieisystem/list-instances">
              <ArrowLeft className="h-4 w-4" /> 返回列表
            </Link>
            <button type="button" className="app-button-primary" onClick={() => void openInstance()}>
              <RefreshCw className="h-4 w-4" /> 重试
            </button>
          </div>
        </section>
      </main>
    );
  }

  const canShowWorkspace = access.workspace_available;

  return (
    <main className="flex h-screen min-h-[560px] flex-col overflow-hidden bg-slate-100">
      <header className="flex h-14 shrink-0 items-center justify-between gap-4 border-b border-slate-200 bg-white px-4">
        <div className="flex min-w-0 items-center gap-3">
          <Link className="cm-icon-button shrink-0" title="返回实例列表" to="/ieisystem/list-instances">
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div className="min-w-0">
            <h1 className="truncate text-base font-semibold text-slate-950">{instance.name}</h1>
            <p className="text-xs text-slate-500">IEI 安全访问 · {instance.owner}</p>
          </div>
        </div>
        <span className="inline-flex shrink-0 items-center gap-2 rounded-full border border-emerald-200 bg-emerald-50 px-3 py-1 text-xs font-medium text-emerald-700">
          <span className="h-2 w-2 rounded-full bg-emerald-400" />
          已验证
        </span>
      </header>

      <section className="grid min-h-0 flex-1 gap-4 p-4 max-xl:grid-rows-[minmax(420px,1fr)_minmax(360px,0.8fr)] xl:grid-cols-[minmax(0,1fr)_minmax(360px,28rem)]">
        <section
          ref={frameContainerRef}
          className="cm-surface flex min-h-0 min-w-0 flex-col overflow-hidden bg-white"
          style={isFullscreen ? { height: "100vh", width: "100vw", borderRadius: 0 } : undefined}
        >
          <div className="flex h-12 shrink-0 items-center justify-between border-b border-slate-200 px-3">
            <span className="min-w-0 truncate text-sm font-medium text-slate-950">{instance.name}</span>
            <div className="flex shrink-0 items-center gap-2">
              <button type="button" className="cm-icon-button" title="刷新访问" onClick={() => void openInstance()}>
                <RefreshCw className={`h-4 w-4 ${loading ? "animate-spin" : ""}`} />
              </button>
              <button type="button" className="cm-icon-button" title={isFullscreen ? "退出全屏" : "全屏"} onClick={handleFullscreen}>
                {isFullscreen ? <Minimize2 className="h-4 w-4" /> : <Maximize2 className="h-4 w-4" />}
              </button>
            </div>
          </div>
          <iframe
            key={`${frameSrc}:${frameVersion}`}
            title={`${instance.name} service`}
            src={frameSrc}
            className="min-h-0 w-full flex-1 border-0 bg-white"
            scrolling="no"
            allow="clipboard-read; clipboard-write; fullscreen; autoplay"
            referrerPolicy="no-referrer"
          />
        </section>

        {canShowWorkspace ? (
          <div className="min-h-0 min-w-0">
            <WorkspaceFileManager
              instanceId={instance.id}
              initialPath={access.workspace_root === "/config" ? "/config" : undefined}
              service={ieiSystemWorkspaceService}
              workspaceKey={`iei:${instance.owner}:${instance.id}`}
              canWrite
              localeOverride="zh"
            />
          </div>
        ) : (
          <section className="cm-surface flex min-h-[360px] items-center justify-center p-6 text-center text-sm text-slate-500">
            该实例没有可用的工作区。
          </section>
        )}
      </section>
    </main>
  );
}
