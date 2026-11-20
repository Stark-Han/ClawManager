import {useState} from 'react';
import {BookOpen} from 'lucide-react';
import {OwnerTeamDialog} from './OwnerTeamDialog';

const chapters = [
 {title:'团队入门', items:[
  ['Team 是什么','Team 是一组一起完成任务的实例。负责人理解目标、分工并汇总结果，其他成员按各自职责完成任务。'],
  ['从哪里进入','左侧工作区切到 Team，点击团队卡片进入群聊。成员有自己的实例入口。在「Team 设置」可以查看当前任务和成员的可用状态。'],
 ]},
 {title:'创建与模板', items:[
  ['新建 Team','在「Team 设置」点「新建 Team」，选择模板、填写名称，按表单配置成员。创建过程中可以在「操作记录」查看进度。'],
  ['自定义团队模板','在「团队模板」填写目标和预期分工，生成后可调整团队信息、成员职责和协作说明。修改模板会用于后续创建。'],
 ]},
 {title:'对话与看板', items:[
  ['发送任务','在群聊输入目标、要求和预期交付物。选择负责人可请整个团队协作，用 @ 选择成员可以直接交给该成员。'],
  ['跟踪执行','点「执行流程」打开看板。任务按待领取、执行中和已完成分列。点卡片可看任务详情和可用的交付物。'],
  ['回看消息','群聊上方的历史定位可在已有多条任务时快速跳转。已停止的团队可以从「Team 设置」查看历史。'],
 ]},
 {title:'文件与实例工具', items:[
  ['共享文件','群聊标题里的「文件」打开团队共享目录。文件菜单按当前权限和文件类型提供上传、预览、下载、重命名或删除。'],
  ['网页与终端','切到「实例工具」，先选成员，再使用该实例可用的网页或终端。实例状态和运行时能力会影响工具是否可用。'],
 ]},
 {title:'团队管理', items:[
  ['查看操作进度','创建、启停等操作可能需要一些时间。可通过团队状态和「操作记录」查看反馈。'],
  ['管理和删除','「Team 设置」的「管理」可修改团队信息。群聊标题里的「Team 操作」显示当前可用的启停和删除选项。删除团队会删除成员实例和团队共享数据，提交前须确认。'],
 ]},
] as const;

function GuideDialog({onClose}: {onClose: () => void}) {
 const [selected,setSelected] = useState(0);
 const chapter = chapters[selected];
 return <OwnerTeamDialog title="Team 使用指南" onClose={onClose}>
  <div className="ot-guide-layout">
   <nav aria-label="使用指南目录">
    {chapters.map(({title},index)=><button type="button" key={title} aria-pressed={selected===index} aria-controls="ot-guide-chapter" onClick={()=>setSelected(index)}>{title}</button>)}
   </nav>
   <article id="ot-guide-chapter" aria-labelledby="ot-guide-chapter-title">
    <h4 id="ot-guide-chapter-title">{chapter.title}</h4>
    {chapter.items.map(([title,description])=><section key={title}><h5>{title}</h5><p>{description}</p></section>)}
   </article>
  </div>
 </OwnerTeamDialog>;
}

export function OwnerTeamGuide() {
 const [open,setOpen] = useState(false);
 return <>
  <button type="button" className="ot-secondary" aria-haspopup="dialog" onClick={()=>setOpen(true)}><BookOpen size={15} aria-hidden="true"/>使用指南</button>
  {open && <GuideDialog onClose={()=>setOpen(false)}/>}
 </>;
}
