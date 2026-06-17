import React, { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { Monitor, MonitorPlay, Play, Search, Square, Trash2 } from "lucide-react";
import ConfirmDialog from "../../components/ConfirmDialog";
import UserLayout from "../../components/UserLayout";
import { useI18n } from "../../contexts/I18nContext";
import { instanceService } from "../../services/instanceService";
import { teamService } from "../../services/teamService";
import type { Instance, InstanceAvailability } from "../../types/instance";
import type { Team, TeamMember } from "../../types/team";

type AvailabilityFilter = "all" | InstanceAvailability;
type TeamMembership = {
  team: Team;
  member: TeamMember;
};
type LoadInstancesOptions = {
  silent?: boolean;
  refreshTeams?: boolean;
};

const INSTANCE_LIST_PAGE_SIZE = 100;
const TEAM_LIST_PAGE_SIZE = 100;

const instanceTimeValue = (instance: Instance) => {
  const value = Date.parse(instance.updated_at || instance.created_at || "");
  return Number.isFinite(value) ? value : 0;
};

const sortInstances = (items: Instance[]) =>
  [...items].sort(
    (left, right) => instanceTimeValue(right) - instanceTimeValue(left) || right.id - left.id,
  );

const loadAllInstances = async () => {
  const firstPage = await instanceService.getInstances(1, INSTANCE_LIST_PAGE_SIZE);
  const instances = [...(firstPage.instances || [])];
  const total = firstPage.total || instances.length;
  const totalPages = Math.ceil(total / INSTANCE_LIST_PAGE_SIZE);

  for (let page = 2; page <= totalPages; page += 1) {
    const nextPage = await instanceService.getInstances(page, INSTANCE_LIST_PAGE_SIZE);
    instances.push(...(nextPage.instances || []));
  }

  return sortInstances(instances);
};

const loadAllTeams = async () => {
  const firstPage = await teamService.listTeams(1, TEAM_LIST_PAGE_SIZE);
  const teams = [...(firstPage.teams || [])];
  const total = firstPage.total || teams.length;
  const totalPages = Math.ceil(total / TEAM_LIST_PAGE_SIZE);

  for (let page = 2; page <= totalPages; page += 1) {
    const nextPage = await teamService.listTeams(page, TEAM_LIST_PAGE_SIZE);
    teams.push(...(nextPage.teams || []));
  }

  return teams;
};

const loadTeamMemberships = async () => {
  const teams = await loadAllTeams();
  const details = await Promise.all(
    teams.map((team) => teamService.getTeam(team.id).catch(() => null)),
  );
  const memberships = new Map<number, TeamMembership[]>();

  details.forEach((detail) => {
    if (!detail) {
      return;
    }
    detail.members.forEach((member) => {
      if (!member.instance_id) {
        return;
      }
      const current = memberships.get(member.instance_id) || [];
      memberships.set(member.instance_id, [
        ...current,
        {
          team: detail.team,
          member,
        },
      ]);
    });
  });

  return memberships;
};

function availabilityForStatus(status: string): InstanceAvailability {
  if (status === "running") {
    return "available";
  }
  if (status === "creating") {
    return "starting";
  }
  return "unavailable";
}

function availabilityLabel(availability: InstanceAvailability) {
  switch (availability) {
    case "available":
      return "Available";
    case "starting":
      return "Starting";
    default:
      return "Unavailable";
  }
}

function availabilityClass(availability: InstanceAvailability) {
  switch (availability) {
    case "available":
      return "border-emerald-200 bg-emerald-50 text-emerald-700";
    case "starting":
      return "border-amber-200 bg-amber-50 text-amber-700";
    default:
      return "border-slate-200 bg-slate-50 text-slate-600";
  }
}

function typeLabel(type: string) {
  return type === "hermes" ? "Hermes" : type === "openclaw" ? "OpenClaw" : type;
}

function modeLabel(mode: Instance["instance_mode"]) {
  return mode === "pro" ? "Pro" : "Lite";
}

function modeClass(mode: Instance["instance_mode"]) {
  return mode === "pro"
    ? "border-indigo-200 bg-indigo-50 text-indigo-700"
    : "border-sky-200 bg-sky-50 text-sky-700";
}

function formatBytes(value?: number) {
  if (!value || value <= 0) {
    return "0 B";
  }
  const units = ["B", "KB", "MB", "GB", "TB"];
  let size = value;
  let unitIndex = 0;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex += 1;
  }
  return `${size.toFixed(size >= 10 || unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`;
}

