import { useEffect, useId, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { FolderOpen, PanelLeftClose, Users, X } from "lucide-react";
import type { TeamMember } from "../../types/team";
import "./team-chat-workspace.css";

export type TeamWorkspacePanel = "closed" | "kanban" | "files";

// Width is the workspace's usable width, not the browser viewport.
export function workspaceMode(width: number) {
  return width >= 1472 ? "dual" : width >= 1248 ? "single" : "drawer";
}

export default function TeamChatWorkspace({ members, membersContent, chat, renderKanban, files }: {
  members: TeamMember[];
  membersContent: ReactNode;
  chat: ReactNode;
  renderKanban: (mini: boolean, open: () => void) => ReactNode;
  files: ReactNode;
}) {
  const root = useRef<HTMLDivElement>(null);
  const leftPanel = useRef<HTMLElement>(null);
  const rightPanel = useRef<HTMLElement>(null);
  const lastTrigger = useRef<HTMLElement | null>(null);
  const membersPanelId = useId();
  const [width, setWidth] = useState(0);
  const [left, setLeft] = useState(false);
  const [right, setRight] = useState<TeamWorkspacePanel>("closed");
  const [filesVisited, setFilesVisited] = useState(false);
  const [lastSide, setLastSide] = useState<"left" | "right">("right");
  const mode = workspaceMode(width);
  useLayoutEffect(() => {
    const node = root.current;
    if (!node) return;
    const observer = new ResizeObserver(([entry]) => setWidth(entry.contentRect.width));
    observer.observe(node);
    return () => observer.disconnect();
  }, []);
  useEffect(() => {
    if (mode !== "dual" && left && right !== "closed") {
      if (lastSide === "left") setRight("closed"); else setLeft(false);
    }
  }, [mode, left, right, lastSide]);
  const rememberTrigger = () => {
    if (document.activeElement instanceof HTMLElement) lastTrigger.current = document.activeElement;
  };
  const close = () => {
    setLeft(false);
    setRight("closed");
    restoreFocus();
  };
  const restoreFocus = () => requestAnimationFrame(() => {
    if (lastTrigger.current?.isConnected && lastTrigger.current.getClientRects().length) lastTrigger.current.focus();
    else root.current?.querySelector<HTMLButtonElement>('[aria-label="展开任务看板"]')?.focus();
  });
  const openRight = (panel: "kanban" | "files") => {
    rememberTrigger();
    setLastSide("right");
    if (mode !== "dual") setLeft(false);
    setRight(current => current === panel ? "closed" : panel);
    if (panel === "files") setFilesVisited(true);
  };
  const drawerOpen = mode === "drawer" && (left || right !== "closed");
  useEffect(() => {
    if (!drawerOpen) return;
    const panel = left ? leftPanel.current : rightPanel.current;
    panel?.querySelector<HTMLButtonElement>("button")?.focus();
    const onKey = (event: KeyboardEvent) => {
      // File previews have their own modal layer and keyboard lifecycle.
      if (document.querySelector('[data-workspace-preview]')) return;
      if (event.key === "Escape") { event.preventDefault(); close(); }
      if (event.key !== "Tab" || !panel) return;
      const controls = [...panel.querySelectorAll<HTMLElement>('button:not([disabled]), a[href], input, textarea, select, summary')]
        .filter(node => node.getClientRects().length > 0);
      const first = controls[0], last = controls[controls.length - 1];
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [drawerOpen, left, right]);

  return <div ref={root} className="team-chat-workspace" data-mode={mode} data-left={left} data-right={right}>
    <div className="team-member-rail" inert={drawerOpen}>
      <button type="button" aria-label="展开成员与团队配置" aria-expanded={left} aria-controls={membersPanelId} onClick={() => {
        rememberTrigger(); setLastSide("left"); setLeft(value => !value); if (mode !== "dual") setRight("closed");
      }}><Users size={19} /><span>成员</span><small>{members.length} 人</small></button>
      {members.slice(0, 5).map(member => <button type="button" key={member.id} title={`${member.display_name} · ${member.role} · 调度 ${member.status} · Runtime ${member.availability || "unknown"}`}
        onClick={() => { rememberTrigger(); setLastSide("left"); setLeft(true); if (mode !== "dual") setRight("closed"); }}>
        <span className="team-member-initial">{member.display_name.slice(0, 2).toUpperCase()}</span>
        <span className="team-member-name">{member.display_name}</span>
        <small className={member.availability === "blocked" || member.availability === "offline" ? "text-red-600" : "text-slate-500"}>
          {member.availability === "blocked" || member.availability === "offline" ? "不可用" : member.availability === "busy" ? "忙碌" : member.availability === "idle" ? "空闲" : "未知"}
        </small>
      </button>)}
      {members.length > 5 && <button type="button" onClick={() => { rememberTrigger(); setLeft(true); setLastSide("left"); if (mode !== "dual") setRight("closed"); }}>+{members.length - 5}</button>}
    </div>
    <aside id={membersPanelId} ref={leftPanel} hidden={!left} className="team-members-panel" role={mode === "drawer" ? "dialog" : undefined} aria-modal={mode === "drawer" ? true : undefined} aria-label="成员与团队配置">
      <header><strong>成员与团队配置</strong><button type="button" aria-label="收起成员" onClick={() => { setLeft(false); restoreFocus(); }}><PanelLeftClose size={18} /></button></header>
      <div className="team-members-body">{membersContent}</div>
    </aside>
    <main className="team-chat-main" inert={drawerOpen}>{chat}</main>
    <aside ref={rightPanel} className="team-tools-panel" role={drawerOpen && right !== "closed" ? "dialog" : undefined} aria-modal={drawerOpen && right !== "closed" ? true : undefined} aria-label="任务看板与文件" inert={drawerOpen && left}>
      <header className="team-tools-toolbar">
        {right !== "closed" && <strong>{right === "kanban" ? "任务看板" : "共享文件"}</strong>}
        {right !== "closed" && <button type="button" onClick={() => openRight("kanban")} aria-pressed={right === "kanban"}>看板</button>}
        <button type="button" onClick={() => openRight("files")} aria-expanded={right === "files"}><FolderOpen size={17} /><span>文件</span></button>
        {right !== "closed" && <button type="button" aria-label="收起右侧面板" onClick={() => { setRight("closed"); restoreFocus(); }}><X size={18} /></button>}
      </header>
      <div className="team-kanban-host" hidden={right === "files"}>{renderKanban(right === "closed", () => openRight("kanban"))}</div>
      <div className="team-files-host" hidden={right !== "files"}>{filesVisited && files}</div>
    </aside>
    {drawerOpen && <button type="button" className="team-drawer-backdrop" aria-label="关闭工具抽屉" onClick={close} />}
  </div>;
}
