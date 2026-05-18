import api from "./api";
import type {
  CreateTeamRequest,
  DispatchTeamTaskRequest,
  TeamDetails,
  TeamListResponse,
  TeamTask,
} from "../types/team";

export const teamService = {
  listTeams: async (
    page: number = 1,
    limit: number = 20,
  ): Promise<TeamListResponse> => {
    const response = await api.get("/teams", {
      params: { page, limit },
    });
    return response.data.data;
  },

  createTeam: async (data: CreateTeamRequest): Promise<TeamDetails> => {
    const response = await api.post("/teams", data);
    return response.data.data;
  },

  getTeam: async (id: number): Promise<TeamDetails> => {
    const response = await api.get(`/teams/${id}`);
    return response.data.data;
  },

  dispatchTask: async (
    id: number,
    data: DispatchTeamTaskRequest,
  ): Promise<TeamTask> => {
    const response = await api.post(`/teams/${id}/tasks`, data);
    return response.data.data;
  },

  deleteTeam: async (id: number): Promise<void> => {
    await api.delete(`/teams/${id}`);
  },

  deleteMember: async (teamId: number, memberId: number | string): Promise<void> => {
    await api.delete(`/teams/${teamId}/members/${memberId}`);
  },
};
