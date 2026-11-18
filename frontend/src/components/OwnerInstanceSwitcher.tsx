import { useCallback, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowRight, ArrowUpRight, Check, PanelsTopLeft, Search, X } from "lucide-react";
import { useLocation, useNavigate } from "react-router-dom";
import { ieiSystemService, type IEISystemInstance } from "../services/ieiSystemService";
import { ownerTeamService, type OwnerTeamSummary } from "../services/ownerTeamService";
import { beforePageRefresh } from "./PageVersionNotice";
import { OWNER_SESSION_INVALID } from "../lib/ownerReadNotice";
import { memberRailState } from "./teams/teamMemberPresentation";
import "../styles/owner-instance-switcher.css";

const logos: Record<string, string> = { openclaw: "/openclaw.png", hermes: "/hermes-card.png", opencode: "/opencode-card.svg", "deepseek-harness": "/deepseek-harness.svg", workbuddy: "/workbuddy.png" };
const names: Record<string, string> = { openclaw: "OpenClaw", hermes: "Hermes", opencode: "OpenCode", "deepseek-harness": "DeepSeek Harness", workbuddy: "WorkBuddy" };
const status = (value: string) => ({ running: "运行中", creating: "启动中", stopped: "已停止", error: "异常", deleting: "删除中" }[value] || "待确认");

export default function OwnerInstanceSwitcher({ current }: { current: IEISystemInstance }) {
  const [open, setOpen] = useState(false);
  const close = useCallback(() => setOpen(false), []);
  return <><button type="button" className="app-button-secondary owner-switch-trigger" aria-expanded={open} onClick={() => setOpen(true)}><PanelsTopLeft size={16} />切换实例</button>
    {open && <OwnerSwitchDrawer key={current.owner} current={current} close={close} />}</>;
}

