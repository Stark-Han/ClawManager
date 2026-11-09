// Development-only acceptance entry. Not imported by the application bundle.
import React from 'react';
import { createRoot } from 'react-dom/client';
import { OwnerTeamTemplates } from '../../src/components/teams/OwnerTeamTemplates';
import '../../src/index.css';
import '../../src/components/teams/owner-team-workspace.css';

createRoot(document.getElementById('root')!).render(<main className="ot-workspace" style={{ height: 'calc(100dvh - 24px)', margin: 12 }}><OwnerTeamTemplates owner="fixture@example.com" canGenerate onOperation={() => {}}/></main>);
