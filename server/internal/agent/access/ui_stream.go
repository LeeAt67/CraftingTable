package access

import (
	"fmt"
	"io"
	"net/http"

	"github.com/leeat/craftingtable/server/internal/agent/core"
)

func writeAgentStreamHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("x-vercel-ai-ui-message-stream", "v1")
	w.WriteHeader(http.StatusOK)
}

func writeAgentStreamEvent(
	w io.Writer,
	cursor int,
	part uiMessageStreamPart,
) error {
	data, err := part.UIStreamJSON()
	if err != nil {
		return fmt.Errorf("marshal %s event: %w", part.UIStreamType(), err)
	}
	if _, err := fmt.Fprintf(w, "id: %d\ndata: %s\n\n", cursor, data); err != nil {
		return fmt.Errorf("write %s event: %w", part.UIStreamType(), err)
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func writeAgentStreamDone(w io.Writer) error {
	if _, err := io.WriteString(w, "data: [DONE]\n\n"); err != nil {
		return fmt.Errorf("write agent stream done: %w", err)
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func uiPartFromStreamChunk(chunk core.StreamChunk) uiMessageStreamPart {
	switch chunk.Kind {
	case core.StreamStart:
		return uiStartPart{
			MessageID: chunk.AssistantMessageID,
			MessageMetadata: uiStartMetadata{
				SessionID:          chunk.SessionID,
				AssistantMessageID: chunk.AssistantMessageID,
				RunID:              chunk.RunID,
				UserMessageID:      chunk.UserMessageID,
			},
		}
	case core.StreamStartStep:
		return uiStartStepPart{}
	case core.StreamTextStart:
		return uiTextStartPart{ID: chunk.PartID}
	case core.StreamTextDelta:
		return uiTextDeltaPart{ID: chunk.PartID, Delta: chunk.Text}
	case core.StreamTextEnd:
		return uiTextEndPart{ID: chunk.PartID}
	case core.StreamFinishStep:
		return uiFinishStepPart{}
	case core.StreamFinish:
		return uiFinishPart{
			FinishReason: "stop",
			MessageMetadata: uiFinishMetadata{
				RunID:  chunk.RunID,
				Status: string(chunk.Status),
			},
		}
	case core.StreamError:
		agentError := chunk.Error
		if agentError == nil {
			value := structToAgentError("模型生成失败")
			agentError = &value
		}
		return uiErrorPart{
			ErrorText:  agentError.Message,
			AgentError: *agentError,
		}
	default:
		return nil
	}
}