function OwnerSwitchDrawer({ current, close }: { current: IEISystemInstance; close: () => void }) {
  const [tab, setTab] = useState<"mine" | "teams">(current.team_id ? "teams" : "mine");
  const [search, setSearch] = useState("");
  const [error, setError] = useState("");
  const [pending, setPending] = useState<number | null>(null);
  const dialog = useRef<HTMLDialogElement>(null);
  const live = useRef(true);
  const request = useRef(0);
  const navigate = useNavigate(), location = useLocation(), cache = useQueryClient();
  useEffect(() => {
    live.current = true;
    const previous = document.activeElement as HTMLElement | null;
    const node = dialog.current; node?.showModal();
    const invalid = () => { cache.removeQueries({ queryKey: ["owner-switch", current.owner] }); close(); };
    window.addEventListener(OWNER_SESSION_INVALID, invalid);
    return () => { live.current = false; request.current++; node?.close(); previous?.focus(); window.removeEventListener(OWNER_SESSION_INVALID, invalid); };
  }, [cache, current.owner, close]);
  const list = useQuery({
    queryKey: ["owner-switch", current.owner], staleTime: 10000, gcTime: 0, retry: false,
    queryFn: async ({ signal }) => {
      const instances: IEISystemInstance[] = [], teams: OwnerTeamSummary[] = [];
      // Fetch every page: a Team member search must not silently cover only
      // the first portal page. Requests remain bounded by server pagination.
      await Promise.all([
        (async () => { for (let page = 1; ; page++) { const data = await ieiSystemService.listInstances(page, 100, signal); instances.push(...data.instances); if (!data.instances.length || page * data.limit >= data.total) break; } })(),
        (async () => { for (let page = 1; ; page++) { const data = await ownerTeamService.list(page, 100, "", signal); teams.push(...data.teams); if (!data.teams.length || page * data.limit >= data.total) break; } })(),
      ]);
      return { instances: instances.filter(x => x.owner.toLowerCase() === current.owner.toLowerCase()), teams };
    },
  });
  const canLeave = () => {
    if (window.dispatchEvent(new Event(beforePageRefresh, { cancelable: true }))) return true;
    setError("请先保存或关闭当前编辑，再切换工作区。"); return false;
  };
  const select = async (id: number) => {
    if (pending || !canLeave()) return;
    const seq = ++request.current; setPending(id); setError("");
    try {
      const target = await ieiSystemService.getInstance(id);
      if (!live.current || seq !== request.current) return;
      if (target.owner.toLowerCase() !== current.owner.toLowerCase() || target.status === "deleting") throw new Error("该实例暂不可访问，请刷新列表。");
      navigate(`/ieisystem/instances/${id}`, { state: { ...location.state, startStopped: false } }); close();
    } catch (e) { if (live.current && seq === request.current) setError(e instanceof Error ? e.message : "切换失败，请重试。"); }
    finally { if (live.current && seq === request.current) setPending(null); }
  };
  const q = search.trim().toLowerCase();
  const matches = (...values: unknown[]) => values.join(" ").toLowerCase().includes(q);
  const teams = list.data?.teams || [];
  const memberIDs = new Set(teams.flatMap(team => (team.members || []).map(m => m.instance_id)));
  const mine = (list.data?.instances || []).filter(i => !i.team_id && !memberIDs.has(i.id));
  const card = (id: number | undefined, name: string, runtime: string, state: string, role?: string) => <button type="button" key={id || name} className={`owner-switch-card ${id === current.id ? "is-current" : ""}`} disabled={!id || pending !== null} onClick={() => id && void select(id)} aria-current={id === current.id ? "true" : undefined}>
    <span className="owner-switch-avatar">{logos[runtime] ? <img src={logos[runtime]} alt="" /> : <PanelsTopLeft size={24} />}</span>
    <span className="owner-switch-identity"><strong title={name}>{name}</strong><small>{names[runtime] || runtime}{id ? ` · #${id}` : ""}{role ? ` · ${role}` : ""}</small><span className="owner-switch-status">{pending === id ? "正在切换…" : state}</span></span>
    {id === current.id ? <span className="owner-switch-current"><Check size={14} />当前</span> : <ArrowRight size={15} />}
  </button>;
  return createPortal(<dialog ref={dialog} className="owner-switch-dialog" aria-labelledby="owner-switch-title" onCancel={close} onClick={e => { if (e.target === e.currentTarget) close(); }}>
    <header><div><h2 id="owner-switch-title">切换实例</h2><p>选择要进入的工作区</p></div><button type="button" className="cm-icon-button" onClick={close} aria-label="关闭切换列表"><X size={18} /></button></header>
    <label className="owner-switch-search"><Search size={17} /><input type="search" value={search} onChange={e => setSearch(e.target.value)} placeholder="搜索实例、团队或成员" aria-label="搜索实例、团队或成员" autoFocus /></label>
    <div className="owner-switch-tabs" role="group" aria-label="切换范围"><button type="button" aria-pressed={tab === "mine"} onClick={() => setTab("mine")}>我的实例 <span>{mine.length}</span></button><button type="button" aria-pressed={tab === "teams"} onClick={() => setTab("teams")}>Team <span>{teams.length}</span></button></div>
    <div className="owner-switch-list">
      {list.isLoading && <p role="status">正在加载工作区…</p>}
      {list.error && <div role="alert">工作区列表加载失败。<button type="button" className="app-button-secondary" onClick={() => void list.refetch()}>重试</button></div>}
      {error && <p role="alert">{error}</p>}
      {tab === "mine" && mine.filter(i => matches(i.alias, i.name, i.type, i.id)).map(i => card(i.id, i.alias?.trim() || i.name, i.type, status(i.status)))}
      {tab === "teams" && teams.filter(team => matches(team.name, team.id) || team.members?.some(m => matches(m.display_name, m.instance_id, m.runtime_type, m.role))).map(team => <section className="owner-switch-group" key={team.id}><header><div><strong>{team.name}</strong><small>Team #{team.id} · {team.member_count} 名成员</small></div><button type="button" className="app-button-secondary" onClick={() => { if (canLeave()) { navigate(`/ieisystem/teams/${team.id}`, { state: { ...location.state } }); close(); } }}>团队聊天 <ArrowUpRight size={14} /></button></header>
        {(team.members || []).filter(m => matches(team.name, team.id) || matches(m.display_name, m.instance_id, m.runtime_type, m.role)).map(m => card(m.instance_id, m.display_name, m.runtime_type || "openclaw", memberRailState(m).label, m.member_key === "leader" || m.role === "leader" ? "Leader" : "Worker"))}</section>)}
      {!list.isLoading && !list.error && (tab === "mine" ? !mine.some(i => matches(i.alias, i.name, i.type, i.id)) : !teams.some(t => matches(t.name, t.id) || t.members?.some(m => matches(m.display_name, m.instance_id, m.runtime_type, m.role)))) && <p className="owner-switch-empty">没有匹配的工作区</p>}
    </div>
  </dialog>, document.body);
}
