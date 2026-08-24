export interface IEIRuntimePresentation {
  id: string;
  name: string;
  category: string;
  tagline: string;
  positioning: string;
  capabilities: string[];
  scenarios: string;
  badge?: string;
  notice?: string;
  theme: {
    accent: string;
    accentSoft: string;
    border: string;
    dot: string;
    selection: string;
  };
}

const runtimeCatalog: Record<string, IEIRuntimePresentation> = {
  openclaw: {
    id: "openclaw",
    name: "OpenClaw",
    category: "通用智能体工作台",
    tagline: "会话、工具与自动化任务空间",
    positioning:
      "面向通用智能体任务的受管工作空间，提供原生会话、工具调用、定时任务、Channel 与 Skill 扩展能力。",
    capabilities: ["原生智能体会话", "工具与定时任务", "Channel 接入", "Skill 扩展", "持续工作区"],
    scenarios: "市场分析、知识处理、自动化任务、资料整理与日常业务助理。",
    theme: {
      accent: "text-rose-600",
      accentSoft: "bg-rose-50",
      border: "border-rose-200",
      dot: "bg-rose-500",
      selection: "border-rose-300 bg-rose-50/70 shadow-[inset_4px_0_0_#f43f5e]",
    },
  },
  hermes: {
    id: "hermes",
    name: "Hermes",
    category: "知识与任务智能体",
    tagline: "持久会话与原生工具工作空间",
    positioning:
      "保留 Hermes 原生会话和工具体验，以持久化工作区承载上下文、配置与执行结果，适合连续推进复杂知识任务。",
    capabilities: ["持久化原生会话", "Hermes 工具集", "模型统一接入", "Skill 扩展", "长任务执行"],
    scenarios: "知识问答、资料整理、深度研究、持续对话与复杂任务执行。",
    theme: {
      accent: "text-violet-600",
      accentSoft: "bg-violet-50",
      border: "border-violet-200",
      dot: "bg-violet-500",
      selection: "border-violet-300 bg-violet-50/70 shadow-[inset_4px_0_0_#8b5cf6]",
    },
  },
  opencode: {
    id: "opencode",
    name: "OpenCode",
    category: "开发者代码工作台",
    tagline: "面向代码生成、终端与仓库协作",
    positioning:
      "面向软件研发的智能编码工作空间，将代码生成、文件操作、终端命令和仓库协作整合在统一工作台中。",
    capabilities: ["智能代码生成与补全", "终端命令执行", "仓库浏览与协作", "代码审查与建议", "自动化脚本"],
    scenarios: "软件开发、代码维护、运维管理、自动化构建及团队研发协作。",
    theme: {
      accent: "text-blue-600",
      accentSoft: "bg-blue-50",
      border: "border-blue-200",
      dot: "bg-blue-600",
      selection: "border-blue-300 bg-blue-50/80 shadow-[inset_4px_0_0_#2563eb]",
    },
  },
  "deepseek-harness": {
    id: "deepseek-harness",
    name: "DeepSeek Harness",
    category: "插件化智能体工作台",
    tagline: "一切皆插件",
    positioning:
      "DeepSeek 官方开源的 Agent Harness。采用“一切皆插件”的 Cordis 架构，将模型、工具、工作流与子代理组合成可扩展的智能体工作空间。",
    capabilities: ["插件化能力组合", "工作区读写", "命令与任务规划", "子代理协作", "多模型接入"],
    scenarios: "智能研发、复杂任务分解、多代理协作、插件实验与可扩展 Agent 工作流。",
    badge: "NEW",
    notice: "Developer Preview · 官方仍在快速迭代，后续版本可能存在兼容性变化。",
    theme: {
      accent: "text-teal-600",
      accentSoft: "bg-teal-50",
      border: "border-teal-200",
      dot: "bg-teal-500",
      selection: "border-teal-300 bg-teal-50/70 shadow-[inset_4px_0_0_#14b8a6]",
    },
  },
  workbuddy: {
    id: "workbuddy",
    name: "WorkBuddy",
    category: "智能办公工作台",
    tagline: "你的智能办公搭档",
    positioning:
      "面向日常办公和项目协作的独立 Linux 桌面，将 WorkBuddy、文档文件、桌面工具和平台模型接入集中在一个持续可用的智能工作空间中。",
    capabilities: ["智能办公协作", "Linux 图形桌面", "文档与项目文件", "平台模型接入", "持久化工作区"],
    scenarios: "资料整理、内容处理、项目协作、日常办公与需要桌面工具的持续任务。",
    notice: "Linux 兼容运行环境",
    theme: {
      accent: "text-amber-700",
      accentSoft: "bg-amber-50",
      border: "border-amber-200",
      dot: "bg-amber-500",
      selection: "border-amber-300 bg-amber-50/70 shadow-[inset_4px_0_0_#f59e0b]",
    },
  },
};

const fallbackRuntime: IEIRuntimePresentation = {
  id: "unknown",
  name: "自定义 Runtime",
  category: "受管工作空间",
  tagline: "由 ClawManager 管理的实例环境",
  positioning: "该实例使用扩展 Runtime。具体工具、交互方式和运行能力由所配置的 Runtime 镜像提供。",
  capabilities: ["独立工作区", "生命周期管理", "安全访问", "文件管理"],
  scenarios: "适用于企业自定义镜像和后续接入的新型 Runtime。",
  theme: {
    accent: "text-slate-600",
    accentSoft: "bg-slate-100",
    border: "border-slate-200",
    dot: "bg-slate-500",
    selection: "border-slate-300 bg-slate-50 shadow-[inset_4px_0_0_#64748b]",
  },
};

export function getIEIRuntimePresentation(type: string): IEIRuntimePresentation {
  const normalized = type.trim().toLowerCase();
  const configured = runtimeCatalog[normalized];
  if (configured) return configured;
  return {
    ...fallbackRuntime,
    id: normalized || fallbackRuntime.id,
    name: type.trim() || fallbackRuntime.name,
  };
}
