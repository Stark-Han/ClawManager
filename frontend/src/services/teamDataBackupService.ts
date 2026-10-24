import api from './api';

export interface TeamDataState {table:string;field:string;captured:string|null;restored:string|null;rows:number}
export interface TeamDataSummary {tables:Array<{table:string;rows:number;quarantined_rows:number}>;states:TeamDataState[];sql_readback_verified:boolean}
export interface TeamDataReport {
 backup_id:string;purpose:string;consistency:string;storage:string;
 capture_started_at:string;capture_finished_at:string;integrity_verified:boolean;
 parts?:Array<{name:string;bytes:number;sha256:string}>;
 write_credential_revoked?:boolean;source_untouched:boolean;runtime_dependency_links?:number;
 restore?:{run_id:string;status:string;phase:string;failure?:string;team:TeamDataSummary;file_count:number;verified_file_count:number;verified_object_count:number;verified_redis_count:number};
 verification?:{integrity_verified:boolean;team:TeamDataSummary;file_count:number;object_count:number;redis_count:number;history_parts_verified:number};
}
export interface TeamDataJob {
 public_id:string;operation:'backup'|'restore'|'cleanup';source_backup_id?:string;
 status:string;phase:string;error_code?:string;cancel_requested:boolean;
 created_at:string;started_at?:string;finished_at?:string;report?:TeamDataReport;
}
export interface TeamDataOverview {configured:boolean;storage?:string;consistency?:string;max_bytes?:number;current_teams?:number;historical_deleted_teams?:number}
const base='/admin/system-backup/team-data';
export const teamDataBackupService={
 overview:async(signal?:AbortSignal):Promise<TeamDataOverview>=>(await api.get(`${base}/overview`,{signal})).data.data,
 list:async(signal?:AbortSignal):Promise<TeamDataJob[]>=>(await api.get(`${base}/jobs`,{signal})).data.data.items,
 get:async(id:string,signal?:AbortSignal):Promise<TeamDataJob>=>(await api.get(`${base}/jobs/${encodeURIComponent(id)}`,{signal})).data.data,
 create:async(operation:TeamDataJob['operation'],key:string,source?:string):Promise<TeamDataJob>=>(await api.post(`${base}/jobs`,{operation,...(source?{source_backup_id:source}:{})},{headers:{'Idempotency-Key':key}})).data.data,
 cancel:async(id:string):Promise<TeamDataJob>=>(await api.post(`${base}/jobs/${encodeURIComponent(id)}/cancel`,{})).data.data,
};
export const teamDataActive=(j:TeamDataJob)=>['pending','running','canceling'].includes(j.status);
