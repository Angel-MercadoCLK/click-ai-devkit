/** What the message list needs to know to decide whether the chat is empty. */
export type ChatViewState = {
  messages: readonly unknown[];
  busy: boolean;
  submittedPrompt: string;
  liveText: string;
  tools: readonly unknown[];
};

/** True only when there is no history, no turn in flight, no streamed text and no tool activity. */
export function isChatEmpty(chat: ChatViewState): boolean {
  return chat.messages.length === 0 && !chat.busy && !chat.submittedPrompt && !chat.liveText && chat.tools.length === 0;
}
