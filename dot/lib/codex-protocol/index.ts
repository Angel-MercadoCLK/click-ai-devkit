// The slice of the Codex app-server protocol Dot uses. The files next to this one are copied from
// `codex app-server generate-ts --experimental` (codex-cli 0.163.0, the version Dot pins; see CODEX_SUPPORTED_RANGE in
// desktop/codex-app-server.mts). They are generated: regenerate them with the pinned version instead of editing them.
// `--experimental` is needed because Dot opts into the experimental API (`availableDecisions` on command approvals).

export const CODEX_PROTOCOL_VERSION = "0.163.0";

export type { InitializeParams } from "./InitializeParams";
export type { InitializeResponse } from "./InitializeResponse";
export type { GetAccountResponse } from "./v2/GetAccountResponse";
export type { LoginAccountParams } from "./v2/LoginAccountParams";
export type { LoginAccountResponse } from "./v2/LoginAccountResponse";
export type { AccountLoginCompletedNotification } from "./v2/AccountLoginCompletedNotification";
export type { ThreadStartParams } from "./v2/ThreadStartParams";
export type { ThreadStartResponse } from "./v2/ThreadStartResponse";
export type { ThreadResumeParams } from "./v2/ThreadResumeParams";
export type { ThreadResumeResponse } from "./v2/ThreadResumeResponse";
export type { ThreadListParams } from "./v2/ThreadListParams";
export type { ThreadListResponse } from "./v2/ThreadListResponse";
export type { ThreadReadParams } from "./v2/ThreadReadParams";
export type { ThreadReadResponse } from "./v2/ThreadReadResponse";
export type { Thread } from "./v2/Thread";
export type { ThreadItem } from "./v2/ThreadItem";
export type { TurnStartParams } from "./v2/TurnStartParams";
export type { TurnStartResponse } from "./v2/TurnStartResponse";
export type { TurnInterruptParams } from "./v2/TurnInterruptParams";
export type { Turn } from "./v2/Turn";
export type { UserInput } from "./v2/UserInput";
export type { AgentMessageDeltaNotification } from "./v2/AgentMessageDeltaNotification";
export type { ItemStartedNotification } from "./v2/ItemStartedNotification";
export type { ItemCompletedNotification } from "./v2/ItemCompletedNotification";
export type { TurnCompletedNotification } from "./v2/TurnCompletedNotification";
export type { ErrorNotification } from "./v2/ErrorNotification";
export type { ServerRequestResolvedNotification } from "./v2/ServerRequestResolvedNotification";
export type { CommandExecutionRequestApprovalParams } from "./v2/CommandExecutionRequestApprovalParams";
export type { CommandExecutionRequestApprovalResponse } from "./v2/CommandExecutionRequestApprovalResponse";
export type { FileChangeRequestApprovalParams } from "./v2/FileChangeRequestApprovalParams";
export type { FileChangeRequestApprovalResponse } from "./v2/FileChangeRequestApprovalResponse";
export type { PermissionsRequestApprovalParams } from "./v2/PermissionsRequestApprovalParams";
export type { PermissionsRequestApprovalResponse } from "./v2/PermissionsRequestApprovalResponse";
export type { ToolRequestUserInputParams } from "./v2/ToolRequestUserInputParams";
export type { ToolRequestUserInputResponse } from "./v2/ToolRequestUserInputResponse";
export type { McpServerElicitationRequestParams } from "./v2/McpServerElicitationRequestParams";
export type { McpServerElicitationRequestResponse } from "./v2/McpServerElicitationRequestResponse";
