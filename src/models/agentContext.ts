import type {
  AgentMessage,
  AgentMessagePart,
} from "@/models/agentContracts";

const MAX_CONTEXT_TURNS = 20;
const MAX_CONTEXT_MESSAGES = 40;
const MAX_CONTEXT_CHARACTERS = 20_000;

type ContextMessage = {
  role: "user" | "assistant";
  content: string;
};

function textFor(
  message: AgentMessage,
  partsByMessageId: Record<string, AgentMessagePart[]>,
) {
  return (partsByMessageId[message.id] ?? [])
    .slice()
    .sort((left, right) => left.index - right.index)
    .map((part) => part.content.text)
    .join("");
}

function characterCount(messages: ContextMessage[]) {
  return messages.reduce(
    (total, message) => total + Array.from(message.content).length,
    0,
  );
}

export function buildAgentContext(
  messages: AgentMessage[],
  partsByMessageId: Record<string, AgentMessagePart[]>,
): ContextMessage[] {
  const turns: ContextMessage[][] = [];

  for (let index = 0; index < messages.length; index += 1) {
    const message = messages[index];
    if (message.role !== "user" || message.status !== "complete") {
      continue;
    }

    const user = {
      role: "user" as const,
      content: textFor(message, partsByMessageId),
    };
    const next = messages[index + 1];
    if (!next || next.role === "user") {
      turns.push([user]);
      continue;
    }

    if (next.role === "assistant" && next.status === "complete") {
      turns.push([
        user,
        {
          role: "assistant",
          content: textFor(next, partsByMessageId),
        },
      ]);
    }
    index += 1;
  }

  const recentTurns = turns.slice(-MAX_CONTEXT_TURNS);
  const selectedTurns: ContextMessage[][] = [];
  let selectedCharacters = 0;

  for (let index = recentTurns.length - 1; index >= 0; index -= 1) {
    const turn = recentTurns[index];
    const turnCharacters = characterCount(turn);
    if (
      selectedTurns.length > 0 &&
      selectedCharacters + turnCharacters > MAX_CONTEXT_CHARACTERS
    ) {
      break;
    }
    selectedTurns.unshift(turn);
    selectedCharacters += turnCharacters;
  }

  return selectedTurns
    .flat()
    .slice(-MAX_CONTEXT_MESSAGES);
}