function getErrorMessage(err: unknown, fallback: string) {
  const responseError = (err as { response?: { data?: { error?: string } } })?.response?.data?.error;
  if (responseError) {
    return responseError;
  }
  return err instanceof Error ? err.message : fallback;
}

const InstanceListPage: React.FC = () => {
  const { t, locale } = useI18n();
  const navigate = useNavigate();
  const [instances, setInstances] = useState<Instance[]>([]);
  const [teamMemberships, setTeamMemberships] = useState<Map<number, TeamMembership[]>>(
    new Map(),
  );
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [deletingIds, setDeletingIds] = useState<number[]>([]);
  const [actionLoading, setActionLoading] = useState<number | null>(null);
  const [pendingDeleteId, setPendingDeleteId] = useState<number | null>(null);
  const [availabilityFilter, setAvailabilityFilter] = useState<AvailabilityFilter>("all");
  const [searchQuery, setSearchQuery] = useState("");

  const refreshTeamMemberships = useCallback(async () => {
    try {
      const memberships = await loadTeamMemberships();
      setTeamMemberships(memberships);
    } catch (teamError) {
      console.error("Failed to load team memberships", teamError);
    }
  }, []);

  const loadInstances = useCallback(
    async (options?: LoadInstancesOptions) => {
      try {
        if (!options?.silent) {
          setLoading(true);
        }
        setError(null);
        const data = await loadAllInstances();
        setInstances(data);
        if (options?.refreshTeams !== false) {
          void refreshTeamMemberships();
        }
      } catch (err: unknown) {
        setError(getErrorMessage(err, t("instances.failedToLoad")));
      } finally {
        if (!options?.silent) {
          setLoading(false);
        }
      }
    },
    [refreshTeamMemberships, t],
  );

  useEffect(() => {
    void loadInstances();
  }, [loadInstances]);

  useEffect(() => {
    if (!instances.some((instance) => instance.status === "creating" || instance.status === "deleting")) {
      return;
    }

    const intervalId = window.setInterval(() => {
      void loadInstances({ silent: true, refreshTeams: false });
    }, 5000);

    return () => window.clearInterval(intervalId);
  }, [instances, loadInstances]);

  const filteredInstances = useMemo(() => {
    const query = searchQuery.trim().toLowerCase();
    return instances.filter((instance) => {
      const availability = availabilityForStatus(instance.status);
      if (availabilityFilter !== "all" && availability !== availabilityFilter) {
        return false;
      }
      if (!query) {
        return true;
      }
      return (
        instance.name.toLowerCase().includes(query) ||
        typeLabel(instance.type).toLowerCase().includes(query) ||
        instance.instance_mode.toLowerCase().includes(query) ||
        modeLabel(instance.instance_mode).toLowerCase().includes(query) ||
        (teamMemberships.get(instance.id) || []).some(({ team, member }) =>
          [
            team.name,
            member.display_name,
            member.member_key,
            member.role,
          ]
            .join(" ")
            .toLowerCase()
            .includes(query),
        )
      );
    });
  }, [availabilityFilter, instances, searchQuery, teamMemberships]);

  const handleDelete = useCallback(
    async (id: number) => {
      try {
        setDeletingIds((prevIds) => [...prevIds, id]);
        await instanceService.deleteInstance(id);
        setPendingDeleteId(null);
        await loadInstances({ silent: true });
      } catch (err: unknown) {
        alert(getErrorMessage(err, t("instances.failedToDelete")));
      } finally {
        setDeletingIds((prevIds) => prevIds.filter((deletingId) => deletingId !== id));
      }
    },
    [loadInstances, t],
  );

  const handleStart = useCallback(
    async (id: number) => {
      try {
        setActionLoading(id);
        await instanceService.startInstance(id);
        await loadInstances({ silent: true, refreshTeams: false });
      } catch (err: unknown) {
        alert(getErrorMessage(err, t("instances.failedToStart")));
      } finally {
        setActionLoading(null);
      }
    },
    [loadInstances, t],
  );

  const handleStop = useCallback(
    async (id: number) => {
      try {
        setActionLoading(id);
        await instanceService.stopInstance(id);
        await loadInstances({ silent: true, refreshTeams: false });
      } catch (err: unknown) {
        alert(getErrorMessage(err, t("instances.failedToStop")));
      } finally {
        setActionLoading(null);
      }
    },
    [loadInstances, t],
  );

  return (
    <UserLayout title={t("instances.listTitle")}>
      <ConfirmDialog
        open={pendingDeleteId !== null}
        title={t("common.delete")}
        message={t("instances.confirmDelete")}
        confirmLabel={t("common.delete")}
        cancelLabel={t("common.cancel")}
        destructive
        loading={pendingDeleteId !== null && deletingIds.includes(pendingDeleteId)}
        onCancel={() => setPendingDeleteId(null)}
        onConfirm={() => {
          if (pendingDeleteId !== null) {
            void handleDelete(pendingDeleteId);
          }
        }}
      />

      <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex flex-wrap gap-2">
          <Link to="/instances/new" className="app-button-primary self-start">
            <Monitor className="h-4 w-4" />
            {t("instances.createInstance")}
          </Link>
          <Link to="/portal" className="app-button-secondary self-start">
            <MonitorPlay className="h-4 w-4" />
            {t("instances.portalView")}
          </Link>
        </div>

        <div className="flex flex-col gap-2 sm:flex-row">
          <div className="relative">
            <Search className="pointer-events-none absolute left-3 top-2.5 h-4 w-4 text-slate-400" />
            <input
              type="text"
              placeholder={t("instances.searchPlaceholder")}
              value={searchQuery}
              onChange={(event) => setSearchQuery(event.target.value)}
              className="app-input w-full pl-9 sm:w-64"
            />
          </div>
          <select
            value={availabilityFilter}
            onChange={(event) => setAvailabilityFilter(event.target.value as AvailabilityFilter)}
            className="app-input"
          >
            <option value="all">All</option>
            <option value="available">Available</option>
            <option value="starting">Starting</option>
            <option value="unavailable">Unavailable</option>
          </select>
        </div>
      </div>

      {loading ? (
        <div className="flex h-64 items-center justify-center text-sm text-slate-500">
          {t("common.loading")}
        </div>
      ) : error ? (
        <div className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          {error}
        </div>
      ) : instances.length === 0 ? (
        <div className="cm-surface p-10 text-center">
          <h3 className="text-sm font-medium text-slate-950">{t("instances.noInstances")}</h3>
          <div className="mt-5">
            <Link to="/instances/new" className="app-button-primary">
              {t("instances.createInstance")}
            </Link>
          </div>
        </div>
      ) : filteredInstances.length === 0 ? (
        <div className="cm-surface p-10 text-center text-sm text-slate-500">
          {t("instances.noMatchingInstances")}
        </div>
      ) : (
        <div className="cm-surface overflow-x-auto">
          <table className="w-full min-w-[760px] table-fixed divide-y divide-slate-200 text-sm">
            <thead className="bg-slate-50 text-left text-xs font-medium uppercase tracking-normal text-slate-500">
              <tr>
                <th className="w-[30%] px-4 py-3">Instance</th>
                <th className="w-[14%] px-4 py-3">Type</th>
                <th className="w-[20%] px-4 py-3">Team</th>
                <th className="w-[16%] px-4 py-3">Availability</th>
                <th className="w-[10%] px-4 py-3">Workspace</th>
                <th className="w-[10%] px-4 py-3 text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 bg-white">
              {filteredInstances.map((instance) => {
                const availability = availabilityForStatus(instance.status);
                const memberships = teamMemberships.get(instance.id) || [];
                const primaryMembership = memberships[0];
                return (
                  <tr
                    key={instance.id}
                    role="link"
                    tabIndex={0}
                    onClick={() => navigate(`/instances/${instance.id}`)}
                    onKeyDown={(event) => {
                      if (event.key === "Enter" || event.key === " ") {
                        event.preventDefault();
                        navigate(`/instances/${instance.id}`);
                      }
                    }}
                    className={`cursor-pointer focus:outline-none focus-visible:bg-slate-50 ${
                      primaryMembership ? "bg-sky-50/30 hover:bg-sky-50/60" : "hover:bg-slate-50"
                    }`}
                  >
                    <td className="max-w-[280px] px-4 py-3">
                      <Link
                        to={`/instances/${instance.id}`}
                        onClick={(event) => event.stopPropagation()}
                        className="block truncate font-medium text-slate-950 hover:text-red-700"
                      >
                        {instance.name}
                      </Link>
                      <div className="mt-1 truncate text-xs text-slate-500">
                        {instance.description || "-"} / {instance.updated_at ? new Date(instance.updated_at).toLocaleString(locale) : "-"}
                      </div>
                    </td>
                    <td className="px-4 py-3 text-slate-600">
                      <div className="flex min-w-0 flex-wrap items-center gap-2">
                        <span className="truncate">{typeLabel(instance.type)}</span>
                        <span
                          className={`inline-flex shrink-0 rounded-md border px-2 py-0.5 text-xs font-medium ${modeClass(
                            instance.instance_mode,
                          )}`}
                        >
                          {modeLabel(instance.instance_mode)}
                        </span>
                      </div>
                    </td>
                    <td className="px-4 py-3 text-slate-600">
                      {primaryMembership ? (
                        <div className="min-w-0">
                          <div className="flex min-w-0 items-center gap-2">
                            <span className="inline-flex shrink-0 rounded-md border border-violet-200 bg-violet-50 px-2 py-0.5 text-xs font-medium text-violet-700">
                              Team
                            </span>
                            <Link
                              to={`/teams/${primaryMembership.team.id}`}
                              onClick={(event) => event.stopPropagation()}
                              className="truncate font-medium text-slate-700 hover:text-red-700"
                            >
                              {primaryMembership.team.name}
                            </Link>
                          </div>
                          <div className="mt-1 truncate text-xs text-slate-500">
                            {primaryMembership.member.display_name ||
                              primaryMembership.member.member_key}{" "}
                            / {primaryMembership.member.role}
                            {memberships.length > 1 ? ` / +${memberships.length - 1}` : ""}
                          </div>
                        </div>
                      ) : (
                        <span className="text-slate-400">-</span>
                      )}
                    </td>
                    <td className="px-4 py-3">
                      <span
                        className={`inline-flex rounded-md border px-2 py-1 text-xs font-medium ${availabilityClass(
                          availability,
                        )}`}
                      >
                        {availabilityLabel(availability)}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-slate-600">
                      {formatBytes(instance.workspace_usage_bytes)}
                    </td>
                    <td
                      className="px-4 py-3"
                      onClick={(event) => event.stopPropagation()}
                      onKeyDown={(event) => event.stopPropagation()}
                    >
                      <div className="flex justify-end gap-2">
                        {instance.status === "running" ? (
                          <button
                            type="button"
                            onClick={() => void handleStop(instance.id)}
                            disabled={actionLoading === instance.id}
                            className="cm-icon-button"
                            title={t("common.stop")}
                          >
                            <Square className="h-4 w-4" />
                          </button>
                        ) : instance.status === "stopped" ? (
                          <button
                            type="button"
                            onClick={() => void handleStart(instance.id)}
                            disabled={actionLoading === instance.id}
                            className="cm-icon-button"
                            title={t("common.start")}
                          >
                            <Play className="h-4 w-4" />
                          </button>
                        ) : null}
                        <button
                          type="button"
                          onClick={() => setPendingDeleteId(instance.id)}
                          disabled={deletingIds.includes(instance.id) || instance.status === "deleting"}
                          className="cm-icon-button border-red-200 text-red-600 hover:border-red-300 hover:bg-red-50 hover:text-red-700"
                          title={t("common.delete")}
                        >
                          <Trash2 className="h-4 w-4" />
                        </button>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </UserLayout>
  );
};

export default InstanceListPage;
