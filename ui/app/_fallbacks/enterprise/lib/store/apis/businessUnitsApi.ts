import { BusinessUnit } from "@enterprise/lib/types/workspace";
import { baseApi } from "@/lib/store/apis/baseApi";

export interface BusinessUnitTeam {
	id: string;
	name: string;
}

export const businessUnitsApi = baseApi.injectEndpoints({
	endpoints: (builder) => ({
		getBusinessUnits: builder.query<{ business_units: BusinessUnit[]; total: number }, void>({
			query: () => ({ url: "/governance/business-units", params: { limit: 100 } }),
			providesTags: ["BusinessUnits"],
		}),
		createBusinessUnit: builder.mutation<{ business_unit: BusinessUnit }, { name: string }>({
			query: (body) => ({ url: "/governance/business-units", method: "POST", body }),
			invalidatesTags: ["BusinessUnits"],
		}),
		deleteBusinessUnit: builder.mutation<void, string>({
			query: (id) => ({ url: `/governance/business-units/${id}`, method: "DELETE" }),
			invalidatesTags: ["BusinessUnits"],
		}),
		getBusinessUnitTeams: builder.query<{ teams: BusinessUnitTeam[]; total: number }, string>({
			query: (id) => ({ url: `/governance/business-units/${id}/teams` }),
			providesTags: (_result, _error, id) => [{ type: "BusinessUnits", id }],
		}),
		assignBusinessUnitTeam: builder.mutation<void, { id: string; team_id: string }>({
			query: ({ id, team_id }) => ({
				url: `/governance/business-units/${id}/teams`,
				method: "POST",
				body: { team_id },
			}),
			invalidatesTags: ["BusinessUnits"],
		}),
		removeBusinessUnitTeam: builder.mutation<void, { id: string; team_id: string }>({
			query: ({ id, team_id }) => ({
				url: `/governance/business-units/${id}/teams/${team_id}`,
				method: "DELETE",
			}),
			invalidatesTags: ["BusinessUnits"],
		}),
	}),
});

export const {
	useGetBusinessUnitsQuery,
	useCreateBusinessUnitMutation,
	useDeleteBusinessUnitMutation,
	useGetBusinessUnitTeamsQuery,
	useAssignBusinessUnitTeamMutation,
	useRemoveBusinessUnitTeamMutation,
} = businessUnitsApi;
