import { useMemo, type ComponentProps } from 'react';
import { ArrowLeft } from 'lucide-react';
import { Link, useLocation, useOutletContext, useParams } from 'react-router-dom';
import type UserLayout from '../../components/UserLayout';
import OwnerInstanceSwitcher from '../../components/OwnerInstanceSwitcher';
import { TeamPageEnvironmentProvider, useTeamPageEnvironment } from '../../components/teams/TeamPageEnvironment';
import { ownerTeamWorkspaceAPI } from '../../services/ownerTeamService';
import { ownerInstanceURL, ownerTeamOverviewURL, type OwnerWorkspaceOutlet } from '../../lib/ownerPortalNavigation';
import TeamDetailPage from './TeamDetailPage';
import '../../components/teams/owner-team-workspace.css';

function NorthboundTeamLayout({ titleAccessory, children }: ComponentProps<typeof UserLayout>) {
  const { id } = useParams();
  const location = useLocation();
  const { owner } = useTeamPageEnvironment();
  const teamId = Number(id);

  return <main className="ot-northbound-layout portal-unified-team" aria-label="团队对话">
    <header className="ot-northbound-header">
      <div><Link className="ot-secondary ot-return" to={ownerTeamOverviewURL(teamId, location.search)}><ArrowLeft size={15} />我的团队空间</Link></div>
      <div className="ot-northbound-header-actions">
        {titleAccessory}
        {owner && <OwnerInstanceSwitcher current={{ owner, teamId }} />}
      </div>
    </header>
    <section className="ot-northbound-body">{children}</section>
  </main>;
}

export default function IEISystemTeamPage() {
  const { id } = useParams(); const location = useLocation();
  const { owner } = useOutletContext<OwnerWorkspaceOutlet>();
  const environment = useMemo(() => ({
    api: ownerTeamWorkspaceAPI,
    owner,
    Layout: NorthboundTeamLayout,
    returnURL: ownerTeamOverviewURL(Number(id), location.search),
    memberEntry: (instanceID: number) => ({
      to: ownerInstanceURL(instanceID, location.search, Number(id)),
      state: { portalReturnSearch: location.search, portalReturnTeamID: Number(id), portalReturnFromChat: true },
    }),
  }), [owner, id, location.search]);
  return <TeamPageEnvironmentProvider value={environment}><TeamDetailPage key={`${owner}:${id}`}/></TeamPageEnvironmentProvider>;
}
